package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oscarcode/elementary-claw/internal/bootstrap"
	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/memory"
	"github.com/oscarcode/elementary-claw/internal/prompt"
	"github.com/oscarcode/elementary-claw/internal/providers/openai"
	"github.com/oscarcode/elementary-claw/internal/session"
	"github.com/oscarcode/elementary-claw/internal/skills"
	"github.com/oscarcode/elementary-claw/internal/tools"
)

// maxToolCallIterations is the safety limit for the tool-call loop to prevent
// infinite cycles if the model keeps requesting tools indefinitely.
const maxToolCallIterations = 25

const maxToolImageAttachmentBytes = 10 * 1024 * 1024

func newHandler(paths config.Paths, store *session.Store, registry *tools.Registry, skillsRegistry *skills.Registry) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		status, err := Inspect(paths, store)
		if err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}

		httpStatus := http.StatusOK
		if !status.ConfigPresent {
			httpStatus = http.StatusServiceUnavailable
		}

		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(httpStatus)
		_ = json.NewEncoder(response).Encode(map[string]any{
			"ok":               httpStatus == http.StatusOK,
			"configPresent":    status.ConfigPresent,
			"authPresent":      status.AuthPresent,
			"providerPending":  status.ProviderPending,
			"bootstrapPresent": status.BootstrapPresent,
			"sessionCount":     status.SessionCount,
		})
	})

	mux.HandleFunc("/status", func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		status, err := Inspect(paths, store)
		if err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}

		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(status)
	})

	mux.HandleFunc("/auth/token", func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		cfg, err := config.LoadFileConfig(paths)
		if err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]any{
			"provider": cfg.Agent.Provider,
			"model":    normalizeProviderModel(cfg.Agent.Provider, cfg.Agent.Model),
		}
		if target, err := resolveUpstreamTarget(paths, cfg); err == nil {
			payload["baseUrl"] = target.BaseURL
			payload["source"] = target.Source
			if authHeader := target.Headers.Get("Authorization"); authHeader != "" {
				payload["authConfigured"] = true
			}
		} else {
			payload["authConfigured"] = false
			payload["detail"] = err.Error()
		}

		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(payload)
	})

	mux.HandleFunc("/v1/runtime", func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		cfg, err := config.LoadFileConfig(paths)
		if err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := map[string]any{
			"provider": cfg.Agent.Provider,
			"model":    normalizeProviderModel(cfg.Agent.Provider, cfg.Agent.Model),
		}

		if target, err := resolveUpstreamTarget(paths, cfg); err == nil {
			payload["baseUrl"] = target.BaseURL
			payload["source"] = target.Source
			payload["authConfigured"] = target.Headers.Get("Authorization") != ""
		} else {
			payload["authConfigured"] = false
			payload["detail"] = err.Error()
		}

		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(payload)
	})

	mux.HandleFunc("/v1/providers/openai/connect", func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var payload struct {
			APIKey string `json:"api_key"`
			Model  string `json:"model"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			http.Error(response, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := openai.SaveAPIKey(paths, payload.APIKey); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}

		cfg, err := config.LoadFileConfig(paths)
		if err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}

		model := strings.TrimSpace(payload.Model)
		if model == "" {
			model = openai.DefaultCodexModel
		}

		cfg.Agent.Provider = "openai"
		cfg.Agent.Model = model
		cfg.Agent.BaseURL = openai.DefaultAPIBaseURL
		cfg.Setup.ProviderPending = false

		if err := config.SaveFileConfig(paths, cfg); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}

		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{
			"ok":       true,
			"provider": cfg.Agent.Provider,
			"model":    cfg.Agent.Model,
			"baseUrl":  cfg.Agent.BaseURL,
		})
	})

	mux.HandleFunc("/v1/providers/openai-codex/init", func(response http.ResponseWriter, request *http.Request) {
		handleOpenAICodexInit(response, request, paths)
	})

	mux.HandleFunc("/v1/providers/openai-codex/exchange", func(response http.ResponseWriter, request *http.Request) {
		handleOpenAICodexExchange(response, request, paths)
	})

	mux.HandleFunc("/v1/sessions/", func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		sessionID := strings.TrimPrefix(request.URL.Path, "/v1/sessions/")
		if sessionID == "" || strings.Contains(sessionID, "/") {
			http.Error(response, "invalid session id", http.StatusBadRequest)
			return
		}

		record, err := store.Get(sessionID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.Error(response, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}

		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(record)
	})

	mux.HandleFunc("/v1/chat/completions", func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		cfg, err := config.LoadFileConfig(paths)
		if err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}

		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(response, "read request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(response, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if strings.TrimSpace(cfg.Agent.Provider) == "openai-codex" {
			if stream, _ := payload["stream"].(bool); stream {
				handleOpenAICodexStreamingChatCompletions(response, request, paths, cfg, payload, store, registry, skillsRegistry)
				return
			}
			handleOpenAICodexChatCompletions(response, request, paths, cfg, payload, store, registry, skillsRegistry)
			return
		}

		target, err := resolveUpstreamTarget(paths, cfg)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadGateway)
			return
		}

		if stream, _ := payload["stream"].(bool); stream {
			handleStreamingChatCompletions(response, request, paths, cfg, target, payload, store, registry, skillsRegistry)
			return
		}

		incomingMessages, err := decodeRequestMessages(payload["messages"])
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}

		sessionID := resolveSessionID(paths, request, payload, store)
		record, err := loadOrCreateSession(store, sessionID)
		if err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}

		// Build the full message list: session history + incoming messages.
		allMessages := append(cloneMessages(record.Messages), incomingMessages...)
		record.Messages = append(record.Messages, incomingMessages...)

		// Inject system prompt at the front if not already present.
		allMessages = prependSystemPrompt(paths, registry, skillsRegistry, allMessages)

		delete(payload, "session_id")
		removeSessionMetadata(payload)
		delete(payload, "x_claw_preview")
		normalizePayloadModel(cfg, payload)

		// x_client_tools=true: the caller manages its own tools client-side.
		// In this mode we skip gateway tool injection and the tool-call execution
		// loop, acting as a pure proxy so tool_calls reach the client unchanged.
		clientTools, _ := payload["x_client_tools"].(bool)
		delete(payload, "x_client_tools")

		// Inject gateway tool definitions only when the registry has tools and
		// the client has not claimed ownership of tool handling.
		if !clientTools && registry != nil && registry.Count() > 0 {
			payload["tools"] = registry.Definitions()
		}

		upstreamURL := buildUpstreamURL(target, request.URL.Path)
		if trimmedQuery := strings.TrimSpace(request.URL.RawQuery); trimmedQuery != "" {
			upstreamURL += "?" + trimmedQuery
		}

		// --- Tool-call loop ---
		// Send to the LLM, execute any requested tool calls, feed results
		// back, and repeat until the model produces a final text response
		// or we hit the iteration safety limit.
		// Skipped entirely when clientTools=true (caller handles tool execution).
		var lastResponseBody []byte
		var lastStatusCode int

		for iteration := 0; iteration < maxToolCallIterations; iteration++ {
			payload["messages"] = encodeMessages(allMessages)

			mergedBody, err := json.Marshal(payload)
			if err != nil {
				http.Error(response, "marshal request: "+err.Error(), http.StatusInternalServerError)
				return
			}

			proxyRequest, err := http.NewRequestWithContext(request.Context(), http.MethodPost, upstreamURL, bytes.NewReader(mergedBody))
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadGateway)
				return
			}
			copyRequestHeaders(proxyRequest.Header, request.Header)
			proxyRequest.Header.Set("Content-Type", "application/json")
			applyTargetHeaders(proxyRequest.Header, target.Headers)

			proxyResponse, err := http.DefaultClient.Do(proxyRequest)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadGateway)
				return
			}

			responseBody, err := io.ReadAll(proxyResponse.Body)
			proxyResponse.Body.Close()
			if err != nil {
				http.Error(response, "read upstream response: "+err.Error(), http.StatusBadGateway)
				return
			}

			lastResponseBody = responseBody
			lastStatusCode = proxyResponse.StatusCode

			// If the upstream returned an error, stop the loop.
			if proxyResponse.StatusCode < 200 || proxyResponse.StatusCode >= 300 {
				break
			}

			// Client-side tool mode: return the raw response immediately so
			// tool_calls are visible to the caller.
			if clientTools {
				break
			}

			assistantMsg, finishReason, ok := extractAssistantResponse(responseBody)
			if !ok {
				break
			}

			// Append the assistant message to the conversation.
			allMessages = append(allMessages, assistantMsg)
			record.Messages = append(record.Messages, assistantMsg)

			// If there are no tool calls, the model is done.
			if len(assistantMsg.ToolCalls) == 0 || finishReason == "stop" {
				break
			}

			// No registry means we can't execute tools — return as-is.
			if registry == nil {
				break
			}

			// Execute each tool call and append results.
			for _, tc := range assistantMsg.ToolCalls {
				toolMessages := executeToolCallMessages(request.Context(), registry, tc)
				allMessages = append(allMessages, toolMessages...)
				record.Messages = append(record.Messages, toolMessages...)
			}

			// Continue the loop: send tool results back to the LLM.
		}

		// Persist the full conversation.
		if lastStatusCode >= 200 && lastStatusCode < 300 {
			if _, err := store.Save(&record); err != nil {
				http.Error(response, err.Error(), http.StatusInternalServerError)
				return
			}

			// Check for bootstrap completion marker in the last assistant message.
			checkBootstrapCompletion(paths, record.Messages)

			// Mark session for compaction if threshold exceeded.
			// Actual compaction (LLM summarization) happens asynchronously
			// to avoid blocking the response.
			if session.NeedsCompaction(&record, session.DefaultCompactionThreshold) {
				go triggerCompaction(paths, store, &record, cfg, target, request.Header)
			}
		}

		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("X-Session-ID", sessionID)
		setProviderErrorHeaders(response.Header(), cfg.Agent.Provider, lastStatusCode, string(lastResponseBody))
		response.Header().Set("X-Claw-Provider", cfg.Agent.Provider)
		response.WriteHeader(lastStatusCode)
		_, _ = response.Write(lastResponseBody)
	})

	mux.HandleFunc("/v1/skills", func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if skillsRegistry == nil {
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode([]any{})
			return
		}

		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(skillsRegistry.ToJSON())
	})

	mux.HandleFunc("/v1/skills/", func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		skillName := strings.TrimPrefix(request.URL.Path, "/v1/skills/")
		if skillName == "" || strings.Contains(skillName, "/") {
			http.Error(response, "invalid skill name", http.StatusBadRequest)
			return
		}

		if skillsRegistry == nil {
			http.Error(response, "skills not available", http.StatusNotFound)
			return
		}

		s, ok := skillsRegistry.Get(skillName)
		if !ok {
			http.Error(response, fmt.Sprintf("skill %q not found", skillName), http.StatusNotFound)
			return
		}

		entry := map[string]any{
			"name":         s.Name,
			"title":        s.Title,
			"description":  s.Description,
			"source":       string(s.Source),
			"enabled":      s.Enabled,
			"path":         s.Path,
			"instructions": s.Instructions,
		}
		if s.Manifest != nil {
			entry["manifest"] = s.Manifest
		}

		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(entry)
	})

	mux.HandleFunc("/v1/", func(response http.ResponseWriter, request *http.Request) {
		cfg, err := config.LoadFileConfig(paths)
		if err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}

		target, err := resolveUpstreamTarget(paths, cfg)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadGateway)
			return
		}

		upstreamURL := buildUpstreamURL(target, request.URL.Path)
		if trimmedQuery := strings.TrimSpace(request.URL.RawQuery); trimmedQuery != "" {
			upstreamURL += "?" + trimmedQuery
		}

		proxyRequest, err := http.NewRequestWithContext(request.Context(), request.Method, upstreamURL, request.Body)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadGateway)
			return
		}

		copyRequestHeaders(proxyRequest.Header, request.Header)
		applyTargetHeaders(proxyRequest.Header, target.Headers)

		proxyResponse, err := http.DefaultClient.Do(proxyRequest)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadGateway)
			return
		}
		defer proxyResponse.Body.Close()

		copyResponseHeaders(response.Header(), proxyResponse.Header)
		response.WriteHeader(proxyResponse.StatusCode)
		_, _ = io.Copy(response, proxyResponse.Body)
	})

	// Registrar handlers de cron
	if err := registerCronHandlers(mux, paths); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to register cron handlers: %v\n", err)
	}

	return mux
}

// detectSystemTimezone returns the IANA timezone name for the host system.
// It tries (in order): the TZ env var, the /etc/localtime symlink (most
// reliable on systemd systems), and /etc/timezone (Debian legacy, may be stale).
// Falls back to "UTC".
func detectSystemTimezone() string {
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}

	if link, err := os.Readlink("/etc/localtime"); err == nil {
		// e.g. /usr/share/zoneinfo/America/Mexico_City
		const prefix = "/usr/share/zoneinfo/"
		if idx := strings.Index(link, prefix); idx >= 0 {
			tz := link[idx+len(prefix):]
			if _, err := time.LoadLocation(tz); err == nil {
				return tz
			}
		}
	}

	if data, err := os.ReadFile("/etc/timezone"); err == nil {
		if tz := strings.TrimSpace(string(data)); tz != "" {
			if _, err := time.LoadLocation(tz); err == nil {
				return tz
			}
		}
	}

	return "UTC"
}

func copyRequestHeaders(destination http.Header, source http.Header) {
	for headerName, values := range source {
		if strings.EqualFold(headerName, "Content-Length") || strings.EqualFold(headerName, "X-Session-ID") {
			continue
		}
		for _, value := range values {
			destination.Add(headerName, value)
		}
	}
}

func copyResponseHeaders(destination http.Header, source http.Header) {
	for headerName, values := range source {
		for _, value := range values {
			destination.Add(headerName, value)
		}
	}
}

func applyTargetHeaders(destination http.Header, overrides http.Header) {
	for headerName, values := range overrides {
		destination.Del(headerName)
		for _, value := range values {
			destination.Add(headerName, value)
		}
	}
}

func normalizePayloadModel(cfg config.FileConfig, payload map[string]any) {
	if model, ok := payload["model"].(string); ok && strings.TrimSpace(model) != "" {
		payload["model"] = normalizeProviderModel(cfg.Agent.Provider, model)
		return
	}
	if strings.TrimSpace(cfg.Agent.Model) != "" {
		payload["model"] = normalizeProviderModel(cfg.Agent.Provider, cfg.Agent.Model)
	}
}

func normalizeProviderModel(provider, model string) string {
	model = strings.TrimSpace(model)
	if provider == "github-copilot" || provider == "openai-codex" {
		model = strings.TrimPrefix(model, "github-copilot/")
		model = strings.TrimPrefix(model, "openai-codex/")
	}
	return model
}

func resolveSessionID(paths config.Paths, request *http.Request, payload map[string]any, store *session.Store) string {
	if sessionID := strings.TrimSpace(request.Header.Get("X-Session-ID")); sessionID != "" {
		return sessionID
	}
	if sessionID := strings.TrimSpace(request.URL.Query().Get("session_id")); sessionID != "" {
		return sessionID
	}
	if sessionID, ok := payload["session_id"].(string); ok && strings.TrimSpace(sessionID) != "" {
		return strings.TrimSpace(sessionID)
	}
	if metadata, ok := payload["metadata"].(map[string]any); ok {
		if sessionID, ok := metadata["session_id"].(string); ok && strings.TrimSpace(sessionID) != "" {
			return strings.TrimSpace(sessionID)
		}
	}
	if bootstrap.DetectBootstrapMode(paths) {
		if _, err := store.Get("bootstrap"); err == nil {
			return "bootstrap"
		}
	}
	return "default"
}

func loadOrCreateSession(store *session.Store, sessionID string) (session.Record, error) {
	record, err := store.Get(sessionID)
	if err != nil {
		return session.Record{}, err
	}
	if record != nil {
		return *record, nil
	}

	title := "Conversation"
	kind := "chat"
	if sessionID == "bootstrap" {
		title = "First Login Welcome"
		kind = "bootstrap"
	}

	return session.Record{
		ID:        sessionID,
		Kind:      kind,
		Title:     title,
		CreatedAt: time.Now().UTC(),
		Messages:  nil,
	}, nil
}

func cloneMessages(messages []session.Message) []session.Message {
	cloned := make([]session.Message, len(messages))
	copy(cloned, messages)
	return cloned
}

// prependSystemPrompt builds the system prompt from workspace personality files
// and prepends it as a "system" role message if no system message is already
// present at the start of the conversation.
func prependSystemPrompt(paths config.Paths, reg *tools.Registry, skillsRegistry *skills.Registry, messages []session.Message) []session.Message {
	if err := config.RepairLegacyWorkspaceFiles(paths); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "repair legacy workspace files: %v\n", err)
	}

	// Don't prepend if messages already start with a system message.
	if len(messages) > 0 && messages[0].Role == "system" {
		return messages
	}

	opts := prompt.FullPromptOptions{}

	// Inject system timezone so the model knows the user's local time.
	opts.Timezone = detectSystemTimezone()

	// Build tool descriptions from registry.
	if reg != nil {
		for _, t := range reg.List() {
			opts.ToolDescriptions = append(opts.ToolDescriptions,
				fmt.Sprintf("`%s` — %s", t.Name(), t.Description()))
		}
	}

	// Build skill entries from registry.
	if skillsRegistry != nil {
		for _, s := range skillsRegistry.ListEnabled() {
			desc := s.Description
			if desc == "" {
				desc = s.Title
			}
			if desc != "" {
				opts.SkillEntries = append(opts.SkillEntries,
					fmt.Sprintf("**%s**: %s", s.Name, desc))
			}
		}
	}

	// Build memory section.
	opts.MemorySection = memory.BuildMemorySection(paths)

	// Detect bootstrap mode.
	if bootstrap.DetectBootstrapMode(paths) {
		opts.BootstrapInstructions = bootstrap.ReadBootstrapInstructions(paths)
	}

	systemContent := prompt.BuildFullSystemPrompt(paths, opts)
	if systemContent == "" {
		return messages
	}

	systemMessage := session.Message{
		Role:    "system",
		Content: systemContent,
	}
	return append([]session.Message{systemMessage}, messages...)
}

// checkBootstrapCompletion scans the last assistant message for the
// [BOOTSTRAP_COMPLETE] marker and, if found, completes the bootstrap ritual
// by removing BOOTSTRAP.md and writing state.
func checkBootstrapCompletion(paths config.Paths, messages []session.Message) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			if strings.Contains(messages[i].Content, "[BOOTSTRAP_COMPLETE]") {
				_ = bootstrap.CompleteBootstrap(paths)
			}
			return
		}
	}
}

// triggerCompaction sends a summarization request to the LLM and replaces
// old messages with the resulting summary. Runs in a goroutine so it does
// not block the client response.
func triggerCompaction(paths config.Paths, store *session.Store, record *session.Record, cfg config.FileConfig, target upstreamTarget, reqHeaders http.Header) {
	compactionMsgs := session.BuildCompactionRequest(record, session.DefaultKeepLastMessages)
	if compactionMsgs == nil {
		return
	}

	payload := map[string]any{
		"model":      normalizeProviderModel(cfg.Agent.Provider, cfg.Agent.Model),
		"messages":   encodeMessages(compactionMsgs),
		"max_tokens": 500,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	upstreamURL := buildUpstreamURL(target, "/v1/chat/completions")
	req, err := http.NewRequest(http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	applyTargetHeaders(req.Header, target.Headers)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	msg, _, ok := extractAssistantResponse(respBody)
	if !ok || msg.Content == "" {
		return
	}

	session.ApplyCompaction(record, msg.Content, session.DefaultKeepLastMessages)
	_, _ = store.Save(record)
}

func encodeMessages(messages []session.Message) []map[string]any {
	encoded := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		entry := map[string]any{
			"role": message.Role,
		}

		// Tool result messages use tool_call_id and name, content may be empty.
		if message.Role == "tool" {
			entry["tool_call_id"] = message.ToolCallID
			entry["name"] = message.Name
			entry["content"] = message.Content
			encoded = append(encoded, entry)
			continue
		}

		// Assistant messages may carry tool_calls instead of (or alongside) content.
		if len(message.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(message.ToolCalls))
			for _, tc := range message.ToolCalls {
				calls = append(calls, map[string]any{
					"id":   tc.ID,
					"type": tc.Type,
					"function": map[string]any{
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
					},
				})
			}
			entry["tool_calls"] = calls
			// Content can be null/empty when the model only issues tool calls.
			if message.Content != "" {
				entry["content"] = message.Content
			} else {
				entry["content"] = nil
			}
			encoded = append(encoded, entry)
			continue
		}

		if len(message.ContentParts) > 0 {
			entry["content"] = encodeContentParts(message.ContentParts)
		} else {
			entry["content"] = message.Content
		}
		encoded = append(encoded, entry)
	}
	return encoded
}

func encodeContentParts(parts []session.ContentPart) []map[string]any {
	encoded := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "text", "input_text":
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			encoded = append(encoded, map[string]any{
				"type": "text",
				"text": text,
			})
		case "image_url":
			if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.URL) == "" {
				continue
			}
			image := map[string]any{"url": part.ImageURL.URL}
			if strings.TrimSpace(part.ImageURL.Detail) != "" {
				image["detail"] = part.ImageURL.Detail
			}
			encoded = append(encoded, map[string]any{
				"type":      "image_url",
				"image_url": image,
			})
		}
	}
	return encoded
}

func decodeRequestMessages(raw any) ([]session.Message, error) {
	if raw == nil {
		return nil, nil
	}

	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("messages must be an array")
	}

	decoded := make([]session.Message, 0, len(items))
	for _, item := range items {
		messageMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("messages must contain objects")
		}
		role, _ := messageMap["role"].(string)
		msg := session.Message{
			Role:    strings.TrimSpace(role),
			Content: normalizeMessageContent(messageMap["content"]),
		}
		msg.ContentParts = decodeContentParts(messageMap["content"])

		// Parse tool_call_id and name for tool result messages.
		if toolCallID, ok := messageMap["tool_call_id"].(string); ok {
			msg.ToolCallID = toolCallID
		}
		if name, ok := messageMap["name"].(string); ok {
			msg.Name = name
		}

		// Parse tool_calls array for assistant messages requesting tools.
		if rawCalls, ok := messageMap["tool_calls"].([]any); ok {
			for _, rawCall := range rawCalls {
				callMap, ok := rawCall.(map[string]any)
				if !ok {
					continue
				}
				tc := session.ToolCall{
					ID:   stringField(callMap, "id"),
					Type: stringField(callMap, "type"),
				}
				if fn, ok := callMap["function"].(map[string]any); ok {
					tc.Function = session.FunctionCall{
						Name:      stringField(fn, "name"),
						Arguments: stringField(fn, "arguments"),
					}
				}
				msg.ToolCalls = append(msg.ToolCalls, tc)
			}
		}

		decoded = append(decoded, msg)
	}

	return decoded, nil
}

func decodeContentParts(raw any) []session.ContentPart {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	parts := make([]session.ContentPart, 0, len(items))
	for _, item := range items {
		part, ok := item.(map[string]any)
		if !ok {
			continue
		}
		partType, _ := part["type"].(string)
		switch partType {
		case "text", "input_text":
			text, _ := part["text"].(string)
			if strings.TrimSpace(text) != "" {
				parts = append(parts, session.ContentPart{Type: "text", Text: text})
			}
		case "image_url":
			image, _ := part["image_url"].(map[string]any)
			url, _ := image["url"].(string)
			detail, _ := image["detail"].(string)
			if strings.TrimSpace(url) != "" {
				parts = append(parts, session.ContentPart{
					Type: "image_url",
					ImageURL: &session.ImageURL{
						URL:    url,
						Detail: detail,
					},
				})
			}
		}
	}
	return parts
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func normalizeMessageContent(raw any) string {
	switch typed := raw.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			switch part := item.(type) {
			case string:
				parts = append(parts, part)
			case map[string]any:
				if text, ok := part["text"].(string); ok {
					parts = append(parts, text)
					continue
				}
				encoded, err := json.Marshal(part)
				if err == nil {
					parts = append(parts, string(encoded))
				}
			default:
				parts = append(parts, fmt.Sprint(part))
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	default:
		encoded, err := json.Marshal(typed)
		if err == nil {
			return string(encoded)
		}
		return fmt.Sprint(typed)
	}
}

// extractAssistantResponse parses the LLM response and returns the assistant
// message along with the finish reason. It supports both text-only responses
// and responses containing tool_calls.
func extractAssistantResponse(responseBody []byte) (session.Message, string, bool) {
	var payload struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role      string `json:"role"`
				Content   any    `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return session.Message{}, "", false
	}
	if len(payload.Choices) == 0 {
		return session.Message{}, "", false
	}

	choice := payload.Choices[0]
	message := session.Message{
		Role:    strings.TrimSpace(choice.Message.Role),
		Content: normalizeMessageContent(choice.Message.Content),
	}
	if message.Role == "" {
		message.Role = "assistant"
	}

	// Parse tool_calls from the response.
	for _, tc := range choice.Message.ToolCalls {
		message.ToolCalls = append(message.ToolCalls, session.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: session.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	finishReason := strings.TrimSpace(choice.FinishReason)

	// A message with tool_calls is always valid even if content is empty.
	if len(message.ToolCalls) > 0 {
		return message, finishReason, true
	}

	// For text-only messages, require non-empty content.
	if strings.TrimSpace(message.Content) == "" {
		return session.Message{}, finishReason, false
	}
	return message, finishReason, true
}

// extractAssistantMessage is a backward-compatible wrapper that ignores the finish reason.
func extractAssistantMessage(responseBody []byte) (session.Message, bool) {
	msg, _, ok := extractAssistantResponse(responseBody)
	return msg, ok
}

// executeToolCallMessages runs a single tool call against the registry and
// returns the tool-result message plus optional multimodal follow-up messages.
func executeToolCallMessages(ctx context.Context, registry *tools.Registry, tc session.ToolCall) []session.Message {
	tool, ok := registry.Get(tc.Function.Name)
	if !ok {
		return []session.Message{{
			Role:       "tool",
			ToolCallID: tc.ID,
			Name:       tc.Function.Name,
			Content:    fmt.Sprintf("error: unknown tool %q", tc.Function.Name),
		}}
	}

	result := tool.Execute(ctx, tc.Function.Arguments)

	content := result.Content
	if result.IsError {
		content = "error: " + content
	}

	messages := []session.Message{{
		Role:       "tool",
		ToolCallID: tc.ID,
		Name:       tc.Function.Name,
		Content:    content,
	}}
	if !result.IsError {
		messages = append(messages, toolAttachmentMessages(result.Attachments)...)
	}
	return messages
}

func toolAttachmentMessages(attachments []tools.Attachment) []session.Message {
	if len(attachments) == 0 {
		return nil
	}
	messages := make([]session.Message, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Type != "image" {
			continue
		}
		path := strings.TrimSpace(attachment.Path)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 || len(data) > maxToolImageAttachmentBytes {
			continue
		}
		mimeType := strings.TrimSpace(attachment.MimeType)
		if mimeType == "" {
			mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
		}
		if mimeType == "" {
			mimeType = "image/png"
		}
		if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
			continue
		}
		dataURL := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
		label := "Attached image from tool result. Look at the attached image and answer the user's request based on it."
		if base := filepath.Base(path); base != "." && base != string(filepath.Separator) {
			label += " File: " + base
		}
		messages = append(messages, session.Message{
			Role:    "user",
			Content: label,
			ContentParts: []session.ContentPart{
				{Type: "text", Text: label},
				{Type: "image_url", ImageURL: &session.ImageURL{URL: dataURL, Detail: "auto"}},
			},
		})
	}
	return messages
}

func removeSessionMetadata(payload map[string]any) {
	if metadata, ok := payload["metadata"].(map[string]any); ok {
		delete(metadata, "session_id")
		if len(metadata) == 0 {
			delete(payload, "metadata")
		}
	}
}
