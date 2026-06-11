package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	mprisPrefix = "org.mpris.MediaPlayer2."
	mprisPath   = "/org/mpris/MediaPlayer2"
	mprisPlayer = "org.mpris.MediaPlayer2.Player"
)

type mediaControlTool struct {
	run commandRunner
}

func NewMediaControlTool() Tool {
	return &mediaControlTool{run: defaultCommandRunner}
}

func (t *mediaControlTool) Name() string { return "media_control" }

func (t *mediaControlTool) Description() string {
	return "Control MPRIS media players like Spotify, Music, or browsers. Supports play, pause, play_pause, stop, next, previous, and status."
}

func (t *mediaControlTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"action": {
				Type:        "string",
				Description: "Media action to run.",
				Enum:        []string{"play", "pause", "play_pause", "stop", "next", "previous", "status"},
			},
			"player": {
				Type:        "string",
				Description: "Optional player name or DBus service substring, for example spotify. If omitted, the first available MPRIS player is used.",
			},
		},
		Required: []string{"action"},
	}
}

func (t *mediaControlTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Action string `json:"action"`
		Player string `json:"player"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	action := strings.ToLower(strings.TrimSpace(params.Action))
	methods := map[string]string{
		"play":       "Play",
		"pause":      "Pause",
		"play_pause": "PlayPause",
		"stop":       "Stop",
		"next":       "Next",
		"previous":   "Previous",
	}
	if action != "status" {
		if _, ok := methods[action]; !ok {
			return ErrorResult("action must be one of: play, pause, play_pause, stop, next, previous, status")
		}
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	players, err := listMPRISPlayers(ctx, run)
	if err != nil {
		return ErrorResult(fmt.Sprintf("could not list media players: %v", err))
	}
	player, err := resolveMPRISPlayer(players, params.Player)
	if err != nil {
		return ErrorResult(err.Error())
	}

	if action == "status" {
		status := getMPRISStringProperty(ctx, run, player, "PlaybackStatus")
		return JSONResult(map[string]any{
			"ok":              true,
			"player":          player,
			"playback_status": status,
			"available":       players,
		})
	}

	if _, err := run(ctx, "busctl", "--user", "call", player, mprisPath, mprisPlayer, methods[action]); err != nil {
		return ErrorResult(fmt.Sprintf("media %s failed for %s: %v", action, player, err))
	}

	return JSONResult(map[string]any{
		"ok":        true,
		"player":    player,
		"action":    action,
		"available": players,
	})
}

func listMPRISPlayers(ctx context.Context, run commandRunner) ([]string, error) {
	out, err := run(ctx, "busctl", "--user", "--no-legend", "list")
	if err != nil {
		return nil, err
	}
	var players []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if strings.HasPrefix(fields[0], mprisPrefix) {
			players = append(players, fields[0])
		}
	}
	return players, nil
}

func resolveMPRISPlayer(players []string, requested string) (string, error) {
	if len(players) == 0 {
		return "", fmt.Errorf("no MPRIS media players are currently available")
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return players[0], nil
	}

	requestedLower := strings.ToLower(requested)
	for _, player := range players {
		if strings.EqualFold(player, requested) || strings.EqualFold(strings.TrimPrefix(player, mprisPrefix), requested) {
			return player, nil
		}
	}
	for _, player := range players {
		if strings.Contains(strings.ToLower(player), requestedLower) {
			return player, nil
		}
	}
	return "", fmt.Errorf("media player %q not found; available players: %s", requested, strings.Join(players, ", "))
}

func getMPRISStringProperty(ctx context.Context, run commandRunner, player, prop string) string {
	out, err := run(ctx, "busctl", "--user", "--json=short", "get-property", player, mprisPath, mprisPlayer, prop)
	if err != nil {
		return ""
	}
	var resp struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return ""
	}
	return resp.Data
}
