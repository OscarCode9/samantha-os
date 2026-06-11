package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// ---------------------------------------------------------------------------
// search_emails tool
// ---------------------------------------------------------------------------

type searchEmailsTool struct{}

func NewSearchEmailsTool() Tool { return &searchEmailsTool{} }

func (t *searchEmailsTool) Name() string { return "search_emails" }

func (t *searchEmailsTool) Description() string {
	return "Search Gmail using Gmail search query syntax (same as the Gmail search bar). Examples: 'from:boss@example.com', 'subject:invoice', 'is:unread after:2024/01/01', 'has:attachment'. Returns a list of matching messages with metadata."
}

func (t *searchEmailsTool) Parameters() Schema {
	ten := 10
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"query": {
				Type:        "string",
				Description: "Gmail search query (e.g. 'from:alice@example.com is:unread', 'subject:invoice has:attachment', 'after:2024/01/01').",
			},
			"max_results": {
				Type:        "integer",
				Description: "Maximum number of results to return (1-50). Defaults to 10.",
				Minimum:     func() *int { v := 1; return &v }(),
				Maximum:     func() *int { v := 50; return &v }(),
				Default:     ten,
			},
		},
		Required: []string{"query"},
	}
}

func (t *searchEmailsTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}{MaxResults: 10}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}
	if params.Query == "" {
		return ErrorResult("query is required")
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 10
	}
	if params.MaxResults > 50 {
		params.MaxResults = 50
	}

	client, err := gmailHTTPClient(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}

	q := url.Values{}
	q.Set("q", params.Query)
	q.Set("maxResults", fmt.Sprintf("%d", params.MaxResults))

	data, err := gmailDoRequest(ctx, client, "/messages", q)
	if err != nil {
		return ErrorResult(fmt.Sprintf("search failed: %s", err))
	}

	var listResp struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		ResultSizeEstimate int `json:"resultSizeEstimate"`
	}
	if err := json.Unmarshal(data, &listResp); err != nil {
		return ErrorResult(fmt.Sprintf("failed to parse search results: %s", err))
	}

	if len(listResp.Messages) == 0 {
		return JSONResult(map[string]any{
			"messages": []any{},
			"total":    0,
			"query":    params.Query,
		})
	}

	summaries := make([]gmailMessageSummary, 0, len(listResp.Messages))
	for _, msg := range listResp.Messages {
		metaQ := url.Values{}
		metaQ.Set("format", "metadata")
		metaQ.Set("metadataHeaders", "From")
		metaQ.Set("metadataHeaders", "Subject")
		metaQ.Set("metadataHeaders", "Date")

		metaData, err := gmailDoRequest(ctx, client, "/messages/"+msg.ID, metaQ)
		if err != nil {
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
		"query":    params.Query,
	})
}
