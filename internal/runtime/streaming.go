package runtime

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/session"
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
func consumeSSEStream(reader io.Reader) (*streamAccumulator, error) {
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

// writeSSEDone writes the SSE [DONE] sentinel and flushes.
func writeSSEDone(w io.Writer) {
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher, ok := w.(interface{ Flush() }); ok {
		flusher.Flush()
	}
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
) {
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

	allMessages := append(cloneMessages(record.Messages), incomingMessages...)
	record.Messages = append(record.Messages, incomingMessages...)

	delete(payload, "session_id")
	removeSessionMetadata(payload)

	if registry != nil && registry.Count() > 0 {
		payload["tools"] = registry.Definitions()
	}
	// Always request streaming from upstream.
	payload["stream"] = true

	upstreamURL := buildUpstreamURL(target, request.URL.Path)
	if trimmedQuery := strings.TrimSpace(request.URL.RawQuery); trimmedQuery != "" {
		upstreamURL += "?" + trimmedQuery
	}

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
			http.Error(response, string(errorBody), proxyResponse.StatusCode)
			return
		}

		acc, err := consumeSSEStream(proxyResponse.Body)
		proxyResponse.Body.Close()
		if err != nil {
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
				http.Error(response, err.Error(), http.StatusInternalServerError)
				return
			}

			response.Header().Set("Content-Type", "text/event-stream")
			response.Header().Set("Cache-Control", "no-cache")
			response.Header().Set("X-Accel-Buffering", "no")
			response.Header().Set("X-Session-ID", sessionID)
			response.WriteHeader(http.StatusOK)
			replaySSELines(response, acc.rawLines)
			return
		}

		// Registry missing — can't execute tools; stream what we have.
		if registry == nil {
			if _, err := store.Save(&record); err != nil {
				http.Error(response, err.Error(), http.StatusInternalServerError)
				return
			}
			response.Header().Set("Content-Type", "text/event-stream")
			response.Header().Set("Cache-Control", "no-cache")
			response.Header().Set("X-Accel-Buffering", "no")
			response.Header().Set("X-Session-ID", sessionID)
			response.WriteHeader(http.StatusOK)
			replaySSELines(response, acc.rawLines)
			return
		}

		// Execute tool calls and feed results back into the loop.
		for _, tc := range assistantMsg.ToolCalls {
			toolResult := executeToolCall(request.Context(), registry, tc)
			allMessages = append(allMessages, toolResult)
			record.Messages = append(record.Messages, toolResult)
		}
	}

	// Safety: iteration limit hit — persist and return empty done.
	_, _ = store.Save(&record)
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache")
	response.Header().Set("X-Session-ID", sessionID)
	response.WriteHeader(http.StatusOK)
	writeSSEDone(response)
}
