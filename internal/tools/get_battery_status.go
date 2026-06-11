package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
)

const (
	upowerBus      = "org.freedesktop.UPower"
	upowerPath     = "/org/freedesktop/UPower"
	upowerDevIface = "org.freedesktop.UPower.Device"
)

type getBatteryStatusTool struct {
	run commandRunner
}

func NewGetBatteryStatusTool() Tool {
	return &getBatteryStatusTool{run: defaultCommandRunner}
}

func (t *getBatteryStatusTool) Name() string { return "get_battery_status" }

func (t *getBatteryStatusTool) Description() string {
	return "Get the current battery status of the device: charge percentage, charging state, and estimated time remaining. Works on laptops and devices with batteries."
}

func (t *getBatteryStatusTool) Parameters() Schema {
	return Schema{Type: "object", Properties: map[string]SchemaProperty{}}
}

func upowerStateLabel(state float64) string {
	switch int(state) {
	case 1:
		return "charging"
	case 2:
		return "discharging"
	case 3:
		return "empty"
	case 4:
		return "fully_charged"
	case 5:
		return "pending_charge"
	case 6:
		return "pending_discharge"
	default:
		return "unknown"
	}
}

func (t *getBatteryStatusTool) Execute(ctx context.Context, _ string) Result {
	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	// Get the display device path.
	out, err := run(ctx, "busctl", "--json=short", "call",
		upowerBus, upowerPath, upowerBus,
		"GetDisplayDevice",
	)
	if err != nil {
		return ErrorResult(fmt.Sprintf("UPower unavailable: %v", err))
	}

	// busctl call returns: {"type":"o","data":["/path/..."]}
	var devResp struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(out, &devResp); err != nil || len(devResp.Data) == 0 || devResp.Data[0] == "" {
		return ErrorResult("could not determine UPower display device path")
	}
	devicePath := devResp.Data[0]

	// readProp fetches a single property value.
	readProp := func(prop string) (any, error) {
		data, err := run(ctx, "busctl", "--json=short", "get-property",
			upowerBus, devicePath, upowerDevIface, prop,
		)
		if err != nil {
			return nil, err
		}
		var result struct {
			Data any `json:"data"`
		}
		if jsonErr := json.Unmarshal(data, &result); jsonErr != nil {
			return nil, jsonErr
		}
		return result.Data, nil
	}

	isPresent, _ := readProp("IsPresent")
	if present, ok := isPresent.(bool); !ok || !present {
		return JSONResult(map[string]any{
			"present": false,
			"status":  "no battery detected",
		})
	}

	percentage, _ := readProp("Percentage")
	state, _ := readProp("State")
	timeToEmpty, _ := readProp("TimeToEmpty")
	timeToFull, _ := readProp("TimeToFull")

	pct, _ := percentage.(float64)
	st, _ := state.(float64)
	tte, _ := timeToEmpty.(float64)
	ttf, _ := timeToFull.(float64)

	result := map[string]any{
		"present":    true,
		"percentage": math.Round(pct*10) / 10,
		"state":      upowerStateLabel(st),
	}
	if tte > 0 {
		result["time_to_empty_minutes"] = int(tte / 60)
	}
	if ttf > 0 {
		result["time_to_full_minutes"] = int(ttf / 60)
	}

	return JSONResult(result)
}
