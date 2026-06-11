package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// send_email tool
// ---------------------------------------------------------------------------

type sendEmailTool struct{}

func NewSendEmailTool() Tool { return &sendEmailTool{} }

func (t *sendEmailTool) Name() string { return "send_email" }

func (t *sendEmailTool) Description() string {
	return "Send an email via Gmail. Composes and delivers a message on behalf of the authorized Gmail account."
}

func (t *sendEmailTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"to": {
				Type:        "string",
				Description: "Recipient email address (or comma-separated list for multiple recipients).",
			},
			"subject": {
				Type:        "string",
				Description: "Email subject line.",
			},
			"body": {
				Type:        "string",
				Description: "Plain-text email body.",
			},
			"cc": {
				Type:        "string",
				Description: "Optional CC email address (or comma-separated list).",
			},
			"reply_to_message_id": {
				Type:        "string",
				Description: "Optional Gmail message ID to reply to. Threads the reply correctly.",
			},
		},
		Required: []string{"to", "subject", "body"},
	}
}

func (t *sendEmailTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		To           string `json:"to"`
		Subject      string `json:"subject"`
		Body         string `json:"body"`
		CC           string `json:"cc"`
		ReplyToMsgID string `json:"reply_to_message_id"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	params.To = strings.TrimSpace(params.To)
	params.Subject = strings.TrimSpace(params.Subject)
	params.Body = strings.TrimSpace(params.Body)

	if params.To == "" {
		return ErrorResult("to is required")
	}
	if params.Subject == "" {
		return ErrorResult("subject is required")
	}
	if params.Body == "" {
		return ErrorResult("body is required")
	}

	client, err := gmailHTTPClient(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}

	// Build RFC 2822 message.
	var buf strings.Builder
	buf.WriteString("To: " + params.To + "\r\n")
	if params.CC != "" {
		buf.WriteString("Cc: " + params.CC + "\r\n")
	}
	buf.WriteString("Subject: " + params.Subject + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buf.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")

	// If replying, look up the In-Reply-To and References headers.
	if params.ReplyToMsgID != "" {
		metaData, err := gmailDoRequest(ctx, client, "/messages/"+params.ReplyToMsgID, nil)
		if err == nil {
			var metaMsg struct {
				Payload struct {
					Headers []struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"headers"`
				} `json:"payload"`
				ThreadID string `json:"threadId"`
			}
			if jsonErr := json.Unmarshal(metaData, &metaMsg); jsonErr == nil {
				getHdr := func(name string) string {
					for _, h := range metaMsg.Payload.Headers {
						if h.Name == name {
							return h.Value
						}
					}
					return ""
				}
				if msgID := getHdr("Message-ID"); msgID != "" {
					buf.WriteString("In-Reply-To: " + msgID + "\r\n")
					refs := getHdr("References")
					if refs != "" {
						buf.WriteString("References: " + refs + " " + msgID + "\r\n")
					} else {
						buf.WriteString("References: " + msgID + "\r\n")
					}
				}
			}
		}
	}

	buf.WriteString("\r\n")
	buf.WriteString(params.Body)

	raw := base64.URLEncoding.EncodeToString([]byte(buf.String()))

	reqBody := []byte(`{"raw":"` + raw + `"}`)
	if params.ReplyToMsgID != "" {
		// We need to look up the thread ID to thread the reply.
		threadData, err := gmailDoRequest(ctx, client, "/messages/"+params.ReplyToMsgID, nil)
		if err == nil {
			var threadMsg struct {
				ThreadID string `json:"threadId"`
			}
			if jsonErr := json.Unmarshal(threadData, &threadMsg); jsonErr == nil && threadMsg.ThreadID != "" {
				reqBody = []byte(`{"raw":"` + raw + `","threadId":"` + threadMsg.ThreadID + `"}`)
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gmailAPIBase+"/messages/send", bytes.NewReader(reqBody))
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to build send request: %s", err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("send request failed: %s", err))
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return ErrorResult(fmt.Sprintf("gmail send error %d: %s", resp.StatusCode, string(respBody)))
	}

	var sentMsg struct {
		ID       string   `json:"id"`
		ThreadID string   `json:"threadId"`
		LabelIDs []string `json:"labelIds"`
	}
	_ = json.Unmarshal(respBody, &sentMsg)

	return JSONResult(map[string]any{
		"ok":        true,
		"message":   "Email sent successfully.",
		"id":        sentMsg.ID,
		"thread_id": sentMsg.ThreadID,
		"to":        params.To,
		"subject":   params.Subject,
	})
}
