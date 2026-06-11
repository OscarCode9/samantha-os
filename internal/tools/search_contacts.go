package tools

import (
	"context"
	"strings"
)

type searchContactsTool struct {
	run commandRunner
}

func NewSearchContactsTool() Tool {
	return &searchContactsTool{run: defaultCommandRunner}
}

func (t *searchContactsTool) Name() string { return "search_contacts" }

func (t *searchContactsTool) Description() string {
	return "Search contacts in the personal Evolution Data Server address book by name or email. Returns matching contact summaries and optionally the raw vCard."
}

func (t *searchContactsTool) Parameters() Schema {
	defaultLimit := 50
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"query": {
				Type:        "string",
				Description: "Name or email fragment to search for.",
			},
			"limit": {
				Type:        "integer",
				Description: "Maximum number of contacts to return. Defaults to 50.",
				Minimum:     intPtr(1),
				Maximum:     intPtr(500),
				Default:     defaultLimit,
			},
			"include_vcard": {
				Type:        "boolean",
				Description: "When true, includes the raw vCard for each contact.",
			},
		},
		Required: []string{"query"},
	}
}

func (t *searchContactsTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Query        string `json:"query"`
		Limit        int    `json:"limit"`
		IncludeVCard bool   `json:"include_vcard"`
	}{Limit: 50}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}
	params.Query = strings.TrimSpace(params.Query)
	if params.Query == "" {
		return ErrorResult("query must not be empty")
	}
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 500 {
		params.Limit = 500
	}

	query := buildContactSearchQuery(params.Query)
	vcards, sourceUID, err := getAddressBookContactList(ctx, t.run, query)
	if err != nil {
		return ErrorResult(err.Error())
	}

	contacts := contactRecordsFromVCards(vcards, params.IncludeVCard, params.Limit)
	return JSONResult(map[string]any{
		"source_uid": sourceUID,
		"query":      params.Query,
		"count":      len(contacts),
		"contacts":   contacts,
	})
	}