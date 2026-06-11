package session

import (
	"fmt"
	"strings"
	"testing"
)

func TestNeedsCompactionBelowThreshold(t *testing.T) {
	record := &Record{Messages: make([]Message, 10)}
	if NeedsCompaction(record, 40) {
		t.Fatal("should not need compaction with 10 messages")
	}
}

func TestNeedsCompactionAboveThreshold(t *testing.T) {
	record := &Record{Messages: make([]Message, 50)}
	if !NeedsCompaction(record, 40) {
		t.Fatal("should need compaction with 50 messages")
	}
}

func TestNeedsCompactionDefaultThreshold(t *testing.T) {
	record := &Record{Messages: make([]Message, DefaultCompactionThreshold + 1)}
	if !NeedsCompaction(record, 0) {
		t.Fatal("should need compaction with default threshold")
	}
}

func TestBuildCompactionRequestEmpty(t *testing.T) {
	record := &Record{Messages: make([]Message, 5)}
	result := BuildCompactionRequest(record, 10)
	if result != nil {
		t.Fatal("expected nil for short conversation")
	}
}

func TestBuildCompactionRequest(t *testing.T) {
	messages := make([]Message, 20)
	for i := range messages {
		if i%2 == 0 {
			messages[i] = Message{Role: "user", Content: fmt.Sprintf("Question %d", i/2+1)}
		} else {
			messages[i] = Message{Role: "assistant", Content: fmt.Sprintf("Answer %d", (i+1)/2)}
		}
	}
	record := &Record{Messages: messages}

	result := BuildCompactionRequest(record, 10)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(result))
	}
	if result[0].Role != "system" {
		t.Fatalf("expected system message, got %s", result[0].Role)
	}
	if !strings.Contains(result[0].Content, "Summarize") {
		t.Fatal("expected summarize instruction")
	}

	// The user message should contain the first 10 messages (indices 0-9).
	if !strings.Contains(result[1].Content, "Question 1") {
		t.Fatal("expected first question in compaction request")
	}
	if !strings.Contains(result[1].Content, "Answer 5") {
		t.Fatal("expected answer 5 in compaction request")
	}
	// Should NOT contain the last 10 messages.
	if strings.Contains(result[1].Content, "Question 6") {
		t.Fatal("should not contain messages from kept portion (Question 6)")
	}
}

func TestBuildCompactionRequestWithToolCalls(t *testing.T) {
	record := &Record{
		Messages: []Message{
			{Role: "user", Content: "Read the file"},
			{Role: "assistant", Content: "", ToolCalls: []ToolCall{
				{ID: "tc1", Type: "function", Function: FunctionCall{Name: "read_file", Arguments: `{"path":"test.txt"}`}},
			}},
			{Role: "tool", Content: "file contents here", ToolCallID: "tc1", Name: "read_file"},
			{Role: "assistant", Content: "The file says..."},
			// Keep last 2.
			{Role: "user", Content: "Thanks"},
			{Role: "assistant", Content: "You're welcome"},
		},
	}

	result := BuildCompactionRequest(record, 2)
	if result == nil {
		t.Fatal("expected compaction request")
	}
	if !strings.Contains(result[1].Content, "read_file") {
		t.Fatal("expected tool call in compaction text")
	}
}

func TestApplyCompaction(t *testing.T) {
	messages := make([]Message, 50)
	for i := range messages {
		if i%2 == 0 {
			messages[i] = Message{Role: "user", Content: fmt.Sprintf("Q%d", i/2+1)}
		} else {
			messages[i] = Message{Role: "assistant", Content: fmt.Sprintf("A%d", (i+1)/2)}
		}
	}
	record := &Record{Messages: messages}

	ApplyCompaction(record, "The user asked 25 questions and got answers.", 10)

	// Should have 1 summary + 10 recent = 11 messages.
	if len(record.Messages) != 11 {
		t.Fatalf("expected 11 messages after compaction, got %d", len(record.Messages))
	}

	// First message should be the summary.
	if record.Messages[0].Role != "system" {
		t.Fatalf("expected system role for summary, got %s", record.Messages[0].Role)
	}
	if !strings.Contains(record.Messages[0].Content, "[Previous conversation summary]") {
		t.Fatal("expected summary marker")
	}
	if !strings.Contains(record.Messages[0].Content, "25 questions") {
		t.Fatal("expected summary content")
	}

	// Last 10 messages should be preserved.
	if record.Messages[1].Content != "Q21" {
		t.Fatalf("expected Q21 as first kept message, got %s", record.Messages[1].Content)
	}
	if record.Messages[10].Content != "A25" {
		t.Fatalf("expected A25 as last message, got %s", record.Messages[10].Content)
	}
}

func TestApplyCompactionShortSession(t *testing.T) {
	record := &Record{
		Messages: []Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello"},
		},
	}
	ApplyCompaction(record, "summary", 10)

	// Should not change anything — too few messages.
	if len(record.Messages) != 2 {
		t.Fatalf("expected 2 messages unchanged, got %d", len(record.Messages))
	}
}

func TestApplyCompactionReducesMessageCount(t *testing.T) {
	messages := make([]Message, 60)
	for i := range messages {
		messages[i] = Message{Role: "user", Content: fmt.Sprintf("msg %d", i)}
	}
	record := &Record{Messages: messages}

	ApplyCompaction(record, "Everything was discussed.", 10)

	if len(record.Messages) >= 15 {
		t.Fatalf("expected compact session under 15 messages, got %d", len(record.Messages))
	}
}
