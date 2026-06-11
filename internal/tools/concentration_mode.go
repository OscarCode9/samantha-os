package tools

import (
	"context"
	"fmt"
	"strings"
)

const (
	elementaryNotificationsSchema          = "io.elementary.notifications"
	elementaryNotificationsDoNotDisturbKey = "do-not-disturb"
	defaultConcentrationReason             = "Concentration mode requested by Samantha"
)

type concentrationModeTool struct {
	run commandRunner
}

func NewConcentrationModeTool() Tool {
	return &concentrationModeTool{run: defaultCommandRunner}
}

func (t *concentrationModeTool) Name() string { return "concentration_mode" }

func (t *concentrationModeTool) Description() string {
	return "Enable or disable system concentration mode on elementary OS. Use when the user asks for 'modo concentracion', 'modo concentración', 'focus mode', or native 'No molestar'. Enabling turns on elementary's native Do Not Disturb switch and also prevents sleep/idle for a limited time by default."
}

func (t *concentrationModeTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"enabled": {
				Type:        "boolean",
				Description: "Whether to enable or disable concentration mode. Defaults to true.",
				Default:     true,
			},
			"reason": {
				Type:        "string",
				Description: "Reason shown on the temporary sleep inhibition while concentration mode is enabled.",
			},
			"duration_minutes": {
				Type:        "integer",
				Description: "How long to prevent sleep/idle while concentration mode is enabled. Defaults to 120 minutes, maximum 480.",
				Minimum:     intPtr(1),
				Maximum:     intPtr(480),
			},
		},
	}
}

func (t *concentrationModeTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Enabled         *bool  `json:"enabled"`
		Reason          string `json:"reason"`
		DurationMinutes int    `json:"duration_minutes"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	enabled := true
	if params.Enabled != nil {
		enabled = *params.Enabled
	}

	if err := setElementaryDoNotDisturb(ctx, run, enabled); err != nil {
		return ErrorResult(fmt.Sprintf("concentration mode failed to toggle native Do Not Disturb: %v", err))
	}

	payload := map[string]any{
		"ok":             true,
		"enabled":        enabled,
		"do_not_disturb": enabled,
		"native_schema":  elementaryNotificationsSchema,
		"native_key":     elementaryNotificationsDoNotDisturbKey,
	}

	if !enabled {
		return JSONResult(payload)
	}

	reason := strings.TrimSpace(params.Reason)
	if reason == "" {
		reason = defaultConcentrationReason
	}

	inhibition, err := startSleepInhibitor(ctx, run, reason, params.DurationMinutes, "sleep_and_idle")
	if err != nil {
		if rollbackErr := setElementaryDoNotDisturb(ctx, run, false); rollbackErr != nil {
			return ErrorResult(fmt.Sprintf("concentration mode failed to start sleep inhibition: %v; native Do Not Disturb rollback failed: %v", err, rollbackErr))
		}
		return ErrorResult(fmt.Sprintf("concentration mode failed to start sleep inhibition: %v; native Do Not Disturb was reverted", err))
	}

	payload["sleep_inhibition"] = inhibition
	return JSONResult(payload)
}

func setElementaryDoNotDisturb(ctx context.Context, run commandRunner, enabled bool) error {
	if run == nil {
		run = defaultCommandRunner
	}

	value := "false"
	if enabled {
		value = "true"
	}

	if _, err := run(ctx, "gsettings", "set", elementaryNotificationsSchema, elementaryNotificationsDoNotDisturbKey, value); err != nil {
		return fmt.Errorf("gsettings set %s %s %s: %w", elementaryNotificationsSchema, elementaryNotificationsDoNotDisturbKey, value, err)
	}
	return nil
}