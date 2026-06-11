package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const defaultSleepInhibitionReason = "Requested by Samantha"

type inhibitSleepTool struct {
	run commandRunner
}

func NewInhibitSleepTool() Tool {
	return &inhibitSleepTool{run: defaultCommandRunner}
}

func (t *inhibitSleepTool) Name() string { return "inhibit_sleep" }

func (t *inhibitSleepTool) Description() string {
	return "Prevent the system from sleeping or idling for a limited time, useful during calls, presentations, or long-running work."
}

func (t *inhibitSleepTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"reason": {
				Type:        "string",
				Description: "Human-readable reason for the inhibition. Defaults to 'Requested by Samantha'.",
			},
			"duration_minutes": {
				Type:        "integer",
				Description: "How long to prevent sleep/idle. Defaults to 120 minutes, maximum 480.",
				Minimum:     intPtr(1),
				Maximum:     intPtr(480),
			},
			"mode": {
				Type:        "string",
				Description: "What to inhibit.",
				Enum:        []string{"sleep", "idle", "sleep_and_idle"},
			},
		},
	}
}

func (t *inhibitSleepTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Reason          string `json:"reason"`
		DurationMinutes int    `json:"duration_minutes"`
		Mode            string `json:"mode"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	duration := params.DurationMinutes
	if duration <= 0 {
		duration = 120
	}
	if duration > 480 {
		return ErrorResult("duration_minutes must be 480 or less")
	}

	reason := strings.TrimSpace(params.Reason)
	if reason == "" {
		reason = "Requested by Samantha"
	}

	mode := strings.ToLower(strings.TrimSpace(params.Mode))
	if mode == "" {
		mode = "sleep_and_idle"
	}
	what := map[string]string{
		"sleep":          "sleep",
		"idle":           "idle",
		"sleep_and_idle": "sleep:idle",
	}[mode]
	if what == "" {
		return ErrorResult("mode must be one of: sleep, idle, sleep_and_idle")
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	payload, err := startSleepInhibitor(ctx, run, reason, duration, mode)
	if err != nil {
		return ErrorResult(err.Error())
	}
	return JSONResult(payload)
}

func startSleepInhibitor(ctx context.Context, run commandRunner, reason string, duration int, mode string) (map[string]any, error) {
	if duration <= 0 {
		duration = 120
	}
	if duration > 480 {
		return nil, fmt.Errorf("duration_minutes must be 480 or less")
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = defaultSleepInhibitionReason
	}

	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "sleep_and_idle"
	}
	what := map[string]string{
		"sleep":          "sleep",
		"idle":           "idle",
		"sleep_and_idle": "sleep:idle",
	}[mode]
	if what == "" {
		return nil, fmt.Errorf("mode must be one of: sleep, idle, sleep_and_idle")
	}

	if run == nil {
		run = defaultCommandRunner
	}

	seconds := duration * 60
	command := fmt.Sprintf(
		"systemd-inhibit --what=%s --who=Samantha --why=%s sleep %d >/dev/null 2>&1 & echo $!",
		what,
		shellQuote(reason),
		seconds,
	)
	out, err := run(ctx, "sh", "-c", command)
	if err != nil {
		return nil, fmt.Errorf("sleep inhibition failed: %v", err)
	}

	pidText := strings.TrimSpace(string(out))
	pid, _ := strconv.Atoi(pidText)

	return map[string]any{
		"ok":               true,
		"pid":              pid,
		"duration_minutes": duration,
		"mode":             mode,
		"reason":           reason,
	}, nil
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
