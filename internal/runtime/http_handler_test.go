package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oscarcode/elementary-claw/internal/bootstrap"
	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/providers/openai"
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

	if len(upstreamMessages) != 3 {
		t.Fatalf("unexpected upstream message count: %d", len(upstreamMessages))
	}
	// First message is the injected system prompt.
	if upstreamMessages[0]["role"] != "system" {
		t.Fatalf("expected system message first, got role=%v", upstreamMessages[0]["role"])
	}
	if upstreamMessages[1]["content"] != "hello from bootstrap" {
		t.Fatalf("unexpected bootstrap content sent upstream: %v", upstreamMessages[1]["content"])
	}
	if upstreamMessages[2]["content"] != "who are you?" {
		t.Fatalf("unexpected user content sent upstream: %v", upstreamMessages[2]["content"])
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

func TestResolveSessionIDIgnoresBootstrapSessionAfterBootstrapComplete(t *testing.T) {
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
	if err := os.Remove(paths.BootstrapPath); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if sessionID := resolveSessionID(paths, request, map[string]any{}, store); sessionID != "default" {
		t.Fatalf("expected default session after bootstrap completion, got %q", sessionID)
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

func TestToolImageAttachmentsForwardedAsMultimodalUserMessage(t *testing.T) {
	imagePath := filepath.Join(t.TempDir(), "screenshot.png")
	if err := os.WriteFile(imagePath, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, 0o600); err != nil {
		t.Fatal(err)
	}

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
			_, _ = response.Write([]byte(`{
				"id": "chatcmpl-vision-1",
				"choices": [{
					"message": {
						"role": "assistant",
						"content": null,
						"tool_calls": [{
							"id": "call_screenshot",
							"type": "function",
							"function": {
								"name": "fake_screenshot",
								"arguments": "{}"
							}
						}]
					},
					"finish_reason": "tool_calls"
				}]
			}`))
			return
		}

		messages, _ := payload["messages"].([]any)
		foundToolResult := false
		foundImage := false
		for _, rawMessage := range messages {
			message, _ := rawMessage.(map[string]any)
			if message["role"] == "tool" && message["tool_call_id"] == "call_screenshot" {
				content, _ := message["content"].(string)
				if strings.Contains(content, `"ok":true`) {
					foundToolResult = true
				}
			}
			if message["role"] != "user" {
				continue
			}
			parts, ok := message["content"].([]any)
			if !ok {
				continue
			}
			for _, rawPart := range parts {
				part, _ := rawPart.(map[string]any)
				if part["type"] != "image_url" {
					continue
				}
				imageURL, _ := part["image_url"].(map[string]any)
				url, _ := imageURL["url"].(string)
				if strings.HasPrefix(url, "data:image/png;base64,") {
					foundImage = true
				}
			}
		}
		if !foundToolResult {
			t.Errorf("expected tool result in second upstream payload")
		}
		if !foundImage {
			t.Errorf("expected screenshot attachment as image_url content part")
		}

		_, _ = response.Write([]byte(`{
			"id": "chatcmpl-vision-2",
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "I can see the screenshot now."
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
	registry.Register(staticRuntimeTool{
		name:        "fake_screenshot",
		description: "returns a screenshot attachment",
		result: tools.Result{
			Content: `{"ok":true}`,
			Attachments: []tools.Attachment{{
				Type:     "image",
				Path:     imagePath,
				MimeType: "image/png",
			}},
		},
	})

	server := httptest.NewServer(newHandler(paths, store, registry, nil))
	defer server.Close()

	requestBody := strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"take a screenshot"}]}`)
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

func TestStreamingPreviewEmitsPhasesToolsAndPreview(t *testing.T) {
	callCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/openai/v1/chat/completions" {
			response.WriteHeader(http.StatusNotFound)
			return
		}
		callCount++

		response.Header().Set("Content-Type", "text/event-stream")
		if callCount == 1 {
			_, _ = io.WriteString(response, strings.Join([]string{
				`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
				"",
				`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_exec_1","type":"function","function":{"name":"exec","arguments":"{\"command\":\"printf hola\"}"}}]}}]}`,
				"",
				`data: {"choices":[{"index":0,"finish_reason":"tool_calls","delta":{}}]}`,
				"",
				`data: [DONE]`,
				"",
			}, "\n"))
			return
		}

		_, _ = io.WriteString(response, strings.Join([]string{
			`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
			"",
			`data: {"choices":[{"index":0,"delta":{"content":"Hola"}}]}`,
			"",
			`data: {"choices":[{"index":0,"delta":{"content":" mundo"}}]}`,
			"",
			`data: {"choices":[{"index":0,"finish_reason":"stop","delta":{}}]}`,
			"",
			`data: [DONE]`,
			"",
		}, "\n"))
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

	body := `{"model":"gpt-4o","stream":true,"x_claw_preview":true,"messages":[{"role":"user","content":"di hola"}]}`
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(data))
	}

	streamBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(streamBody)

	if !strings.Contains(text, `"x_claw_event":"phase"`) {
		t.Fatalf("expected preview phase events in stream, got: %s", text)
	}
	if !strings.Contains(text, `Usando herramientas`) {
		t.Fatalf("expected tool phase in stream, got: %s", text)
	}
	if !strings.Contains(text, `"x_claw_event":"tool"`) || !strings.Contains(text, `exec(`) {
		t.Fatalf("expected tool preview event in stream, got: %s", text)
	}
	if !strings.Contains(text, `"x_claw_event":"preview"`) || !strings.Contains(text, `Hola mundo`) {
		t.Fatalf("expected response preview event in stream, got: %s", text)
	}
	if !strings.Contains(text, `data: [DONE]`) {
		t.Fatalf("expected SSE done sentinel, got: %s", text)
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

func TestNoArgToolSchemasKeepEmptyPropertiesUpstream(t *testing.T) {
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
	registry.Register(tools.NewGetBatteryStatusTool())

	server := httptest.NewServer(newHandler(paths, store, registry, nil))
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	if len(receivedTools) != 1 {
		t.Fatalf("expected 1 tool definition sent upstream, got %d", len(receivedTools))
	}

	toolDef, ok := receivedTools[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected tool definition type: %T", receivedTools[0])
	}
	fn, ok := toolDef["function"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected function payload type: %T", toolDef["function"])
	}
	params, ok := fn["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected parameters payload type: %T", fn["parameters"])
	}
	properties, present := params["properties"]
	if !present {
		t.Fatalf("expected parameters.properties in upstream payload: %#v", params)
	}
	propertyMap, ok := properties.(map[string]any)
	if !ok {
		t.Fatalf("expected parameters.properties to decode as an object, got %T", properties)
	}
	if len(propertyMap) != 0 {
		t.Fatalf("expected empty properties object for no-arg tool, got %#v", propertyMap)
	}
}

// TestClientToolsPassthrough verifies that when x_client_tools=true is set:
//   - Gateway does NOT inject registry tool definitions into the upstream payload.
//   - The raw tool_calls response is returned to the caller as-is (not executed).
func TestClientToolsPassthrough(t *testing.T) {
	var receivedPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/openai/v1/chat/completions" {
			response.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&receivedPayload); err != nil {
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		// Return a tool_calls response to verify it reaches the client unchanged.
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"id":"chatcmpl-xt","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"tc1","type":"function","function":{"name":"my_tool","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`))
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

	// Registry with tools — these should NOT be injected when x_client_tools=true.
	registry := tools.NewRegistry()
	registry.Register(tools.NewExecTool(tools.ExecToolOptions{}))

	server := httptest.NewServer(newHandler(paths, store, registry, nil))
	defer server.Close()

	// Client sends its own tool definition alongside x_client_tools=true.
	body := `{
		"model": "gpt-4o",
		"x_client_tools": true,
		"tools": [{"type":"function","function":{"name":"my_tool","description":"custom"}}],
		"tool_choice": "auto",
		"messages": [{"role":"user","content":"call my tool"}]
	}`
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(b))
	}

	// The upstream must NOT have received the registry's exec tool — only the
	// client-supplied tool definition.
	if upstreamTools, ok := receivedPayload["tools"].([]any); ok {
		if len(upstreamTools) != 1 {
			t.Fatalf("expected 1 client tool upstream, got %d", len(upstreamTools))
		}
		toolDef := upstreamTools[0].(map[string]any)
		fn := toolDef["function"].(map[string]any)
		if fn["name"] != "my_tool" {
			t.Fatalf("expected client tool 'my_tool' upstream, got %q", fn["name"])
		}
	} else {
		t.Fatal("upstream payload missing tools field")
	}

	// x_client_tools must have been stripped before forwarding.
	if _, present := receivedPayload["x_client_tools"]; present {
		t.Fatal("x_client_tools must be stripped before sending to upstream")
	}

	// The raw tool_calls response must be returned to the caller.
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	choices := result["choices"].([]any)
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	if _, hasToolCalls := msg["tool_calls"]; !hasToolCalls {
		t.Fatal("expected tool_calls in response to client, gateway must not have executed them")
	}
	if choice["finish_reason"] != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, got %q", choice["finish_reason"])
	}
}

func TestRuntimeEndpointReportsProviderState(t *testing.T) {
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

	server := httptest.NewServer(newHandler(paths, store, nil, nil))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/runtime")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}

	if got := payload["provider"]; got != "copilot-proxy" {
		t.Fatalf("unexpected provider: %v", got)
	}
	if got := payload["baseUrl"]; got != "http://localhost:4000/openai/v1" {
		t.Fatalf("unexpected base URL: %v", got)
	}
	if got := payload["source"]; got != "config" {
		t.Fatalf("unexpected source: %v", got)
	}
	if got, _ := payload["authConfigured"].(bool); got {
		t.Fatal("expected authConfigured=false for copilot-proxy")
	}
}

func TestNormalizeProviderModelStripsGitHubCopilotPrefix(t *testing.T) {
	if got := normalizeProviderModel("github-copilot", "github-copilot/gpt-5.4"); got != "gpt-5.4" {
		t.Fatalf("unexpected normalized GitHub Copilot model: %q", got)
	}
	if got := normalizeProviderModel("openai", "github-copilot/gpt-5.4"); got != "github-copilot/gpt-5.4" {
		t.Fatalf("unexpected normalized OpenAI model: %q", got)
	}
}

func TestOpenAIConnectEndpointStoresAPIKeyAndSwitchesProvider(t *testing.T) {
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
		ProviderPending: true,
	}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(newHandler(paths, store, nil, nil))
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/providers/openai/connect", bytes.NewBufferString(`{"api_key":"sk-openai-test"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if got := payload["provider"]; got != "openai" {
		t.Fatalf("unexpected provider payload: %v", got)
	}
	if got := payload["model"]; got != openai.DefaultCodexModel {
		t.Fatalf("unexpected model payload: %v", got)
	}
	if got := payload["baseUrl"]; got != openai.DefaultAPIBaseURL {
		t.Fatalf("unexpected base URL payload: %v", got)
	}

	cfg, err := config.LoadFileConfig(paths)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.Provider != "openai" {
		t.Fatalf("unexpected provider in config: %s", cfg.Agent.Provider)
	}
	if cfg.Agent.Model != openai.DefaultCodexModel {
		t.Fatalf("unexpected model in config: %s", cfg.Agent.Model)
	}
	if cfg.Agent.BaseURL != openai.DefaultAPIBaseURL {
		t.Fatalf("unexpected base URL in config: %s", cfg.Agent.BaseURL)
	}
	if cfg.Setup.ProviderPending {
		t.Fatal("expected provider pending to be cleared")
	}

	authData, err := os.ReadFile(paths.AuthPath)
	if err != nil {
		t.Fatal(err)
	}
	authText := string(authData)
	if !strings.Contains(authText, `"openai:default"`) {
		t.Fatalf("expected openai profile in auth store: %s", authText)
	}
	if !strings.Contains(authText, `"token": "sk-openai-test"`) {
		t.Fatalf("expected OpenAI key in auth store: %s", authText)
	}
}

func TestChatCompletionsGitHubRateLimitSetsClassificationHeader(t *testing.T) {
	paths := makeTestPaths(t.TempDir())
	store := session.NewStore(paths)
	if err := config.InitializeWorkspace(paths, config.SetupOptions{
		WorkspaceDir:    paths.WorkspaceDir,
		UserName:        "oscar",
		PreferredName:   "Oscar",
		AssistantName:   "Vero",
		AssistantNature: "A local AI teammate",
		AssistantVibe:   "direct",
		Provider:        "github-copilot",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.CopilotTokenCachePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.CopilotTokenCachePath, []byte("{\n  \"token\": \"cached-token\",\n  \"expiresAt\": 4102444800000,\n  \"updatedAt\": 1700000000000\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_TOKEN", "ghu-test-token")

	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Host != "api.individual.githubcopilot.com" {
				t.Fatalf("unexpected upstream host: %s", request.URL.Host)
			}
			if request.URL.Path != "/chat/completions" {
				t.Fatalf("unexpected upstream path: %s", request.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`Sorry, you've exceeded your 5 hour session limits.`,
				)),
			}, nil
		}),
	}
	defer func() {
		http.DefaultClient = originalClient
	}()

	server := httptest.NewServer(newHandler(paths, store, nil, nil))
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hola"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("X-Claw-Error-Class"); got != "github-rate-limit" {
		t.Fatalf("unexpected error class: %s", got)
	}
	if got := resp.Header.Get("X-Claw-Provider"); got != "github-copilot" {
		t.Fatalf("unexpected provider header: %s", got)
	}
}

func TestStreamingPreviewGitHubRateLimitEmitsOpenAICodexOffer(t *testing.T) {
	paths := makeTestPaths(t.TempDir())
	store := session.NewStore(paths)
	if err := config.InitializeWorkspace(paths, config.SetupOptions{
		WorkspaceDir:    paths.WorkspaceDir,
		UserName:        "oscar",
		PreferredName:   "Oscar",
		AssistantName:   "Vero",
		AssistantNature: "A local AI teammate",
		AssistantVibe:   "direct",
		Provider:        "github-copilot",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.CopilotTokenCachePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.CopilotTokenCachePath, []byte("{\n  \"token\": \"cached-token\",\n  \"expiresAt\": 4102444800000,\n  \"updatedAt\": 1700000000000\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_TOKEN", "ghu-test-token")

	registry := tools.NewRegistry()
	registry.Register(tools.NewExecTool(tools.ExecToolOptions{}))

	callCount := 0
	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Host != "api.individual.githubcopilot.com" {
				t.Fatalf("unexpected upstream host: %s", request.URL.Host)
			}
			if request.URL.Path != "/chat/completions" {
				t.Fatalf("unexpected upstream path: %s", request.URL.Path)
			}

			callCount++
			if callCount == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
					Body: io.NopCloser(strings.NewReader(strings.Join([]string{
						`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
						"",
						`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_exec_1","type":"function","function":{"name":"exec","arguments":"{\"command\":\"printf hola\"}"}}]}}]}`,
						"",
						`data: {"choices":[{"index":0,"finish_reason":"tool_calls","delta":{}}]}`,
						"",
						`data: [DONE]`,
						"",
					}, "\n"))),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`Sorry, you've exceeded your 5 hour session limits.`,
				)),
			}, nil
		}),
	}
	defer func() {
		http.DefaultClient = originalClient
	}()

	server := httptest.NewServer(newHandler(paths, store, registry, nil))
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o","stream":true,"x_claw_preview":true,"messages":[{"role":"user","content":"hola"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	streamBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	text := string(streamBody)
	if !strings.Contains(text, `"x_claw_event":"error"`) {
		t.Fatalf("expected preview error event in stream, got: %s", text)
	}
	if !strings.Contains(text, openAICodexOfferPrefix) {
		t.Fatalf("expected OpenAI Codex offer prefix in stream, got: %s", text)
	}
	if !strings.Contains(text, `5 hour session limits`) {
		t.Fatalf("expected original GitHub limit detail in stream, got: %s", text)
	}
	if !strings.Contains(text, `data: [DONE]`) {
		t.Fatalf("expected SSE done sentinel, got: %s", text)
	}
}

func makeTestPaths(root string) config.Paths {
	stateDir := filepath.Join(root, ".samantha")
	workspaceDir := filepath.Join(stateDir, "workspace")
	credentialsDir := filepath.Join(stateDir, "state", "credentials")

	return config.Paths{
		HomeDir:               root,
		StateDir:              stateDir,
		WorkspaceDir:          workspaceDir,
		CredentialsDir:        credentialsDir,
		SessionsDir:           filepath.Join(stateDir, "state", "sessions"),
		ConfigPath:            filepath.Join(stateDir, "samantha.json"),
		AuthPath:              filepath.Join(stateDir, "agents", "main", "agent", "auth-profiles.json"),
		CopilotTokenCachePath: filepath.Join(credentialsDir, "github-copilot.token.json"),
		AgentPath:             filepath.Join(workspaceDir, "AGENTS.md"),
		IdentityPath:          filepath.Join(workspaceDir, "IDENTITY.md"),
		SoulPath:              filepath.Join(workspaceDir, "SOUL.md"),
		UserPath:              filepath.Join(workspaceDir, "USER.md"),
		ToolsPath:             filepath.Join(workspaceDir, "TOOLS.md"),
		HeartbeatPath:         filepath.Join(workspaceDir, "HEARTBEAT.md"),
		BootstrapPath:         filepath.Join(workspaceDir, "BOOTSTRAP.md"),
		BundledSkillsDir:      filepath.Join(stateDir, "skills", "bundled"),
		ManagedSkillsDir:      filepath.Join(stateDir, "skills", "managed"),
		WorkspaceSkillsDir:    filepath.Join(workspaceDir, "skills"),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type staticRuntimeTool struct {
	name        string
	description string
	result      tools.Result
}

func (tool staticRuntimeTool) Name() string {
	return tool.name
}

func (tool staticRuntimeTool) Description() string {
	return tool.description
}

func (tool staticRuntimeTool) Parameters() tools.Schema {
	return tools.Schema{Type: "object", Properties: map[string]tools.SchemaProperty{}}
}

func (tool staticRuntimeTool) Execute(context.Context, string) tools.Result {
	return tool.result
}
