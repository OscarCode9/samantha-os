package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var bluetoothAddressPattern = regexp.MustCompile(`(?i)^([0-9a-f]{2}:){5}[0-9a-f]{2}$`)

type bluetoothDeviceTool struct {
	run commandRunner
}

func NewBluetoothDeviceTool() Tool {
	return &bluetoothDeviceTool{run: defaultCommandRunner}
}

func (t *bluetoothDeviceTool) Name() string { return "bluetooth_device" }

func (t *bluetoothDeviceTool) Description() string {
	return "Control Bluetooth through BlueZ using bluetoothctl: list devices, power on/off, scan, connect, and disconnect by MAC address or device name."
}

func (t *bluetoothDeviceTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"action": {
				Type:        "string",
				Description: "Bluetooth action to run.",
				Enum:        []string{"list", "power_on", "power_off", "scan_on", "scan_off", "connect", "disconnect"},
			},
			"target": {
				Type:        "string",
				Description: "Device MAC address or case-insensitive name substring for connect/disconnect.",
			},
		},
		Required: []string{"action"},
	}
}

func (t *bluetoothDeviceTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Action string `json:"action"`
		Target string `json:"target"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	action := strings.ToLower(strings.TrimSpace(params.Action))
	switch action {
	case "list":
		devices, err := bluetoothKnownDevices(ctx, run)
		if err != nil {
			return ErrorResult(fmt.Sprintf("Bluetooth list failed: %v", err))
		}
		return JSONResult(map[string]any{"ok": true, "devices": devices})
	case "power_on":
		return bluetoothSimpleAction(ctx, run, action, "power", "on")
	case "power_off":
		return bluetoothSimpleAction(ctx, run, action, "power", "off")
	case "scan_on":
		return bluetoothSimpleAction(ctx, run, action, "scan", "on")
	case "scan_off":
		return bluetoothSimpleAction(ctx, run, action, "scan", "off")
	case "connect", "disconnect":
		address, name, err := resolveBluetoothTarget(ctx, run, params.Target)
		if err != nil {
			return ErrorResult(err.Error())
		}
		out, err := run(ctx, "bluetoothctl", action, address)
		if err != nil {
			return ErrorResult(fmt.Sprintf("Bluetooth %s failed: %v (%s)", action, err, strings.TrimSpace(string(out))))
		}
		return JSONResult(map[string]any{
			"ok":      true,
			"action":  action,
			"address": address,
			"name":    name,
			"output":  strings.TrimSpace(string(out)),
		})
	default:
		return ErrorResult("action must be one of: list, power_on, power_off, scan_on, scan_off, connect, disconnect")
	}
}

func bluetoothSimpleAction(ctx context.Context, run commandRunner, action string, args ...string) Result {
	out, err := run(ctx, "bluetoothctl", args...)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Bluetooth %s failed: %v (%s)", action, err, strings.TrimSpace(string(out))))
	}
	return JSONResult(map[string]any{
		"ok":     true,
		"action": action,
		"output": strings.TrimSpace(string(out)),
	})
}

func bluetoothKnownDevices(ctx context.Context, run commandRunner) ([]map[string]any, error) {
	out, err := run(ctx, "bluetoothctl", "devices")
	if err != nil {
		return nil, err
	}
	allDevices := parseBluetoothDevices(string(out))

	pairedOut, pairedErr := run(ctx, "bluetoothctl", "paired-devices")
	if pairedErr == nil {
		paired := parseBluetoothDevices(string(pairedOut))
		pairedSet := map[string]bool{}
		for _, d := range paired {
			if address, _ := d["address"].(string); address != "" {
				pairedSet[address] = true
			}
		}
		for _, d := range allDevices {
			if address, _ := d["address"].(string); address != "" {
				d["paired"] = pairedSet[address]
			}
		}
	}

	return allDevices, nil
}

func parseBluetoothDevices(output string) []map[string]any {
	var devices []map[string]any
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "Device" {
			continue
		}
		devices = append(devices, map[string]any{
			"address": fields[1],
			"name":    strings.Join(fields[2:], " "),
		})
	}
	return devices
}

func resolveBluetoothTarget(ctx context.Context, run commandRunner, target string) (address string, name string, err error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", fmt.Errorf("target is required for connect/disconnect")
	}
	if bluetoothAddressPattern.MatchString(target) {
		return strings.ToUpper(target), "", nil
	}

	devices, err := bluetoothKnownDevices(ctx, run)
	if err != nil {
		return "", "", fmt.Errorf("could not resolve Bluetooth device %q: %v", target, err)
	}

	targetLower := strings.ToLower(target)
	for _, d := range devices {
		deviceName, _ := d["name"].(string)
		if strings.EqualFold(deviceName, target) {
			address, _ := d["address"].(string)
			return address, deviceName, nil
		}
	}
	for _, d := range devices {
		deviceName, _ := d["name"].(string)
		if strings.Contains(strings.ToLower(deviceName), targetLower) {
			address, _ := d["address"].(string)
			return address, deviceName, nil
		}
	}

	return "", "", fmt.Errorf("Bluetooth device %q not found", target)
}
