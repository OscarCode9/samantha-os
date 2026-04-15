package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/session"
	"github.com/oscarcode/elementary-claw/internal/skills"
	"github.com/oscarcode/elementary-claw/internal/tools"
)

// maxToolCallIterations is the safety limit for the tool-call loop to prevent
// infinite cycles if the model keeps requesting tools indefinitely.
const maxToolCallIterations = 25

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

		target, err := resolveUpstreamTarget(paths, cfg)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadGateway)
			return
		}

		payload := map[string]any{
			"provider": cfg.Agent.Provider,
			"baseUrl":  target.BaseURL,
			"source":   target.Source,
		}
		if authHeader := target.Headers.Get("Authorization"); authHeader != "" {
			payload["authConfigured"] = true
		}

		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(payload)
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

		target, err := resolveUpstreamTarget(paths, cfg)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadGateway)
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

		if stream, _ := payload["stream"].(bool); stream {
			handleStreamingChatCompletions(response, request, paths, cfg, target, payload, store, registry)
			return
		}

		incomingMessages, err := decodeRequestMessages(payload["messages"])
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}

		sessionID := resolveSessionID(request, payload, store)
		record, err := loadOrCreateSession(store, sessionID)
		if err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}

		// Build the full message list: session history + incoming messages.
		allMessages := append(cloneMessages(record.Messages), incomingMessages...)
		record.Messages = append(record.Messages, incomingMessages...)

		delete(payload, "session_id")
		removeSessionMetadata(payload)

		// Inject tool definitions when the registry has tools.
		if registry != nil && registry.Count() > 0 {
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
				toolResult := executeToolCall(request.Context(), registry, tc)
				allMessages = append(allMessages, toolResult)
				record.Messages = append(record.Messages, toolResult)
			}

			// Continue the loop: send tool results back to the LLM.
		}

		// Persist the full conversation.
		if lastStatusCode >= 200 && lastStatusCode < 300 {
			if _, err := store.Save(&record); err != nil {
				http.Error(response, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("X-Session-ID", sessionID)
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

	return mux
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

func resolveSessionID(request *http.Request, payload map[string]any, store *session.Store) string {
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
	if _, err := store.Get("bootstrap"); err == nil {
		return "bootstrap"
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

		entry["content"] = message.Content
		encoded = append(encoded, entry)
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

// executeToolCall runs a single tool call against the registry and returns
// a tool-result message ready to be appended to the conversation.
func executeToolCall(ctx context.Context, registry *tools.Registry, tc session.ToolCall) session.Message {
	tool, ok := registry.Get(tc.Function.Name)
	if !ok {
		return session.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Name:       tc.Function.Name,
			Content:    fmt.Sprintf("error: unknown tool %q", tc.Function.Name),
		}
	}

	result := tool.Execute(ctx, tc.Function.Arguments)

	content := result.Content
	if result.IsError {
		content = "error: " + content
	}

	return session.Message{
		Role:       "tool",
		ToolCallID: tc.ID,
		Name:       tc.Function.Name,
		Content:    content,
	}
}

func removeSessionMetadata(payload map[string]any) {
	if metadata, ok := payload["metadata"].(map[string]any); ok {
		delete(metadata, "session_id")
		if len(metadata) == 0 {
			delete(payload, "metadata")
		}
	}
}
