package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// gmailMessageDetail is the full email content returned by read_email.
type gmailMessageDetail struct {
	ID          string   `json:"id"`
	ThreadID    string   `json:"thread_id"`
	From        string   `json:"from"`
	To          string   `json:"to"`
	CC          string   `json:"cc,omitempty"`
	Subject     string   `json:"subject"`
	Date        string   `json:"date"`
	Body        string   `json:"body"`
	BodyType    string   `json:"body_type"` // "text" or "html"
	Attachments []string `json:"attachments,omitempty"`
	Unread      bool     `json:"unread"`
	Labels      []string `json:"labels"`
}

// gmailDecodeBody traverses a Gmail message payload to extract the text body.
func gmailDecodeBody(payload *gmailPayload) (body, bodyType string) {
	if payload == nil {
		return "", "text"
	}

	// Prefer text/plain, fall back to text/html.
	if payload.MimeType == "text/plain" && payload.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err == nil {
			return string(decoded), "text"
		}
	}
	if payload.MimeType == "text/html" && payload.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err == nil {
			return string(decoded), "html"
		}
	}

	// Recurse into multipart parts.
	var plainPart, htmlPart string
	for i := range payload.Parts {
		part := &payload.Parts[i]
		b, t := gmailDecodeBody(part)
		if t == "text" && b != "" {
			plainPart = b
		} else if t == "html" && b != "" {
			htmlPart = b
		}
	}
	if plainPart != "" {
		return plainPart, "text"
	}
	if htmlPart != "" {
		return htmlPart, "html"
	}
	return "", "text"
}

type gmailPayload struct {
	MimeType string `json:"mimeType"`
	Headers  []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"headers"`
	Body struct {
		Data string `json:"data"`
	} `json:"body"`
	Parts []gmailPayload `json:"parts"`
	// FileName is present for attachments.
	Filename string `json:"filename"`
}

// ---------------------------------------------------------------------------
// read_email tool
// ---------------------------------------------------------------------------

type readEmailTool struct{}

func NewReadEmailTool() Tool { return &readEmailTool{} }

func (t *readEmailTool) Name() string { return "read_email" }

func (t *readEmailTool) Description() string {
	return "Read the full content of a Gmail message by its ID (use list_emails or search_emails to get IDs). Returns sender, recipients, subject, date, and decoded body text."
}

func (t *readEmailTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"message_id": {
				Type:        "string",
				Description: "The Gmail message ID to retrieve (from list_emails or search_emails).",
			},
		},
		Required: []string{"message_id"},
	}
}

func (t *readEmailTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		MessageID string `json:"message_id"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}
	params.MessageID = strings.TrimSpace(params.MessageID)
	if params.MessageID == "" {
		return ErrorResult("message_id is required")
	}

	client, err := gmailHTTPClient(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}

	q := url.Values{}
	q.Set("format", "full")

	data, err := gmailDoRequest(ctx, client, "/messages/"+params.MessageID, q)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to fetch message: %s", err))
	}

	var raw struct {
		ID       string       `json:"id"`
		ThreadID string       `json:"threadId"`
		LabelIDs []string     `json:"labelIds"`
		Snippet  string       `json:"snippet"`
		Payload  gmailPayload `json:"payload"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return ErrorResult(fmt.Sprintf("failed to parse message: %s", err))
	}

	headers := raw.Payload.Headers
	getHdr := func(name string) string {
		for _, h := range headers {
			if h.Name == name {
				return h.Value
			}
		}
		return ""
	}

	unread := false
	for _, lbl := range raw.LabelIDs {
		if lbl == "UNREAD" {
			unread = true
			break
		}
	}

	// Collect attachment filenames.
	var attachments []string
	var collectAttachments func(p *gmailPayload)
	collectAttachments = func(p *gmailPayload) {
		if p.Filename != "" && p.Body.Data == "" {
			attachments = append(attachments, p.Filename)
		}
		for i := range p.Parts {
			collectAttachments(&p.Parts[i])
		}
	}
	collectAttachments(&raw.Payload)

	dateStr := getHdr("Date")
	if parsed, err := time.Parse("Mon, 2 Jan 2006 15:04:05 -0700", dateStr); err == nil {
		dateStr = parsed.Format("2006-01-02 15:04")
	}

	body, bodyType := gmailDecodeBody(&raw.Payload)

	detail := gmailMessageDetail{
		ID:          raw.ID,
		ThreadID:    raw.ThreadID,
		From:        getHdr("From"),
		To:          getHdr("To"),
		CC:          getHdr("Cc"),
		Subject:     getHdr("Subject"),
		Date:        dateStr,
		Body:        body,
		BodyType:    bodyType,
		Attachments: attachments,
		Unread:      unread,
		Labels:      raw.LabelIDs,
	}

	return JSONResult(detail)
}
