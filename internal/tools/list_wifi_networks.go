package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type listWiFiNetworksTool struct {
	run commandRunner
}

func NewListWiFiNetworksTool() Tool {
	return &listWiFiNetworksTool{run: defaultCommandRunner}
}

func (t *listWiFiNetworksTool) Name() string { return "list_wifi_networks" }

func (t *listWiFiNetworksTool) Description() string {
	return "List visible Wi-Fi networks through NetworkManager, including SSID, signal strength, security, and whether each network is active."
}

func (t *listWiFiNetworksTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"rescan": {
				Type:        "boolean",
				Description: "Whether NetworkManager should rescan before listing. Defaults to true.",
			},
		},
	}
}

func (t *listWiFiNetworksTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Rescan *bool `json:"rescan"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	rescan := true
	if params.Rescan != nil {
		rescan = *params.Rescan
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	rescanArg := "yes"
	if !rescan {
		rescanArg = "no"
	}
	out, err := run(ctx, "nmcli", "-t", "-f", "IN-USE,SSID,SIGNAL,SECURITY", "device", "wifi", "list", "--rescan", rescanArg)
	if err != nil {
		return ErrorResult(fmt.Sprintf("list Wi-Fi networks failed: %v (%s)", err, strings.TrimSpace(string(out))))
	}

	return JSONResult(map[string]any{
		"ok":       true,
		"networks": parseNMCLIWiFiList(string(out)),
	})
}

func parseNMCLIWiFiList(output string) []map[string]any {
	var networks []map[string]any
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := splitEscapedColon(line)
		if len(fields) < 4 {
			continue
		}
		signal, _ := strconv.Atoi(fields[2])
		networks = append(networks, map[string]any{
			"active":   fields[0] == "*",
			"ssid":     fields[1],
			"signal":   signal,
			"security": fields[3],
		})
	}
	return networks
}

func splitEscapedColon(s string) []string {
	var fields []string
	var current strings.Builder
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == ':':
			fields = append(fields, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	fields = append(fields, current.String())
	return fields
}
