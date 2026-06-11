package tools

import (
	"context"
	"strings"
)

type deleteContactTool struct {
	run commandRunner
}

func NewDeleteContactTool() Tool {
	return &deleteContactTool{run: defaultCommandRunner}
}

func (t *deleteContactTool) Name() string { return "delete_contact" }

func (t *deleteContactTool) Description() string {
	return "Delete a contact from the personal Evolution Data Server address book by UID."
}

func (t *deleteContactTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"uid": {
				Type:        "string",
				Description: "Contact UID to delete.",
			},
		},
		Required: []string{"uid"},
	}
	}

func (t *deleteContactTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		UID string `json:"uid"`
	}{ }
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}
	uid := strings.TrimSpace(params.UID)
	if uid == "" {
		return ErrorResult("uid must not be empty")
	}

	sourceUID, err := removeAddressBookContacts(ctx, t.run, []string{uid})
	if err != nil {
		return ErrorResult(err.Error())
	}
	return JSONResult(map[string]any{
		"ok":         true,
		"source_uid": sourceUID,
		"uid":        uid,
	})
	}