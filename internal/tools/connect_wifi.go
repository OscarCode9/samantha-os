package tools

import (
	"context"
	"fmt"
	"strings"
)

type connectWiFiTool struct {
	run commandRunner
}

func NewConnectWiFiTool() Tool {
	return &connectWiFiTool{run: defaultCommandRunner}
}

func (t *connectWiFiTool) Name() string { return "connect_wifi" }

func (t *connectWiFiTool) Description() string {
	return "Connect to a Wi-Fi network through NetworkManager. If the network profile already exists, a password is usually not needed."
}

func (t *connectWiFiTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"ssid": {
				Type:        "string",
				Description: "Wi-Fi SSID to connect to.",
			},
			"password": {
				Type:        "string",
				Description: "Optional Wi-Fi password. Leave empty for known or open networks.",
			},
			"interface": {
				Type:        "string",
				Description: "Optional network interface name, for example wlan0.",
			},
			"rescan": {
				Type:        "boolean",
				Description: "Whether to ask NetworkManager to rescan before connecting. Defaults to true.",
			},
		},
		Required: []string{"ssid"},
	}
}

func (t *connectWiFiTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		SSID      string `json:"ssid"`
		Password  string `json:"password"`
		Interface string `json:"interface"`
		Rescan    *bool  `json:"rescan"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	ssid := strings.TrimSpace(params.SSID)
	if ssid == "" {
		return ErrorResult("ssid must not be empty")
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	if params.Rescan == nil || *params.Rescan {
		_, _ = run(ctx, "nmcli", "device", "wifi", "rescan")
	}

	args := []string{"device", "wifi", "connect", ssid}
	if params.Password != "" {
		args = append(args, "password", params.Password)
	}
	if strings.TrimSpace(params.Interface) != "" {
		args = append(args, "ifname", strings.TrimSpace(params.Interface))
	}

	out, err := run(ctx, "nmcli", args...)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Wi-Fi connection failed: %v (%s)", err, strings.TrimSpace(string(out))))
	}

	return JSONResult(map[string]any{
		"ok":     true,
		"ssid":   ssid,
		"output": strings.TrimSpace(string(out)),
	})
}
