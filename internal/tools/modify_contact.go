package tools

import (
	"context"
)

type modifyContactTool struct {
	run commandRunner
}

func NewModifyContactTool() Tool {
	return &modifyContactTool{run: defaultCommandRunner}
}

func (t *modifyContactTool) Name() string { return "modify_contact" }

func (t *modifyContactTool) Description() string {
	return "Modify a contact in the personal Evolution Data Server address book. For complete control pass raw_vcard plus uid; otherwise common fields are applied as a patch over the existing contact."
}

func (t *modifyContactTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"uid": {
				Type:        "string",
				Description: "Contact UID to modify.",
			},
			"raw_vcard": {
				Type:        "string",
				Description: "Complete replacement vCard payload. When provided it takes precedence over individual fields.",
			},
			"full_name": {
				Type:        "string",
				Description: "Updated full contact name.",
			},
			"given_name": {
				Type:        "string",
				Description: "Updated given name.",
			},
			"family_name": {
				Type:        "string",
				Description: "Updated family name.",
			},
			"nickname": {
				Type:        "string",
				Description: "Updated nickname.",
			},
			"email": {
				Type:        "string",
				Description: "Primary email replacement when emails is omitted.",
			},
			"emails": {
				Type:        "array",
				Description: "Replacement set of email addresses.",
				Items:       &SchemaProperty{Type: "string"},
			},
			"phone": {
				Type:        "string",
				Description: "Primary phone replacement when phones is omitted.",
			},
			"phones": {
				Type:        "array",
				Description: "Replacement set of phone numbers.",
				Items:       &SchemaProperty{Type: "string"},
			},
			"organization": {
				Type:        "string",
				Description: "Updated organization or company.",
			},
			"title": {
				Type:        "string",
				Description: "Updated job title.",
			},
			"note": {
				Type:        "string",
				Description: "Updated note.",
			},
		},
		Required: []string{"uid"},
	}
}

func (t *modifyContactTool) Execute(ctx context.Context, arguments string) Result {
	var params contactMutation
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}
	params = normalizeContactMutation(params)
	if params.UID == "" {
		return ErrorResult("uid must not be empty")
	}

	var vcard string
	var err error
	if params.RawVCard != "" {
		vcard, err = normalizeProvidedContactVCard(params.UID, params.RawVCard)
		if err != nil {
			return ErrorResult(err.Error())
		}
	} else {
		existingVCard, _, getErr := getAddressBookContact(ctx, t.run, params.UID)
		if getErr != nil {
			return ErrorResult(getErr.Error())
		}
		existing := parseContactVCard(existingVCard)
		updated := applyContactMutation(existing, params)
		updated.UID = params.UID
		vcard, err = buildContactVCard(updated)
		if err != nil {
			return ErrorResult(err.Error())
		}
	}

	sourceUID, err := modifyAddressBookContacts(ctx, t.run, []string{vcard})
	if err != nil {
		return ErrorResult(err.Error())
	}
	updated := parseContactVCard(vcard)
	updated.UID = params.UID
	updated.VCard = ""

	return JSONResult(map[string]any{
		"ok":         true,
		"source_uid": sourceUID,
		"contact":    updated,
	})
	}