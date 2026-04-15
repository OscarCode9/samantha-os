package runtime

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oscarcode/elementary-claw/internal/bootstrap"
	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/session"
	"github.com/oscarcode/elementary-claw/internal/tools"
)

func TestGetBootstrapSession(t *testing.T) {
	paths := makeTestPaths(t.TempDir())
	store := session.NewStore(paths)
	if err := config.InitializeWorkspace(paths, config.SetupOptions{
		WorkspaceDir:    paths.WorkspaceDir,
		UserName:        "oscar",
		PreferredName:   "Oscar",
		AssistantName:   "Vero",
		AssistantNature: "A local AI teammate",
		AssistantVibe:   "direct",
		Provider:        "copilot-proxy",
		ProviderBaseURL: "http://localhost:4000/openai/v1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := bootstrap.GenerateFirstMessage(paths, store, "hello from bootstrap"); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(newHandler(paths, store, nil, nil))
	defer server.Close()

	response, err := http.Get(server.URL + "/v1/sessions/bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("unexpected status %d: %s", response.StatusCode, string(body))
	}

	var record session.Record
	if err := json.NewDecoder(response.Body).Decode(&record); err != nil {
		t.Fatal(err)
	}
	if record.ID != "bootstrap" {
		t.Fatalf("unexpected session id: %s", record.ID)
	}
	if len(record.Messages) != 1 {
		t.Fatalf("unexpected message count: %d", len(record.Messages))
	}
}

func TestChatCompletionsPersistsBootstrapConversation(t *testing.T) {
	var upstreamMessages []map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/openai/v1/chat/completions" {
			response.WriteHeader(http.StatusNotFound)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		items, _ := payload["messages"].([]any)
		for _, item := range items {
			message, _ := item.(map[string]any)
			upstreamMessages = append(upstreamMessages, message)
		}

		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hello from upstream"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()

	paths := makeTestPaths(t.TempDir())
	store := session.NewStore(paths)
	if err := config.InitializeWorkspace(paths, config.SetupOptions{
		WorkspaceDir:    paths.WorkspaceDir,
		UserName:        "oscar",
		PreferredName:   "Oscar",
		AssistantName:   "Vero",
		AssistantNature: "A local AI teammate",
		AssistantVibe:   "direct",
		Provider:        "copilot-proxy",
		ProviderBaseURL: upstream.URL + "/openai/v1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := bootstrap.GenerateFirstMessage(paths, store, "hello from bootstrap"); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(newHandler(paths, store, nil, nil))
	defer server.Close()

	requestBody := strings.NewReader(`{"model":"gpt-4o","session_id":"bootstrap","messages":[{"role":"user","content":"who are you?"}]}`)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", requestBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("unexpected status %d: %s", response.StatusCode, string(body))
	}
	if response.Header.Get("X-Session-ID") != "bootstrap" {
		t.Fatalf("unexpected session header: %s", response.Header.Get("X-Session-ID"))
	}

	if len(upstreamMessages) != 2 {
		t.Fatalf("unexpected upstream message count: %d", len(upstreamMessages))
	}
	if upstreamMessages[0]["content"] != "hello from bootstrap" {
		t.Fatalf("unexpected bootstrap content sent upstream: %v", upstreamMessages[0]["content"])
	}
	if upstreamMessages[1]["content"] != "who are you?" {
		t.Fatalf("unexpected user content sent upstream: %v", upstreamMessages[1]["content"])
	}

	record, err := store.Get("bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Messages) != 3 {
		t.Fatalf("unexpected persisted message count: %d", len(record.Messages))
	}
	if record.Messages[2].Content != "hello from upstream" {
		t.Fatalf("unexpected assistant content persisted: %s", record.Messages[2].Content)
	}
}

// TestToolCallLoopExecutesToolsAndReturnsResult tests the full agentic loop:
// 1. Client sends a user message
// 2. LLM responds with a tool_call
// 3. Gateway executes the tool locally
// 4. Gateway sends tool result back to the LLM
// 5. LLM responds with final text
// 6. Gateway returns the final text to the client and persists everything
func TestToolCallLoopExecutesToolsAndReturnsResult(t *testing.T) {
	callCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/openai/v1/chat/completions" {
			response.WriteHeader(http.StatusNotFound)
			return
		}

		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		callCount++

		response.Header().Set("Content-Type", "application/json")

		if callCount == 1 {
			// First call: LLM requests a tool call
			_, _ = response.Write([]byte(`{
				"id": "chatcmpl-1",
				"object": "chat.completion",
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": null,
						"tool_calls": [{
							"id": "call_abc123",
							"type": "function",
							"function": {
								"name": "exec",
								"arguments": "{\"command\":\"echo tool-result-ok\"}"
							}
						}]
					},
					"finish_reason": "tool_calls"
				}]
			}`))
			return
		}

		// Second call: LLM receives tool result, returns final text.
		// Verify the tool result message is present.
		messages, _ := payload["messages"].([]any)
		foundToolResult := false
		for _, msg := range messages {
			m, _ := msg.(map[string]any)
			if m["role"] == "tool" && m["tool_call_id"] == "call_abc123" {
				content, _ := m["content"].(string)
				if strings.Contains(content, "tool-result-ok") {
					foundToolResult = true
				}
			}
		}
		if !foundToolResult {
			t.Errorf("expected tool result message with 'tool-result-ok' in second LLM call")
		}

		_, _ = response.Write([]byte(`{
			"id": "chatcmpl-2",
			"object": "chat.completion",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "The command output was tool-result-ok"
				},
				"finish_reason": "stop"
			}]
		}`))
	}))
	defer upstream.Close()

	paths := makeTestPaths(t.TempDir())
	store := session.NewStore(paths)
	if err := config.InitializeWorkspace(paths, config.SetupOptions{
		WorkspaceDir:    paths.WorkspaceDir,
		UserName:        "oscar",
		PreferredName:   "Oscar",
		AssistantName:   "Vero",
		AssistantNature: "A local AI teammate",
		AssistantVibe:   "direct",
		Provider:        "copilot-proxy",
		ProviderBaseURL: upstream.URL + "/openai/v1",
	}); err != nil {
		t.Fatal(err)
	}

	// Create a registry with the exec tool.
	registry := tools.NewRegistry()
	registry.Register(tools.NewExecTool(tools.ExecToolOptions{}))

	server := httptest.NewServer(newHandler(paths, store, registry, nil))
	defer server.Close()

	requestBody := strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"run echo"}]}`)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", requestBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	// Verify the LLM was called twice (tool_call + final).
	if callCount != 2 {
		t.Fatalf("expected 2 upstream calls, got %d", callCount)
	}

	// Verify the response body is the final text.
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	choices, _ := result["choices"].([]any)
	if len(choices) == 0 {
		t.Fatal("expected at least one choice in response")
	}
	choice, _ := choices[0].(map[string]any)
	msg, _ := choice["message"].(map[string]any)
	content, _ := msg["content"].(string)
	if !strings.Contains(content, "tool-result-ok") {
		t.Fatalf("expected final response to contain 'tool-result-ok': %s", content)
	}

	// Verify session persistence: should have user + assistant(tool_call) + tool + assistant(final) = 4 messages.
	sessionID := resp.Header.Get("X-Session-ID")
	record, err := store.Get(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Messages) != 4 {
		t.Fatalf("expected 4 persisted messages, got %d", len(record.Messages))
	}
	if record.Messages[0].Role != "user" {
		t.Fatalf("expected first message to be user, got %s", record.Messages[0].Role)
	}
	if record.Messages[1].Role != "assistant" || len(record.Messages[1].ToolCalls) == 0 {
		t.Fatalf("expected second message to be assistant with tool_calls")
	}
	if record.Messages[2].Role != "tool" || record.Messages[2].ToolCallID != "call_abc123" {
		t.Fatalf("expected third message to be tool result with call_abc123")
	}
	if record.Messages[3].Role != "assistant" || record.Messages[3].Content == "" {
		t.Fatalf("expected fourth message to be assistant with text content")
	}
}

// TestToolCallLoopUnknownTool verifies that unknown tool names produce an error
// result rather than crashing the gateway.
func TestToolCallLoopUnknownTool(t *testing.T) {
	callCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/openai/v1/chat/completions" {
			response.WriteHeader(http.StatusNotFound)
			return
		}
		callCount++

		response.Header().Set("Content-Type", "application/json")

		if callCount == 1 {
			_, _ = response.Write([]byte(`{
				"id": "chatcmpl-1",
				"choices": [{
					"message": {
						"role": "assistant",
						"content": null,
						"tool_calls": [{
							"id": "call_unknown",
							"type": "function",
							"function": {
								"name": "nonexistent_tool",
								"arguments": "{}"
							}
						}]
					},
					"finish_reason": "tool_calls"
				}]
			}`))
			return
		}

		// Second call: LLM sees the error, responds with text.
		_, _ = response.Write([]byte(`{
			"id": "chatcmpl-2",
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Sorry, that tool is not available."
				},
				"finish_reason": "stop"
			}]
		}`))
	}))
	defer upstream.Close()

	paths := makeTestPaths(t.TempDir())
	store := session.NewStore(paths)
	if err := config.InitializeWorkspace(paths, config.SetupOptions{
		WorkspaceDir:    paths.WorkspaceDir,
		UserName:        "oscar",
		PreferredName:   "Oscar",
		AssistantName:   "Vero",
		AssistantNature: "A local AI teammate",
		AssistantVibe:   "direct",
		Provider:        "copilot-proxy",
		ProviderBaseURL: upstream.URL + "/openai/v1",
	}); err != nil {
		t.Fatal(err)
	}

	registry := tools.NewRegistry()
	registry.Register(tools.NewExecTool(tools.ExecToolOptions{}))

	server := httptest.NewServer(newHandler(paths, store, registry, nil))
	defer server.Close()

	requestBody := strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"use nonexistent tool"}]}`)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", requestBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	if callCount != 2 {
		t.Fatalf("expected 2 upstream calls, got %d", callCount)
	}
}

// TestToolDefinitionsSentToUpstream verifies that tool definitions are included
// in the payload sent to the LLM when the registry has tools.
func TestToolDefinitionsSentToUpstream(t *testing.T) {
	var receivedTools []any
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/openai/v1/chat/completions" {
			response.WriteHeader(http.StatusNotFound)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		receivedTools, _ = payload["tools"].([]any)

		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()

	paths := makeTestPaths(t.TempDir())
	store := session.NewStore(paths)
	if err := config.InitializeWorkspace(paths, config.SetupOptions{
		WorkspaceDir:    paths.WorkspaceDir,
		UserName:        "oscar",
		PreferredName:   "Oscar",
		AssistantName:   "Vero",
		AssistantNature: "A local AI teammate",
		AssistantVibe:   "direct",
		Provider:        "copilot-proxy",
		ProviderBaseURL: upstream.URL + "/openai/v1",
	}); err != nil {
		t.Fatal(err)
	}

	registry := tools.NewRegistry()
	registry.Register(tools.NewExecTool(tools.ExecToolOptions{}))
	registry.Register(tools.NewReadFileTool(t.TempDir()))

	server := httptest.NewServer(newHandler(paths, store, registry, nil))
	defer server.Close()

	requestBody := strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", requestBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	if len(receivedTools) != 2 {
		t.Fatalf("expected 2 tool definitions sent upstream, got %d", len(receivedTools))
	}
}

func makeTestPaths(root string) config.Paths {
	stateDir := filepath.Join(root, ".openclaw")
	workspaceDir := filepath.Join(stateDir, "workspace")
	credentialsDir := filepath.Join(stateDir, "state", "credentials")

	return config.Paths{
		HomeDir:               root,
		StateDir:              stateDir,
		WorkspaceDir:          workspaceDir,
		CredentialsDir:        credentialsDir,
		SessionsDir:           filepath.Join(stateDir, "state", "sessions"),
		ConfigPath:            filepath.Join(stateDir, "openclaw.json"),
		AuthPath:              filepath.Join(stateDir, "agents", "main", "agent", "auth-profiles.json"),
		CopilotTokenCachePath: filepath.Join(credentialsDir, "github-copilot.token.json"),
		AgentPath:             filepath.Join(workspaceDir, "AGENTS.md"),
		IdentityPath:          filepath.Join(workspaceDir, "IDENTITY.md"),
		SoulPath:              filepath.Join(workspaceDir, "SOUL.md"),
		UserPath:              filepath.Join(workspaceDir, "USER.md"),
		ToolsPath:             filepath.Join(workspaceDir, "TOOLS.md"),
		HeartbeatPath:         filepath.Join(workspaceDir, "HEARTBEAT.md"),
		BootstrapPath:         filepath.Join(workspaceDir, "BOOTSTRAP.md"),
	}
}
