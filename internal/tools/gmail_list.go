package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// gmailMessageSummary is a minimal message representation for listing.
type gmailMessageSummary struct {
	ID      string `json:"id"`
	Snippet string `json:"snippet"`
	From    string `json:"from,omitempty"`
	Subject string `json:"subject,omitempty"`
	Date    string `json:"date,omitempty"`
	Unread  bool   `json:"unread"`
}

// gmailDoRequest performs an authenticated GET against the Gmail API.
func gmailDoRequest(ctx context.Context, client *http.Client, endpoint string, query url.Values) ([]byte, error) {
	u := gmailAPIBase + endpoint
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gmail API error %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// gmailGetHeader extracts a specific header from a Gmail message payload.
func gmailGetHeader(headers []struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}, name string) string {
	for _, h := range headers {
		if h.Name == name {
			return h.Value
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// list_emails tool
// ---------------------------------------------------------------------------

type listEmailsTool struct{}

func NewListEmailsTool() Tool { return &listEmailsTool{} }

func (t *listEmailsTool) Name() string { return "list_emails" }

func (t *listEmailsTool) Description() string {
	return "List recent emails from Gmail. Returns a summary of messages including ID, subject, sender, date, and snippet. Use read_email to get the full content of a specific message."
}

func (t *listEmailsTool) Parameters() Schema {
	ten := 10
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"max_results": {
				Type:        "integer",
				Description: "Maximum number of emails to return (1-50). Defaults to 10.",
				Minimum:     func() *int { v := 1; return &v }(),
				Maximum:     func() *int { v := 50; return &v }(),
				Default:     ten,
			},
			"label": {
				Type:        "string",
				Description: "Gmail label to filter by. Defaults to INBOX. Use SENT, DRAFTS, TRASH, SPAM, or any custom label.",
				Default:     "INBOX",
			},
			"unread_only": {
				Type:        "boolean",
				Description: "If true, only return unread messages. Defaults to false.",
				Default:     false,
			},
		},
	}
}

func (t *listEmailsTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		MaxResults int    `json:"max_results"`
		Label      string `json:"label"`
		UnreadOnly bool   `json:"unread_only"`
	}{MaxResults: 10, Label: "INBOX"}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 10
	}
	if params.MaxResults > 50 {
		params.MaxResults = 50
	}
	if params.Label == "" {
		params.Label = "INBOX"
	}

	client, err := gmailHTTPClient(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}

	q := url.Values{}
	q.Set("maxResults", fmt.Sprintf("%d", params.MaxResults))
	q.Set("labelIds", params.Label)
	if params.UnreadOnly {
		q.Set("q", "is:unread")
	}

	data, err := gmailDoRequest(ctx, client, "/messages", q)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to list messages: %s", err))
	}

	var listResp struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		ResultSizeEstimate int `json:"resultSizeEstimate"`
	}
	if err := json.Unmarshal(data, &listResp); err != nil {
		return ErrorResult(fmt.Sprintf("failed to parse message list: %s", err))
	}

	if len(listResp.Messages) == 0 {
		return JSONResult(map[string]any{
			"messages": []any{},
			"total":    0,
			"label":    params.Label,
		})
	}

	// Fetch metadata for each message (in-thread, not parallel, to avoid hammering the API).
	summaries := make([]gmailMessageSummary, 0, len(listResp.Messages))
	for _, msg := range listResp.Messages {
		metaQ := url.Values{}
		metaQ.Set("format", "metadata")
		metaQ.Set("metadataHeaders", "From")
		metaQ.Set("metadataHeaders", "Subject")
		metaQ.Set("metadataHeaders", "Date")

		metaData, err := gmailDoRequest(ctx, client, "/messages/"+msg.ID, metaQ)
		if err != nil {
			// Skip messages we can't fetch rather than aborting.
			continue
		}

		var msgDetail struct {
			ID      string `json:"id"`
			Snippet string `json:"snippet"`
			Payload struct {
				Headers []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"payload"`
			LabelIDs []string `json:"labelIds"`
		}
		if err := json.Unmarshal(metaData, &msgDetail); err != nil {
			continue
		}

		unread := false
		for _, lbl := range msgDetail.LabelIDs {
			if lbl == "UNREAD" {
				unread = true
				break
			}
		}

		dateStr := gmailGetHeader(msgDetail.Payload.Headers, "Date")
		// Parse and reformat the date to something readable.
		if parsed, err := time.Parse("Mon, 2 Jan 2006 15:04:05 -0700", dateStr); err == nil {
			dateStr = parsed.Format("2006-01-02 15:04")
		}

		summaries = append(summaries, gmailMessageSummary{
			ID:      msgDetail.ID,
			Snippet: msgDetail.Snippet,
			From:    gmailGetHeader(msgDetail.Payload.Headers, "From"),
			Subject: gmailGetHeader(msgDetail.Payload.Headers, "Subject"),
			Date:    dateStr,
			Unread:  unread,
		})
	}

	return JSONResult(map[string]any{
		"messages": summaries,
		"total":    len(summaries),
		"label":    params.Label,
	})
}
