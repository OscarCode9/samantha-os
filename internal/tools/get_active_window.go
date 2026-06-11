package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	gnomeShellBus   = "org.gnome.Shell"
	gnomeShellPath  = "/org/gnome/Shell"
	gnomeShellIface = "org.gnome.Shell"

	atspiBus             = "org.a11y.Bus"
	atspiBusPath         = "/org/a11y/bus"
	atspiBusIface        = "org.a11y.Bus"
	atspiRegistryBus     = "org.a11y.atspi.Registry"
	atspiRootPath        = "/org/a11y/atspi/accessible/root"
	atspiAccessibleIface = "org.a11y.atspi.Accessible"
	atspiStateActive     = 1
)

type getActiveWindowTool struct {
	run commandRunner
}

func NewGetActiveWindowTool() Tool {
	return &getActiveWindowTool{run: defaultCommandRunner}
}

func (t *getActiveWindowTool) Name() string { return "get_active_window" }

func (t *getActiveWindowTool) Description() string {
	return "Get the currently focused window/app: title, app name, app id, WM class, and process id when available. Uses GNOME Shell DBus, AT-SPI accessibility, and X11 fallbacks."
}

func (t *getActiveWindowTool) Parameters() Schema {
	return Schema{Type: "object", Properties: map[string]SchemaProperty{}}
}

func (t *getActiveWindowTool) Execute(ctx context.Context, _ string) Result {
	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	payload, err := activeWindowFromGnomeShell(ctx, run)
	if err == nil {
		return JSONResult(payload)
	}
	gnomeErr := err

	payload, err = activeWindowFromATSPI(ctx, run)
	if err == nil {
		return JSONResult(payload)
	}
	atspiErr := err

	payload, err = activeWindowFromXDoTool(ctx, run)
	if err == nil {
		return JSONResult(payload)
	}
	xdotoolErr := err

	payload, err = activeWindowFromXProp(ctx, run)
	if err == nil {
		return JSONResult(payload)
	}

	return ErrorResult(fmt.Sprintf("could not determine active window: gnome shell: %v; at-spi: %v; xdotool: %v; xprop: %v", gnomeErr, atspiErr, xdotoolErr, err))
}

func activeWindowFromGnomeShell(ctx context.Context, run commandRunner) (map[string]any, error) {
	script := `(function () {
  const Shell = imports.gi.Shell;
  const win = global.display.focus_window;
  if (!win) return JSON.stringify({ active: false, source: "gnome-shell" });
  const app = Shell.WindowTracker.get_default().get_window_app(win);
  return JSON.stringify({
    active: true,
    source: "gnome-shell",
    title: win.get_title ? (win.get_title() || "") : "",
    wm_class: win.get_wm_class ? (win.get_wm_class() || "") : "",
    pid: win.get_pid ? win.get_pid() : 0,
    app_id: app ? (app.get_id() || "") : "",
    app_name: app ? (app.get_name() || "") : ""
  });
})()`

	out, err := run(ctx, "busctl", "--user", "--json=short", "call",
		gnomeShellBus, gnomeShellPath, gnomeShellIface,
		"Eval",
		"s",
		script,
	)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) < 2 {
		return nil, fmt.Errorf("unexpected GNOME Shell Eval response")
	}

	var ok bool
	var raw string
	_ = json.Unmarshal(resp.Data[0], &ok)
	_ = json.Unmarshal(resp.Data[1], &raw)
	if !ok {
		return nil, fmt.Errorf("GNOME Shell Eval returned false: %s", raw)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

type atspiRef struct {
	Destination string
	Path        string
}

func activeWindowFromATSPI(ctx context.Context, run commandRunner) (map[string]any, error) {
	out, err := run(ctx, "busctl", "--user", "--json=short", "call",
		atspiBus, atspiBusPath, atspiBusIface,
		"GetAddress",
	)
	if err != nil {
		return nil, err
	}

	address, err := parseBusctlString(out)
	if err != nil {
		return nil, fmt.Errorf("parse AT-SPI bus address: %w", err)
	}
	if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("AT-SPI bus address is empty")
	}

	apps, err := atspiChildren(ctx, run, address, atspiRegistryBus, atspiRootPath)
	if err != nil {
		return nil, err
	}
	if len(apps) == 0 {
		return nil, fmt.Errorf("AT-SPI registry returned no applications")
	}

	windowCount := 0
	for _, app := range apps {
		appName, _ := atspiName(ctx, run, address, app.Destination, app.Path)
		children, err := atspiChildren(ctx, run, address, app.Destination, app.Path)
		if err != nil {
			continue
		}

		for _, child := range children {
			role, err := atspiRoleName(ctx, run, address, child.Destination, child.Path)
			if err != nil || !isATSPIWindowRole(role) {
				continue
			}
			windowCount++

			state, err := atspiState(ctx, run, address, child.Destination, child.Path)
			if err != nil || !atspiStateHas(state, atspiStateActive) {
				continue
			}

			title, _ := atspiName(ctx, run, address, child.Destination, child.Path)
			return map[string]any{
				"active":   true,
				"source":   "at-spi",
				"app_name": appName,
				"title":    title,
				"role":     strings.TrimSpace(role),
				"bus_name": child.Destination,
				"path":     child.Path,
			}, nil
		}
	}

	return map[string]any{
		"active":       false,
		"source":       "at-spi",
		"app_count":    len(apps),
		"window_count": windowCount,
		"title":        "",
		"note":         "no active accessible window reported",
	}, nil
}

func atspiChildren(ctx context.Context, run commandRunner, address, destination, path string) ([]atspiRef, error) {
	out, err := run(ctx, "busctl", "--address="+address, "--json=short", "call",
		destination, path, atspiAccessibleIface,
		"GetChildren",
	)
	if err != nil {
		return nil, err
	}
	return parseBusctlRefs(out)
}

func atspiName(ctx context.Context, run commandRunner, address, destination, path string) (string, error) {
	out, err := run(ctx, "busctl", "--address="+address, "--json=short", "get-property",
		destination, path, atspiAccessibleIface,
		"Name",
	)
	if err != nil {
		return "", err
	}
	return parseBusctlString(out)
}

func atspiRoleName(ctx context.Context, run commandRunner, address, destination, path string) (string, error) {
	out, err := run(ctx, "busctl", "--address="+address, "--json=short", "call",
		destination, path, atspiAccessibleIface,
		"GetRoleName",
	)
	if err != nil {
		return "", err
	}
	return parseBusctlString(out)
}

func atspiState(ctx context.Context, run commandRunner, address, destination, path string) ([]uint32, error) {
	out, err := run(ctx, "busctl", "--address="+address, "--json=short", "call",
		destination, path, atspiAccessibleIface,
		"GetState",
	)
	if err != nil {
		return nil, err
	}
	return parseBusctlUint32List(out)
}

func isATSPIWindowRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "frame", "window", "dialog", "alert":
		return true
	default:
		return false
	}
}

func atspiStateHas(bits []uint32, state uint) bool {
	index := int(state / 32)
	offset := state % 32
	if index < 0 || index >= len(bits) {
		return false
	}
	return bits[index]&(uint32(1)<<offset) != 0
}

func parseBusctlString(out []byte) (string, error) {
	data, err := parseBusctlData(out)
	if err != nil {
		return "", err
	}
	if value, ok := data.(string); ok {
		return value, nil
	}
	if values, ok := data.([]any); ok && len(values) > 0 {
		if value, ok := values[0].(string); ok {
			return value, nil
		}
	}
	return "", fmt.Errorf("expected string data")
}

func parseBusctlRefs(out []byte) ([]atspiRef, error) {
	data, err := parseBusctlData(out)
	if err != nil {
		return nil, err
	}

	items, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("expected array data")
	}
	if len(items) == 1 {
		if nested, ok := items[0].([]any); ok {
			items = nested
		}
	}

	refs := make([]atspiRef, 0, len(items))
	for _, item := range items {
		pair, ok := item.([]any)
		if !ok || len(pair) < 2 {
			continue
		}
		destination, okDestination := pair[0].(string)
		path, okPath := pair[1].(string)
		if okDestination && okPath && destination != "" && path != "" {
			refs = append(refs, atspiRef{Destination: destination, Path: path})
		}
	}
	return refs, nil
}

func parseBusctlUint32List(out []byte) ([]uint32, error) {
	data, err := parseBusctlData(out)
	if err != nil {
		return nil, err
	}

	items, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("expected array data")
	}
	if len(items) == 1 {
		if nested, ok := items[0].([]any); ok {
			items = nested
		}
	}

	values := make([]uint32, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case float64:
			if typed >= 0 {
				values = append(values, uint32(typed))
			}
		case json.Number:
			if parsed, err := strconv.ParseUint(string(typed), 10, 32); err == nil {
				values = append(values, uint32(parsed))
			}
		}
	}
	return values, nil
}

func parseBusctlData(out []byte) (any, error) {
	var resp struct {
		Data any `json:"data"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	decoder.UseNumber()
	if err := decoder.Decode(&resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func activeWindowFromXDoTool(ctx context.Context, run commandRunner) (map[string]any, error) {
	out, err := run(ctx, "xdotool", "getactivewindow")
	if err != nil {
		return nil, err
	}
	windowID := strings.TrimSpace(string(out))
	if windowID == "" {
		return nil, fmt.Errorf("xdotool returned empty window id")
	}

	title := ""
	if out, err := run(ctx, "xdotool", "getwindowname", windowID); err == nil {
		title = strings.TrimSpace(string(out))
	}

	pid := 0
	if out, err := run(ctx, "xdotool", "getwindowpid", windowID); err == nil {
		if parsed, parseErr := strconv.Atoi(strings.TrimSpace(string(out))); parseErr == nil {
			pid = parsed
		}
	}

	appCommand := ""
	if pid > 0 {
		if out, err := run(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "comm="); err == nil {
			appCommand = strings.TrimSpace(string(out))
		}
	}

	return map[string]any{
		"active":      true,
		"source":      "xdotool",
		"window_id":   windowID,
		"title":       title,
		"pid":         pid,
		"app_command": appCommand,
	}, nil
}

func activeWindowFromXProp(ctx context.Context, run commandRunner) (map[string]any, error) {
	out, err := run(ctx, "sh", "-c", userSessionEnvPrefix()+"xprop -root _NET_ACTIVE_WINDOW")
	if err != nil {
		return nil, err
	}
	windowID := parseXPropWindowID(string(out))
	if windowID == "" || windowID == "0x0" {
		return map[string]any{
			"active": false,
			"source": "xprop",
			"title":  "",
			"note":   "no active X11/XWayland window reported",
		}, nil
	}

	propsOut, err := run(ctx, "sh", "-c", userSessionEnvPrefix()+"xprop -id "+shellQuote(windowID)+" WM_NAME _NET_WM_NAME WM_CLASS _NET_WM_PID")
	if err != nil {
		return map[string]any{
			"active":    true,
			"source":    "xprop",
			"window_id": windowID,
		}, nil
	}

	props := parseXPropWindowProperties(string(propsOut))
	props["active"] = true
	props["source"] = "xprop"
	props["window_id"] = windowID
	return props, nil
}

func userSessionEnvPrefix() string {
	return `DISPLAY="${DISPLAY:-$(systemctl --user show-environment | sed -n 's/^DISPLAY=//p')}"; ` +
		`XAUTHORITY="${XAUTHORITY:-$(systemctl --user show-environment | sed -n 's/^XAUTHORITY=//p')}"; ` +
		`export DISPLAY XAUTHORITY; `
}

func parseXPropWindowID(output string) string {
	idx := strings.LastIndex(output, "#")
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(output[idx+1:])
}

func parseXPropWindowProperties(output string) map[string]any {
	props := map[string]any{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "_NET_WM_NAME"):
			if value := parseXPropQuotedValue(line); value != "" {
				props["title"] = value
			}
		case strings.HasPrefix(line, "WM_NAME"):
			if _, ok := props["title"]; !ok {
				if value := parseXPropQuotedValue(line); value != "" {
					props["title"] = value
				}
			}
		case strings.HasPrefix(line, "WM_CLASS"):
			values := parseXPropQuotedValues(line)
			if len(values) > 0 {
				props["wm_class"] = values[len(values)-1]
				props["app_name"] = values[len(values)-1]
			}
		case strings.HasPrefix(line, "_NET_WM_PID"):
			if idx := strings.LastIndex(line, "="); idx >= 0 {
				if pid, err := strconv.Atoi(strings.TrimSpace(line[idx+1:])); err == nil {
					props["pid"] = pid
				}
			}
		}
	}
	return props
}

func parseXPropQuotedValue(line string) string {
	values := parseXPropQuotedValues(line)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func parseXPropQuotedValues(line string) []string {
	var values []string
	for {
		start := strings.Index(line, `"`)
		if start < 0 {
			return values
		}
		line = line[start+1:]
		end := strings.Index(line, `"`)
		if end < 0 {
			return values
		}
		values = append(values, line[:end])
		line = line[end+1:]
	}
}
