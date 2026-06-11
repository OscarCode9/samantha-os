package tools

import "context"

type listContactsTool struct {
	run commandRunner
}

func NewListContactsTool() Tool {
	return &listContactsTool{run: defaultCommandRunner}
}

func (t *listContactsTool) Name() string { return "list_contacts" }

func (t *listContactsTool) Description() string {
	return "List contacts from the personal Evolution Data Server address book. Returns contact UID, name, emails, phones, organization, title, and optionally the raw vCard."
}

func (t *listContactsTool) Parameters() Schema {
	defaultLimit := 100
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"limit": {
				Type:        "integer",
				Description: "Maximum number of contacts to return. Defaults to 100.",
				Minimum:     intPtr(1),
				Maximum:     intPtr(500),
				Default:     defaultLimit,
			},
			"include_vcard": {
				Type:        "boolean",
				Description: "When true, includes the raw vCard for each contact.",
			},
		},
	}
}

func (t *listContactsTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Limit        int  `json:"limit"`
		IncludeVCard bool `json:"include_vcard"`
	}{Limit: 100}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}
	if params.Limit <= 0 {
		params.Limit = 100
	}
	if params.Limit > 500 {
		params.Limit = 500
	}

	vcards, sourceUID, err := getAddressBookContactList(ctx, t.run, "")
	if err != nil {
		return ErrorResult(err.Error())
	}

	contacts := contactRecordsFromVCards(vcards, params.IncludeVCard, params.Limit)
	return JSONResult(map[string]any{
		"source_uid": sourceUID,
		"count":      len(contacts),
		"contacts":   contacts,
	})
	}