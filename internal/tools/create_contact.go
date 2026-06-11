package tools

import (
	"context"
	"strings"
)

type createContactTool struct {
	run commandRunner
}

func NewCreateContactTool() Tool {
	return &createContactTool{run: defaultCommandRunner}
}

func (t *createContactTool) Name() string { return "create_contact" }

func (t *createContactTool) Description() string {
	return "Create a contact in the personal Evolution Data Server address book. For full control provide raw_vcard. Otherwise you can pass common contact fields and Sam will build the vCard for you."
}

func (t *createContactTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"raw_vcard": {
				Type:        "string",
				Description: "Complete vCard payload. When provided it takes precedence over individual fields.",
			},
			"full_name": {
				Type:        "string",
				Description: "Full contact display name.",
			},
			"given_name": {
				Type:        "string",
				Description: "Given name.",
			},
			"family_name": {
				Type:        "string",
				Description: "Family name.",
			},
			"nickname": {
				Type:        "string",
				Description: "Nickname.",
			},
			"email": {
				Type:        "string",
				Description: "Primary email address.",
			},
			"emails": {
				Type:        "array",
				Description: "Email addresses to store on the contact.",
				Items:       &SchemaProperty{Type: "string"},
			},
			"phone": {
				Type:        "string",
				Description: "Primary phone number.",
			},
			"phones": {
				Type:        "array",
				Description: "Phone numbers to store on the contact.",
				Items:       &SchemaProperty{Type: "string"},
			},
			"organization": {
				Type:        "string",
				Description: "Organization or company.",
			},
			"title": {
				Type:        "string",
				Description: "Job title.",
			},
			"note": {
				Type:        "string",
				Description: "Free-form note.",
			},
		},
	}
}

func (t *createContactTool) Execute(ctx context.Context, arguments string) Result {
	var params contactMutation
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}
	params = normalizeContactMutation(params)

	var vcard string
	var err error
	if params.RawVCard != "" {
		vcard, err = normalizeProvidedContactVCard("", params.RawVCard)
		if err != nil {
			return ErrorResult(err.Error())
		}
	} else {
		record := applyContactMutation(contactRecord{}, params)
		vcard, err = buildContactVCard(record)
		if err != nil {
			return ErrorResult(err.Error())
		}
	}

	uids, sourceUID, err := createAddressBookContacts(ctx, t.run, []string{vcard})
	if err != nil {
		return ErrorResult(err.Error())
	}
	created := parseContactVCard(vcard)
	if len(uids) > 0 && strings.TrimSpace(uids[0]) != "" {
		created.UID = strings.TrimSpace(uids[0])
	}
	created.VCard = ""

	return JSONResult(map[string]any{
		"ok":         true,
		"source_uid": sourceUID,
		"contact":    created,
	})
	}