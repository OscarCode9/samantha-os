package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/providers/openaicodex"
	"github.com/oscarcode/elementary-claw/internal/session"
	"github.com/oscarcode/elementary-claw/internal/skills"
	"github.com/oscarcode/elementary-claw/internal/tools"
)

type codexLoopResult struct {
	SessionID    string
	Record       session.Record
	Message      session.Message
	Usage        *openaicodex.Usage
	FinishReason string
}

func handleOpenAICodexInit(response http.ResponseWriter, request *http.Request, paths config.Paths) {
	if request.Method != http.MethodPost {
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	init, err := openaicodex.BeginOAuth(paths)
	if err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(map[string]any{
		"ok":       true,
		"provider": "openai-codex",
		"model":    openaicodex.DefaultModel,
		"authUrl":  init.AuthURL,
		"state":    init.State,
	})
}

func handleOpenAICodexExchange(response http.ResponseWriter, request *http.Request, paths config.Paths) {
	if request.Method != http.MethodPost {
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		RawURL string `json:"raw_url"`
		Model  string `json:"model"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		http.Error(response, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	credential, err := openaicodex.ExchangeOAuth(request.Context(), paths, payload.RawURL)
	if err != nil {
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
		model = openaicodex.DefaultModel
	}

	cfg.Agent.Provider = "openai-codex"
	cfg.Agent.Model = model
	cfg.Agent.BaseURL = openaicodex.DefaultBaseURL
	cfg.Setup.ProviderPending = false

	if err := config.SaveFileConfig(paths, cfg); err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(map[string]any{
		"ok":        true,
		"provider":  cfg.Agent.Provider,
		"model":     cfg.Agent.Model,
		"baseUrl":   cfg.Agent.BaseURL,
		"accountId": credential.AccountID,
	})
}

func handleOpenAICodexChatCompletions(
	response http.ResponseWriter,
	request *http.Request,
	paths config.Paths,
	cfg config.FileConfig,
	payload map[string]any,
	store *session.Store,
	registry *tools.Registry,
	skillsRegistry *skills.Registry,
) {
	result, statusErr, err := runOpenAICodexLoop(request.Context(), paths, cfg, request, payload, store, registry, skillsRegistry, nil)
	if statusErr != nil {
		response.Header().Set("X-Claw-Provider", cfg.Agent.Provider)
		setProviderErrorHeaders(response.Header(), cfg.Agent.Provider, statusErr.StatusCode, statusErr.Body)
		http.Error(response, statusErr.Body, statusErr.StatusCode)
		return
	}
	if err != nil {
		http.Error(response, err.Error(), http.StatusBadGateway)
		return
	}

	if err := persistCodexLoopResult(paths, store, &result.Record, cfg, request.Header); err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("X-Session-ID", result.SessionID)
	response.Header().Set("X-Claw-Provider", cfg.Agent.Provider)
	_ = json.NewEncoder(response).Encode(buildChatCompletionResponse(result))
}

func handleOpenAICodexStreamingChatCompletions(
	response http.ResponseWriter,
	request *http.Request,
	paths config.Paths,
	cfg config.FileConfig,
	payload map[string]any,
	store *session.Store,
	registry *tools.Registry,
	skillsRegistry *skills.Registry,
) {
	livePreview, _ := payload["x_claw_preview"].(bool)
	responseStarted := false
	startResponse := func() {
		if responseStarted {
			return
		}
		writeStreamingHeaders(response, resolveSessionID(paths, request, payload, store))
		responseStarted = true
	}
	emitPreview := func(kind string, content string) {
		if !livePreview {
			return
		}
		startResponse()
		writePreviewEvent(response, kind, content)
	}

	previewWriter := newCodexPreviewWriter(emitPreview)
	var onPreview func(string, string)
	if livePreview {
		onPreview = previewWriter.Handle
	}

	result, statusErr, err := runOpenAICodexLoop(request.Context(), paths, cfg, request, payload, store, registry, skillsRegistry, onPreview)
	if statusErr != nil {
		if livePreview && responseStarted {
			emitPreview("error", decorateGatewayErrorMessage(cfg.Agent.Provider, statusErr.StatusCode, statusErr.Body))
			writeSSEDone(response)
			return
		}
		response.Header().Set("X-Claw-Provider", cfg.Agent.Provider)
		setProviderErrorHeaders(response.Header(), cfg.Agent.Provider, statusErr.StatusCode, statusErr.Body)
		http.Error(response, statusErr.Body, statusErr.StatusCode)
		return
	}
	if err != nil {
		if livePreview && responseStarted {
			emitPreview("error", err.Error())
			writeSSEDone(response)
			return
		}
		http.Error(response, err.Error(), http.StatusBadGateway)
		return
	}

	if err := persistCodexLoopResult(paths, store, &result.Record, cfg, request.Header); err != nil {
		if livePreview && responseStarted {
			emitPreview("error", err.Error())
			writeSSEDone(response)
			return
		}
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	if livePreview {
		startResponse()
		emitPreview("phase", "Respuesta lista")
		writeSSEDone(response)
		return
	}

	writeChatCompletionSSE(response, result)
}

func runOpenAICodexLoop(
	ctx context.Context,
	paths config.Paths,
	cfg config.FileConfig,
	request *http.Request,
	payload map[string]any,
	store *session.Store,
	registry *tools.Registry,
	skillsRegistry *skills.Registry,
	onPreview func(string, string),
) (codexLoopResult, *openaicodex.HTTPStatusError, error) {
	incomingMessages, err := decodeRequestMessages(payload["messages"])
	if err != nil {
		return codexLoopResult{}, nil, err
	}

	sessionID := resolveSessionID(paths, request, payload, store)
	record, err := loadOrCreateSession(store, sessionID)
	if err != nil {
		return codexLoopResult{}, nil, err
	}

	allMessages := append(cloneMessages(record.Messages), incomingMessages...)
	record.Messages = append(record.Messages, incomingMessages...)
	allMessages = prependSystemPrompt(paths, registry, skillsRegistry, allMessages)

	delete(payload, "session_id")
	removeSessionMetadata(payload)
	delete(payload, "x_claw_preview")
	normalizePayloadModel(cfg, payload)

	clientTools, _ := payload["x_client_tools"].(bool)
	delete(payload, "x_client_tools")
	if !clientTools && registry != nil && registry.Count() > 0 {
		payload["tools"] = registry.Definitions()
	}

	credential, _, err := openaicodex.ResolveCredential(ctx, paths)
	if err != nil {
		return codexLoopResult{}, nil, err
	}

	model, _ := payload["model"].(string)
	codexTools := extractCodexToolDefinitions(payload["tools"])

	for iteration := 0; iteration < maxToolCallIterations; iteration++ {
		if onPreview != nil && iteration > 0 {
			onPreview("phase", "Consultando modelo…")
		}

		previewState := &codexPreviewState{}
		var codexResponse openaicodex.Response
		codexResponse, err = openaicodex.StreamResponse(
			ctx,
			credential,
			model,
			allMessages,
			codexTools,
			func(event openaicodex.StreamEvent) {
				if onPreview == nil {
					return
				}
				previewState.Handle(event, onPreview)
			},
		)
		if err != nil {
			var httpStatusErr *openaicodex.HTTPStatusError
			if errors.As(err, &httpStatusErr) {
				return codexLoopResult{}, httpStatusErr, nil
			}
			return codexLoopResult{}, nil, err
		}

		assistantMessage := codexResponse.Message
		record.Messages = append(record.Messages, assistantMessage)
		allMessages = append(allMessages, assistantMessage)

		finishReason := "stop"
		if len(assistantMessage.ToolCalls) > 0 {
			finishReason = "tool_calls"
		}

		if clientTools || len(assistantMessage.ToolCalls) == 0 {
			return codexLoopResult{
				SessionID:    sessionID,
				Record:       record,
				Message:      assistantMessage,
				Usage:        codexResponse.Usage,
				FinishReason: finishReason,
			}, nil, nil
		}

		if registry == nil {
			return codexLoopResult{
				SessionID:    sessionID,
				Record:       record,
				Message:      assistantMessage,
				Usage:        codexResponse.Usage,
				FinishReason: finishReason,
			}, nil, nil
		}

		if onPreview != nil {
			onPreview("phase", "Usando herramientas…")
		}
		for _, toolCall := range assistantMessage.ToolCalls {
			if onPreview != nil {
				onPreview("tool", formatToolPreview(toolCall))
			}
			toolMessages := executeToolCallMessages(ctx, registry, toolCall)
			record.Messages = append(record.Messages, toolMessages...)
			allMessages = append(allMessages, toolMessages...)
			if onPreview != nil && len(toolMessages) > 0 {
				onPreview("tool", formatToolResultPreview(toolMessages[0]))
			}
		}
		if onPreview != nil {
			onPreview("phase", "Analizando resultados…")
		}
	}

	return codexLoopResult{}, nil, errors.New("OpenAI Codex alcanzó el límite de iteraciones")
}

func persistCodexLoopResult(paths config.Paths, store *session.Store, record *session.Record, cfg config.FileConfig, requestHeaders http.Header) error {
	if _, err := store.Save(record); err != nil {
		return err
	}
	checkBootstrapCompletion(paths, record.Messages)
	if strings.TrimSpace(cfg.Agent.Provider) != "openai-codex" && session.NeedsCompaction(record, session.DefaultCompactionThreshold) {
		target, err := resolveUpstreamTarget(paths, cfg)
		if err == nil {
			go triggerCompaction(paths, store, record, cfg, target, requestHeaders)
		}
	}
	return nil
}

func buildChatCompletionResponse(result codexLoopResult) map[string]any {
	message := map[string]any{
		"role": "assistant",
	}
	if strings.TrimSpace(result.Message.Content) != "" {
		message["content"] = result.Message.Content
	} else {
		message["content"] = nil
	}
	if len(result.Message.ToolCalls) > 0 {
		message["tool_calls"] = encodeToolCalls(result.Message.ToolCalls)
	}

	response := map[string]any{
		"id":      syntheticChatCompletionID(result),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"choices": []any{
			map[string]any{
				"index":         0,
				"message":       message,
				"finish_reason": result.FinishReason,
			},
		},
	}
	if result.Usage != nil {
		response["usage"] = map[string]any{
			"prompt_tokens":     result.Usage.InputTokens,
			"completion_tokens": result.Usage.OutputTokens,
			"total_tokens":      result.Usage.InputTokens + result.Usage.OutputTokens,
		}
	}
	return response
}

func writeChatCompletionSSE(response http.ResponseWriter, result codexLoopResult) {
	writeStreamingHeaders(response, result.SessionID)

	id := syntheticChatCompletionID(result)
	writeOpenAIChunk(response, map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"choices": []any{
			map[string]any{
				"index": 0,
				"delta": map[string]any{
					"role": "assistant",
				},
			},
		},
	})

	if strings.TrimSpace(result.Message.Content) != "" {
		writeOpenAIChunk(response, map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"choices": []any{
				map[string]any{
					"index": 0,
					"delta": map[string]any{
						"content": result.Message.Content,
					},
				},
			},
		})
	}

	if len(result.Message.ToolCalls) > 0 {
		for index, toolCall := range result.Message.ToolCalls {
			writeOpenAIChunk(response, map[string]any{
				"id":      id,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"choices": []any{
					map[string]any{
						"index": 0,
						"delta": map[string]any{
							"tool_calls": []any{
								map[string]any{
									"index": index,
									"id":    toolCall.ID,
									"type":  toolCall.Type,
									"function": map[string]any{
										"name":      toolCall.Function.Name,
										"arguments": toolCall.Function.Arguments,
									},
								},
							},
						},
					},
				},
			})
		}
	}

	writeOpenAIChunk(response, map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"choices": []any{
			map[string]any{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": result.FinishReason,
			},
		},
	})

	writeSSEDone(response)
}

func writeOpenAIChunk(writer io.Writer, payload map[string]any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	writeSSEEvent(writer, string(data))
}

func encodeToolCalls(toolCalls []session.ToolCall) []any {
	encoded := make([]any, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		encoded = append(encoded, map[string]any{
			"id":   toolCall.ID,
			"type": toolCall.Type,
			"function": map[string]any{
				"name":      toolCall.Function.Name,
				"arguments": toolCall.Function.Arguments,
			},
		})
	}
	return encoded
}

func syntheticChatCompletionID(result codexLoopResult) string {
	if id := strings.TrimSpace(result.SessionID); id != "" {
		return "chatcmpl-" + id
	}
	return fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
}

func extractCodexToolDefinitions(raw any) []openaicodex.ToolDefinition {
	switch value := raw.(type) {
	case nil:
		return nil
	case []tools.Definition:
		definitions := make([]openaicodex.ToolDefinition, 0, len(value))
		for _, definition := range value {
			definitions = append(definitions, openaicodex.ToolDefinition{
				Name:        definition.Function.Name,
				Description: definition.Function.Description,
				Parameters:  schemaToMap(definition.Function.Parameters),
			})
		}
		return definitions
	case []any:
		definitions := make([]openaicodex.ToolDefinition, 0, len(value))
		for _, item := range value {
			definitionMap, _ := item.(map[string]any)
			if definitionMap == nil {
				continue
			}
			functionMap, _ := definitionMap["function"].(map[string]any)
			if functionMap == nil {
				continue
			}
			parameters, _ := functionMap["parameters"].(map[string]any)
			definitions = append(definitions, openaicodex.ToolDefinition{
				Name:        stringValue(functionMap["name"]),
				Description: stringValue(functionMap["description"]),
				Parameters:  parameters,
			})
		}
		return definitions
	default:
		return nil
	}
}

func schemaToMap(schema tools.Schema) map[string]any {
	data, err := json.Marshal(schema)
	if err != nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	return payload
}

func stringValue(raw any) string {
	text, _ := raw.(string)
	return strings.TrimSpace(text)
}

type codexPreviewWriter struct {
	emit func(string, string)
}

func newCodexPreviewWriter(emit func(string, string)) *codexPreviewWriter {
	return &codexPreviewWriter{emit: emit}
}

func (writer *codexPreviewWriter) Handle(kind string, content string) {
	if writer == nil || writer.emit == nil {
		return
	}
	writer.emit(kind, content)
}

type codexPreviewState struct {
	previewBuilder strings.Builder
	sentDraftPhase bool
}

func (state *codexPreviewState) Handle(event openaicodex.StreamEvent, emit func(string, string)) {
	if emit == nil {
		return
	}
	switch event.Type {
	case "output_text_delta":
		if !state.sentDraftPhase {
			state.sentDraftPhase = true
			emit("phase", "Redactando respuesta…")
		}
		state.previewBuilder.WriteString(event.Content)
		emit("preview", state.previewBuilder.String())
	}
}
