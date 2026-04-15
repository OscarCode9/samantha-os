package runtime

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/oscarcode/elementary-claw/internal/session"
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
