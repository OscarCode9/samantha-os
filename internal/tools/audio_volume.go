package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type audioVolumeTool struct {
	run commandRunner
}

func NewAudioVolumeTool() Tool {
	return &audioVolumeTool{run: defaultCommandRunner}
}

func (t *audioVolumeTool) Name() string { return "audio_volume" }

func (t *audioVolumeTool) Description() string {
	return "Get or change the default output volume using PulseAudio/PipeWire via pactl or wpctl. Supports get, set, mute, unmute, and toggle_mute."
}

func (t *audioVolumeTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"action": {
				Type:        "string",
				Description: "Volume action. Defaults to get unless volume_percent is provided.",
				Enum:        []string{"get", "set", "mute", "unmute", "toggle_mute"},
			},
			"volume_percent": {
				Type:        "integer",
				Description: "Target output volume percentage for action=set. Allowed range: 0-150.",
				Minimum:     intPtr(0),
				Maximum:     intPtr(150),
			},
		},
	}
}

func (t *audioVolumeTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Action        string `json:"action"`
		VolumePercent *int   `json:"volume_percent"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	action := strings.ToLower(strings.TrimSpace(params.Action))
	if action == "" {
		if params.VolumePercent != nil {
			action = "set"
		} else {
			action = "get"
		}
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	backend := "pactl"
	switch action {
	case "get":
	case "set":
		if params.VolumePercent == nil {
			return ErrorResult("volume_percent is required when action is set")
		}
		if *params.VolumePercent < 0 || *params.VolumePercent > 150 {
			return ErrorResult("volume_percent must be between 0 and 150")
		}
		if _, err := run(ctx, "pactl", "set-sink-volume", "@DEFAULT_SINK@", fmt.Sprintf("%d%%", *params.VolumePercent)); err != nil {
			backend = "wpctl"
			wpValue := fmt.Sprintf("%.2f", float64(*params.VolumePercent)/100)
			if _, wpErr := run(ctx, "wpctl", "set-volume", "@DEFAULT_AUDIO_SINK@", wpValue); wpErr != nil {
				return ErrorResult(fmt.Sprintf("set volume failed: pactl: %v; wpctl: %v", err, wpErr))
			}
		}
	case "mute":
		if _, err := run(ctx, "pactl", "set-sink-mute", "@DEFAULT_SINK@", "1"); err != nil {
			backend = "wpctl"
			if _, wpErr := run(ctx, "wpctl", "set-mute", "@DEFAULT_AUDIO_SINK@", "1"); wpErr != nil {
				return ErrorResult(fmt.Sprintf("mute failed: pactl: %v; wpctl: %v", err, wpErr))
			}
		}
	case "unmute":
		if _, err := run(ctx, "pactl", "set-sink-mute", "@DEFAULT_SINK@", "0"); err != nil {
			backend = "wpctl"
			if _, wpErr := run(ctx, "wpctl", "set-mute", "@DEFAULT_AUDIO_SINK@", "0"); wpErr != nil {
				return ErrorResult(fmt.Sprintf("unmute failed: pactl: %v; wpctl: %v", err, wpErr))
			}
		}
	case "toggle_mute":
		if _, err := run(ctx, "pactl", "set-sink-mute", "@DEFAULT_SINK@", "toggle"); err != nil {
			backend = "wpctl"
			if _, wpErr := run(ctx, "wpctl", "set-mute", "@DEFAULT_AUDIO_SINK@", "toggle"); wpErr != nil {
				return ErrorResult(fmt.Sprintf("toggle mute failed: pactl: %v; wpctl: %v", err, wpErr))
			}
		}
	default:
		return ErrorResult("action must be one of: get, set, mute, unmute, toggle_mute")
	}

	volumeOut, volumeErr := run(ctx, "pactl", "get-sink-volume", "@DEFAULT_SINK@")
	muteOut, muteErr := run(ctx, "pactl", "get-sink-mute", "@DEFAULT_SINK@")

	result := map[string]any{
		"ok":      true,
		"action":  action,
		"backend": backend,
	}
	if volumeErr == nil {
		if pct, ok := parsePactlVolumePercent(string(volumeOut)); ok {
			result["volume_percent"] = pct
		}
		result["volume_raw"] = strings.TrimSpace(string(volumeOut))
	}
	if muteErr == nil {
		muted := parsePactlMute(string(muteOut))
		result["muted"] = muted
		result["mute_raw"] = strings.TrimSpace(string(muteOut))
	}
	if volumeErr != nil || muteErr != nil {
		wpOut, wpErr := run(ctx, "wpctl", "get-volume", "@DEFAULT_AUDIO_SINK@")
		if wpErr == nil {
			backend = "wpctl"
			result["backend"] = backend
			if pct, ok := parseWpctlVolumePercent(string(wpOut)); ok {
				result["volume_percent"] = pct
			}
			result["muted"] = parseWpctlMute(string(wpOut))
			result["volume_raw"] = strings.TrimSpace(string(wpOut))
		}
	}

	return JSONResult(result)
}

func parsePactlVolumePercent(output string) (int, bool) {
	idx := strings.Index(output, "%")
	if idx < 0 {
		return 0, false
	}
	start := idx - 1
	for start >= 0 && output[start] >= '0' && output[start] <= '9' {
		start--
	}
	if start == idx-1 {
		return 0, false
	}
	value, err := strconv.Atoi(output[start+1 : idx])
	return value, err == nil
}

func parsePactlMute(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "yes") || strings.Contains(lower, "true") || strings.Contains(lower, "1")
}

func parseWpctlVolumePercent(output string) (int, bool) {
	idx := strings.Index(output, "Volume:")
	if idx < 0 {
		return 0, false
	}
	raw := strings.TrimSpace(output[idx+len("Volume:"):])
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return 0, false
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	return int(value*100 + 0.5), true
}

func parseWpctlMute(output string) bool {
	return strings.Contains(strings.ToUpper(output), "MUTED")
}

func intPtr(v int) *int {
	return &v
}
