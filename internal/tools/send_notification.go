package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	notificationsBus  = "org.freedesktop.Notifications"
	notificationsPath = "/org/freedesktop/Notifications"
)

type sendNotificationTool struct {
	run commandRunner
}

func NewSendNotificationTool() Tool {
	return &sendNotificationTool{run: defaultCommandRunner}
}

func (t *sendNotificationTool) Name() string { return "send_notification" }

func (t *sendNotificationTool) Description() string {
	return "Send a desktop notification to the user via DBus (org.freedesktop.Notifications). Use to proactively alert the user, confirm an action, or give a status update."
}

func (t *sendNotificationTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"summary": {
				Type:        "string",
				Description: "Short notification title.",
			},
			"body": {
				Type:        "string",
				Description: "Optional longer description text.",
			},
			"icon": {
				Type:        "string",
				Description: "Optional icon name (e.g. dialog-information, dialog-warning, appointment-new). Defaults to dialog-information.",
			},
			"expire_seconds": {
				Type:        "integer",
				Description: "How long (seconds) before the notification auto-dismisses. 0 = server default. Defaults to 5.",
			},
		},
		Required: []string{"summary"},
	}
}

func (t *sendNotificationTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Summary       string `json:"summary"`
		Body          string `json:"body"`
		Icon          string `json:"icon"`
		ExpireSeconds int    `json:"expire_seconds"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	summary := strings.TrimSpace(params.Summary)
	if summary == "" {
		return ErrorResult("summary must not be empty")
	}

	icon := strings.TrimSpace(params.Icon)
	if icon == "" {
		icon = "dialog-information"
	}

	expireMs := params.ExpireSeconds * 1000
	if expireMs == 0 {
		expireMs = 5000
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	// Notify(app_name, replaces_id, app_icon, summary, body, actions[], hints{}, expire_timeout)
	// Signature: susssasa{sv}i
	out, err := run(ctx, "busctl", "--user", "--json=short", "call",
		notificationsBus, notificationsPath, notificationsBus,
		"Notify",
		"susssasa{sv}i",
		"Sam",                       // app_name
		"0",                         // replaces_id (0 = new)
		icon,                        // app_icon
		summary,                     // summary
		params.Body,                 // body
		"0",                         // actions (empty array)
		"0",                         // hints (empty dict)
		fmt.Sprintf("%d", expireMs), // expire_timeout ms
	)
	if err != nil {
		return ErrorResult(fmt.Sprintf("send notification failed: %v", err))
	}

	var resp struct {
		Data []uint32 `json:"data"`
	}
	_ = json.Unmarshal(out, &resp)

	notificationID := 0
	if len(resp.Data) > 0 {
		notificationID = int(resp.Data[0])
	}

	return JSONResult(map[string]any{
		"ok":      true,
		"summary": summary,
		"body":    params.Body,
		"id":      notificationID,
	})
}
