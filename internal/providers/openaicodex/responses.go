package openaicodex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/oscarcode/elementary-claw/internal/session"
)

var ResponsesEndpoint = DefaultBaseURL + "/responses"

type HTTPStatusError struct {
	StatusCode int
	Body       string
}

func (err *HTTPStatusError) Error() string {
	return fmt.Sprintf("OpenAI Codex HTTP %d: %s", err.StatusCode, err.Body)
}

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

type StreamEvent struct {
	Type      string
	Content   string
	ItemID    string
	CallID    string
	Name      string
	Arguments string
	Usage     *Usage
}

type Response struct {
	ID      string
	Message session.Message
	Usage   *Usage
}

func StreamResponse(
	ctx context.Context,
	credential Credential,
	model string,
	messages []session.Message,
	tools []ToolDefinition,
	onEvent func(StreamEvent),
) (Response, error) {
	instructions, input := convertMessages(messages)

	requestBody := map[string]any{
		"model":  normalizeModel(model),
		"store":  false,
		"stream": true,
		"input":  input,
	}
	if strings.TrimSpace(instructions) != "" {
		requestBody["instructions"] = instructions
	}
	if len(tools) > 0 {
		encodedTools := make([]map[string]any, 0, len(tools))
		for _, tool := range tools {
			encodedTools = append(encodedTools, map[string]any{
				"type":        "function",
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
			})
		}
		requestBody["tools"] = encodedTools
		requestBody["tool_choice"] = "auto"
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return Response{}, fmt.Errorf("marshal OpenAI Codex request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, ResponsesEndpoint, bytes.NewReader(payload))
	if err != nil {
		return Response{}, fmt.Errorf("build OpenAI Codex request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("OpenAI-Beta", "responses=experimental")
	request.Header.Set("User-Agent", "elementary-claw/1.0")
	if accountID := strings.TrimSpace(credential.AccountID); accountID != "" {
		request.Header.Set("chatgpt-account-id", accountID)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return Response{}, fmt.Errorf("call OpenAI Codex responses endpoint: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(response.Body)
		return Response{}, &HTTPStatusError{
			StatusCode: response.StatusCode,
			Body:       strings.TrimSpace(string(body)),
		}
	}

	return parseResponseStream(response.Body, onEvent)
}

func parseResponseStream(reader io.Reader, onEvent func(StreamEvent)) (Response, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var responseID string
	var content strings.Builder
	var toolOrder []string
	toolNames := make(map[string]string)
	itemIDToCallID := make(map[string]string)
	toolArgs := make(map[string][]string)
	var usage *Usage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "" || payload == "[DONE]" {
			continue
		}

		var event map[string]any
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}

		eventType, _ := event["type"].(string)
		switch eventType {
		case "response.created":
			if responseMap, _ := event["response"].(map[string]any); responseMap != nil {
				if id, _ := responseMap["id"].(string); id != "" {
					responseID = id
				}
			}
		case "response.output_text.delta":
			delta, _ := event["delta"].(string)
			if delta != "" {
				content.WriteString(delta)
				if onEvent != nil {
					onEvent(StreamEvent{Type: "output_text_delta", Content: delta})
				}
			}
		case "response.function_call_arguments.delta":
			itemID, _ := event["item_id"].(string)
			callID, _ := event["call_id"].(string)
			delta, _ := event["delta"].(string)
			key := strings.TrimSpace(itemID)
			if key == "" {
				key = strings.TrimSpace(callID)
			}
			if key == "" || delta == "" {
				continue
			}
			toolArgs[key] = append(toolArgs[key], delta)
			if onEvent != nil {
				onEvent(StreamEvent{
					Type:      "tool_arguments_delta",
					ItemID:    itemID,
					CallID:    callID,
					Arguments: delta,
				})
			}
		case "response.output_item.added":
			item, _ := event["item"].(map[string]any)
			if item == nil {
				continue
			}
			if itemType, _ := item["type"].(string); itemType != "function_call" {
				continue
			}

			itemID, _ := item["id"].(string)
			callID, _ := item["call_id"].(string)
			name, _ := item["name"].(string)
			if itemID == "" {
				itemID = callID
			}
			if callID == "" {
				callID = itemID
			}
			if itemID == "" {
				continue
			}
			if _, exists := toolNames[itemID]; !exists {
				toolOrder = append(toolOrder, itemID)
			}
			toolNames[itemID] = name
			itemIDToCallID[itemID] = callID
			if onEvent != nil {
				onEvent(StreamEvent{
					Type:   "tool_call_added",
					ItemID: itemID,
					CallID: callID,
					Name:   name,
				})
			}
		case "response.completed", "response.done":
			responseMap, _ := event["response"].(map[string]any)
			if responseMap == nil {
				continue
			}
			if id, _ := responseMap["id"].(string); id != "" {
				responseID = id
			}
			usageMap, _ := responseMap["usage"].(map[string]any)
			if usageMap != nil {
				inputTokens := int(numberValue(usageMap["input_tokens"]))
				outputTokens := int(numberValue(usageMap["output_tokens"]))
				usage = &Usage{
					InputTokens:  inputTokens,
					OutputTokens: outputTokens,
				}
				if onEvent != nil {
					onEvent(StreamEvent{
						Type:  "usage",
						Usage: usage,
					})
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return Response{}, fmt.Errorf("read OpenAI Codex stream: %w", err)
	}

	if len(toolOrder) == 0 && len(toolArgs) > 0 {
		for key := range toolArgs {
			toolOrder = append(toolOrder, key)
		}
		sort.Strings(toolOrder)
	}

	message := session.Message{
		Role:    "assistant",
		Content: content.String(),
	}
	for _, itemID := range toolOrder {
		message.ToolCalls = append(message.ToolCalls, session.ToolCall{
			ID:   itemIDToCallID[itemID],
			Type: "function",
			Function: session.FunctionCall{
				Name:      toolNames[itemID],
				Arguments: strings.Join(toolArgs[itemID], ""),
			},
		})
	}

	return Response{
		ID:      responseID,
		Message: message,
		Usage:   usage,
	}, nil
}

func convertMessages(messages []session.Message) (string, []any) {
	instructionsParts := make([]string, 0, 1)
	input := make([]any, 0, len(messages))

	for _, message := range messages {
		switch message.Role {
		case "system":
			text := strings.TrimSpace(message.Content)
			if text != "" {
				instructionsParts = append(instructionsParts, text)
			}
		case "tool":
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": message.ToolCallID,
				"output":  message.Content,
			})
		case "assistant":
			if strings.TrimSpace(message.Content) != "" || len(message.ContentParts) > 0 {
				input = append(input, map[string]any{
					"type":    "message",
					"role":    "assistant",
					"content": encodeMessageParts(message, false),
				})
			}
			for _, toolCall := range message.ToolCalls {
				input = append(input, map[string]any{
					"type":      "function_call",
					"call_id":   toolCall.ID,
					"name":      toolCall.Function.Name,
					"arguments": toolCall.Function.Arguments,
				})
			}
		default:
			input = append(input, map[string]any{
				"type":    "message",
				"role":    "user",
				"content": encodeMessageParts(message, true),
			})
		}
	}

	if len(input) == 0 {
		input = append(input, map[string]any{
			"type": "message",
			"role": "user",
			"content": []map[string]any{
				{"type": "input_text", "text": "Hello, let's start the onboarding setup."},
			},
		})
	}

	return strings.Join(instructionsParts, "\n\n"), input
}

func encodeMessageParts(message session.Message, userRole bool) []map[string]any {
	parts := make([]map[string]any, 0, len(message.ContentParts)+1)
	if len(message.ContentParts) == 0 {
		text := strings.TrimSpace(message.Content)
		if text == "" {
			return parts
		}
		if userRole {
			return []map[string]any{{"type": "input_text", "text": text}}
		}
		return []map[string]any{{"type": "output_text", "text": text}}
	}

	for _, part := range message.ContentParts {
		switch part.Type {
		case "text", "input_text":
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			partType := "output_text"
			if userRole {
				partType = "input_text"
			}
			parts = append(parts, map[string]any{
				"type": partType,
				"text": text,
			})
		case "image_url":
			if !userRole || part.ImageURL == nil || strings.TrimSpace(part.ImageURL.URL) == "" {
				continue
			}
			item := map[string]any{
				"type":      "input_image",
				"image_url": part.ImageURL.URL,
			}
			if detail := strings.TrimSpace(part.ImageURL.Detail); detail != "" {
				item["detail"] = detail
			}
			parts = append(parts, item)
		}
	}

	return parts
}

func normalizeModel(model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return DefaultModel
	}
	parts := strings.Split(trimmed, "/")
	model = strings.TrimSpace(parts[len(parts)-1])
	if model == "gpt-5.3-codex" {
		return "gpt-5.4"
	}
	if model != "gpt-5.4" {
		return DefaultModel
	}
	return model
}

func numberValue(raw any) float64 {
	switch value := raw.(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		number, err := value.Float64()
		if err == nil {
			return number
		}
	}
	return 0
}
