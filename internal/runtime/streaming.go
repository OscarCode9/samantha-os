package runtime

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/session"
	"github.com/oscarcode/elementary-claw/internal/skills"
	"github.com/oscarcode/elementary-claw/internal/tools"
)

// sseChunk represents a single parsed SSE chunk from a streaming chat
// completions response.
type sseChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Delta        struct {
			Role      string `json:"role,omitempty"`
			Content   string `json:"content,omitempty"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id,omitempty"`
				Type     string `json:"type,omitempty"`
				Function struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta"`
	} `json:"choices"`
}

// streamAccumulator collects streamed delta chunks into a complete assistant
// message and finish reason. It also stores the raw SSE lines so they can be
// replayed to the client.
type streamAccumulator struct {
	role         string
	content      strings.Builder
	toolCalls    map[int]*toolCallAccumulator // index -> accumulator
	finishReason string
	rawLines     []string // original "data: ..." lines including "data: [DONE]"
}

type toolCallAccumulator struct {
	id       string
	typ      string
	funcName strings.Builder
	funcArgs strings.Builder
}

type previewEvent struct {
	Event   string `json:"x_claw_event"`
	Content string `json:"content,omitempty"`
}

func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{
		toolCalls: make(map[int]*toolCallAccumulator),
	}
}

// addChunk processes a single parsed SSE chunk.
func (a *streamAccumulator) addChunk(chunk sseChunk) {
	if len(chunk.Choices) == 0 {
		return
	}

	choice := chunk.Choices[0]
	delta := choice.Delta

	if delta.Role != "" {
		a.role = delta.Role
	}
	if delta.Content != "" {
		a.content.WriteString(delta.Content)
	}
	if choice.FinishReason != "" {
		a.finishReason = choice.FinishReason
	}

	for _, tc := range delta.ToolCalls {
		acc, ok := a.toolCalls[tc.Index]
		if !ok {
			acc = &toolCallAccumulator{}
			a.toolCalls[tc.Index] = acc
		}
		if tc.ID != "" {
			acc.id = tc.ID
		}
		if tc.Type != "" {
			acc.typ = tc.Type
		}
		if tc.Function.Name != "" {
			acc.funcName.WriteString(tc.Function.Name)
		}
		if tc.Function.Arguments != "" {
			acc.funcArgs.WriteString(tc.Function.Arguments)
		}
	}
}

// message returns the accumulated assistant message.
func (a *streamAccumulator) message() session.Message {
	role := a.role
	if role == "" {
		role = "assistant"
	}

	msg := session.Message{
		Role:    role,
		Content: a.content.String(),
	}

	if len(a.toolCalls) > 0 {
		// Build tool calls in index order.
		maxIdx := -1
		for idx := range a.toolCalls {
			if idx > maxIdx {
				maxIdx = idx
			}
		}
		for i := 0; i <= maxIdx; i++ {
			acc, ok := a.toolCalls[i]
			if !ok {
				continue
			}
			typ := acc.typ
			if typ == "" {
				typ = "function"
			}
			msg.ToolCalls = append(msg.ToolCalls, session.ToolCall{
				ID:   acc.id,
				Type: typ,
				Function: session.FunctionCall{
					Name:      acc.funcName.String(),
					Arguments: acc.funcArgs.String(),
				},
			})
		}
	}

	return msg
}

// hasToolCalls returns true if any tool calls were accumulated.
func (a *streamAccumulator) hasToolCalls() bool {
	return len(a.toolCalls) > 0
}

// consumeSSEStream reads an SSE stream from the given reader, parses each
// chunk, and feeds it to the accumulator. The raw SSE lines are stored for
// later replay. Returns the accumulated message and finish reason.
func consumeSSEStream(reader io.Reader, onChunk func(*streamAccumulator, sseChunk)) (*streamAccumulator, error) {
	acc := newStreamAccumulator()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Store all lines for replay.
		acc.rawLines = append(acc.rawLines, line)

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			continue
		}

		var chunk sseChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// Skip unparseable chunks.
			continue
		}

		acc.addChunk(chunk)
		if onChunk != nil {
			onChunk(acc, chunk)
		}
	}

	if err := scanner.Err(); err != nil {
		return acc, fmt.Errorf("read SSE stream: %w", err)
	}

	return acc, nil
}

// replaySSELines writes stored SSE lines to a writer, flushing after each
// line if the writer supports it. This is used to forward the final streaming
// response to the client.
func replaySSELines(w io.Writer, lines []string) {
	flusher, canFlush := w.(interface{ Flush() })

	for _, line := range lines {
		fmt.Fprint(w, line+"\n")
		if canFlush {
			flusher.Flush()
		}
	}
}

// writeSSEEvent writes a single SSE data event and flushes.
func writeSSEEvent(w io.Writer, data string) {
	fmt.Fprintf(w, "data: %s\n\n", data)
	if flusher, ok := w.(interface{ Flush() }); ok {
		flusher.Flush()
	}
}

func writePreviewEvent(w io.Writer, kind string, content string) {
	payload, err := json.Marshal(previewEvent{
		Event:   kind,
		Content: content,
	})
	if err != nil {
		return
	}
	writeSSEEvent(w, string(payload))
}

// writeSSEDone writes the SSE [DONE] sentinel and flushes.
func writeSSEDone(w io.Writer) {
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher, ok := w.(interface{ Flush() }); ok {
		flusher.Flush()
	}
}

func writeStreamingHeaders(response http.ResponseWriter, sessionID string) {
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache")
	response.Header().Set("X-Accel-Buffering", "no")
	response.Header().Set("X-Session-ID", sessionID)
	response.WriteHeader(http.StatusOK)
}

func summarizeToolCallArguments(arguments string) string {
	if strings.TrimSpace(arguments) == "" {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil || len(payload) == 0 {
		return ""
	}

	keys := []string{"path", "pattern", "command", "url", "query", "message", "title", "name", "job_id", "uid"}
	parts := make([]string, 0, 2)

	appendValue := func(key string, value any) {
		if len(parts) >= 2 {
			return
		}
		rendered := summarizePreviewValue(value)
		if rendered == "" {
			return
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, rendered))
	}

	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		appendValue(key, value)
	}

	if len(parts) == 0 {
		fallbackKeys := make([]string, 0, len(payload))
		for key := range payload {
			fallbackKeys = append(fallbackKeys, key)
		}
		sort.Strings(fallbackKeys)
		for _, key := range fallbackKeys {
			appendValue(key, payload[key])
		}
	}

	return strings.Join(parts, ", ")
}

func summarizePreviewValue(value any) string {
	switch v := value.(type) {
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return ""
		}
		if len(text) > 48 {
			text = text[:45] + "..."
		}
		return strconv.Quote(text)
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(v, 'f', -1, 64))
	case bool:
		return strconv.FormatBool(v)
	case []any:
		return fmt.Sprintf("%d items", len(v))
	case map[string]any:
		return fmt.Sprintf("%d fields", len(v))
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func formatToolPreview(tc session.ToolCall) string {
	name := strings.TrimSpace(tc.Function.Name)
	if name == "" {
		name = "tool"
	}

	summary := summarizeToolCallArguments(tc.Function.Arguments)
	if summary == "" {
		return fmt.Sprintf("%s()", name)
	}
	return fmt.Sprintf("%s(%s)", name, summary)
}

func formatToolResultPreview(msg session.Message) string {
	name := strings.TrimSpace(msg.Name)
	if name == "" {
		name = "tool"
	}

	content := strings.TrimSpace(msg.Content)
	if strings.HasPrefix(content, "error:") {
		detail := strings.TrimSpace(strings.TrimPrefix(content, "error:"))
		if len(detail) > 72 {
			detail = detail[:69] + "..."
		}
		return fmt.Sprintf("%s failed: %s", name, detail)
	}

	return fmt.Sprintf("%s finished", name)
}

// handleStreamingChatCompletions runs the tool-call loop with a streaming
// upstream and forwards SSE events from the final (non-tool-call) response
// directly to the client.
func handleStreamingChatCompletions(
	response http.ResponseWriter,
	request *http.Request,
	paths config.Paths,
	cfg config.FileConfig,
	target upstreamTarget,
	payload map[string]any,
	store *session.Store,
	registry *tools.Registry,
	skillsRegistry *skills.Registry,
) {
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

	allMessages := append(cloneMessages(record.Messages), incomingMessages...)
	record.Messages = append(record.Messages, incomingMessages...)

	// Inject system prompt at the front if not already present.
	allMessages = prependSystemPrompt(paths, registry, skillsRegistry, allMessages)

	delete(payload, "session_id")
	removeSessionMetadata(payload)
	livePreview, _ := payload["x_claw_preview"].(bool)
	delete(payload, "x_claw_preview")
	normalizePayloadModel(cfg, payload)

	if registry != nil && registry.Count() > 0 {
		payload["tools"] = registry.Definitions()
	}
	// Always request streaming from upstream.
	payload["stream"] = true

	upstreamURL := buildUpstreamURL(target, request.URL.Path)
	if trimmedQuery := strings.TrimSpace(request.URL.RawQuery); trimmedQuery != "" {
		upstreamURL += "?" + trimmedQuery
	}

	responseStarted := false
	startResponse := func() {
		if responseStarted {
			return
		}
		writeStreamingHeaders(response, sessionID)
		responseStarted = true
	}
	emitPreview := func(kind string, content string) {
		if !livePreview {
			return
		}
		startResponse()
		writePreviewEvent(response, kind, content)
	}

	for iteration := 0; iteration < maxToolCallIterations; iteration++ {
		if livePreview && responseStarted {
			emitPreview("phase", "Consultando modelo…")
		}
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
		proxyRequest.Header.Set("Accept", "text/event-stream")
		applyTargetHeaders(proxyRequest.Header, target.Headers)

		proxyResponse, err := http.DefaultClient.Do(proxyRequest)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadGateway)
			return
		}

		// Non-2xx: forward the error body as plain JSON and stop.
		if proxyResponse.StatusCode < 200 || proxyResponse.StatusCode >= 300 {
			errorBody, _ := io.ReadAll(proxyResponse.Body)
			proxyResponse.Body.Close()
			errorText := strings.TrimSpace(string(errorBody))
			if livePreview && responseStarted {
				emitPreview("error", decorateGatewayErrorMessage(cfg.Agent.Provider, proxyResponse.StatusCode, errorText))
				writeSSEDone(response)
				return
			}
			response.Header().Set("X-Claw-Provider", cfg.Agent.Provider)
			setProviderErrorHeaders(response.Header(), cfg.Agent.Provider, proxyResponse.StatusCode, errorText)
			http.Error(response, errorText, proxyResponse.StatusCode)
			return
		}

		if livePreview {
			startResponse()
			if iteration == 0 {
				emitPreview("phase", "Pensando y preparando respuesta…")
			}
		}

		sentDraftPhase := false
		acc, err := consumeSSEStream(proxyResponse.Body, func(acc *streamAccumulator, chunk sseChunk) {
			if !livePreview || len(chunk.Choices) == 0 {
				return
			}

			deltaContent := chunk.Choices[0].Delta.Content
			if deltaContent == "" {
				return
			}

			if !sentDraftPhase {
				sentDraftPhase = true
				emitPreview("phase", "Redactando respuesta…")
			}
			emitPreview("preview", acc.content.String())
		})
		proxyResponse.Body.Close()
		if err != nil {
			if livePreview && responseStarted {
				emitPreview("error", "No pude leer el stream del modelo.")
				writeSSEDone(response)
				return
			}
			http.Error(response, "read upstream stream: "+err.Error(), http.StatusBadGateway)
			return
		}

		assistantMsg := acc.message()
		allMessages = append(allMessages, assistantMsg)
		record.Messages = append(record.Messages, assistantMsg)

		// No tool calls — this is the final response; stream it to the client.
		if !acc.hasToolCalls() || acc.finishReason == "stop" {
			// Persist before writing so errors can still return HTTP 5xx.
			if _, err := store.Save(&record); err != nil {
				if livePreview && responseStarted {
					emitPreview("error", err.Error())
					writeSSEDone(response)
					return
				}
				http.Error(response, err.Error(), http.StatusInternalServerError)
				return
			}

			// Check for bootstrap completion marker.
			checkBootstrapCompletion(paths, record.Messages)

			if livePreview {
				startResponse()
				emitPreview("phase", "Respuesta lista")
				writeSSEDone(response)
				return
			}

			writeStreamingHeaders(response, sessionID)
			replaySSELines(response, acc.rawLines)
			return
		}

		// Registry missing — can't execute tools; stream what we have.
		if registry == nil {
			if _, err := store.Save(&record); err != nil {
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
				emitPreview("error", "El gateway no tiene registry de tools disponible.")
				writeSSEDone(response)
				return
			}
			writeStreamingHeaders(response, sessionID)
			replaySSELines(response, acc.rawLines)
			return
		}

		// Execute tool calls and feed results back into the loop.
		if livePreview {
			emitPreview("phase", "Usando herramientas…")
		}
		for _, tc := range assistantMsg.ToolCalls {
			if livePreview {
				emitPreview("tool", formatToolPreview(tc))
			}
			toolMessages := executeToolCallMessages(request.Context(), registry, tc)
			allMessages = append(allMessages, toolMessages...)
			record.Messages = append(record.Messages, toolMessages...)
			if livePreview {
				if len(toolMessages) > 0 {
					emitPreview("tool", formatToolResultPreview(toolMessages[0]))
				}
			}
		}
		if livePreview {
			emitPreview("phase", "Analizando resultados…")
		}
	}

	// Safety: iteration limit hit — persist and return empty done.
	_, _ = store.Save(&record)
	if livePreview {
		startResponse()
		emitPreview("error", "El agente alcanzó el límite de iteraciones.")
		writeSSEDone(response)
		return
	}
	writeStreamingHeaders(response, sessionID)
	writeSSEDone(response)
}
