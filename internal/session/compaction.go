package session

import (
	"fmt"
	"strings"
)

// DefaultCompactionThreshold is the message count above which a session
// should be compacted.
const DefaultCompactionThreshold = 40

// DefaultKeepLastMessages is how many recent messages to preserve verbatim
// after compaction.
const DefaultKeepLastMessages = 10

// NeedsCompaction returns true if the session has more messages than the
// given threshold.
func NeedsCompaction(record *Record, maxMessages int) bool {
	if maxMessages <= 0 {
		maxMessages = DefaultCompactionThreshold
	}
	return len(record.Messages) > maxMessages
}

// BuildCompactionRequest prepares a message list asking the LLM to summarize
// the old portion of the conversation. The messages to compact are serialized
// as a single user message for summarization.
func BuildCompactionRequest(record *Record, keepLast int) []Message {
	if keepLast <= 0 {
		keepLast = DefaultKeepLastMessages
	}
	if len(record.Messages) <= keepLast {
		return nil // nothing to compact
	}

	// Messages to summarize: everything except the last N.
	toSummarize := record.Messages[:len(record.Messages)-keepLast]

	var sb strings.Builder
	for _, msg := range toSummarize {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				sb.WriteString(fmt.Sprintf("  → tool_call: %s(%s)\n", tc.Function.Name, tc.Function.Arguments))
			}
		}
	}

	return []Message{
		{
			Role: "system",
			Content: "Summarize the following conversation concisely. " +
				"Preserve key facts, decisions, user preferences, and important context. " +
				"Do not include tool call details unless the result is important.",
		},
		{
			Role:    "user",
			Content: sb.String(),
		},
	}
}

// ApplyCompaction replaces old messages with a summary message, keeping the
// last N messages intact. The summary is stored as a system message.
func ApplyCompaction(record *Record, summary string, keepLast int) {
	if keepLast <= 0 {
		keepLast = DefaultKeepLastMessages
	}
	if len(record.Messages) <= keepLast {
		return
	}

	recentMessages := make([]Message, len(record.Messages[len(record.Messages)-keepLast:]))
	copy(recentMessages, record.Messages[len(record.Messages)-keepLast:])

	summaryMsg := Message{
		Role:    "system",
		Content: "[Previous conversation summary]\n" + summary,
	}

	record.Messages = append([]Message{summaryMsg}, recentMessages...)
}
