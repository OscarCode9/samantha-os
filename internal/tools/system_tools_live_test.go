package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func requireLiveDBus(t *testing.T) {
	t.Helper()
	if os.Getenv("ELEMENTARY_CLAW_LIVE_DBUS") != "1" {
		t.Skip("set ELEMENTARY_CLAW_LIVE_DBUS=1 to run live DBus tests")
	}
	if strings.TrimSpace(os.Getenv("DBUS_SESSION_BUS_ADDRESS")) == "" {
		t.Skip("DBUS_SESSION_BUS_ADDRESS is required for live DBus tests")
	}
}

func requireLiveCommand(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := exec.LookPath(name); err == nil {
			return
		}
	}
	t.Skipf("one of these commands is required for live test: %s", strings.Join(names, ", "))
}

func decodeLiveResult(t *testing.T, result Result) map[string]any {
	t.Helper()
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode live result JSON: %v\n%s", err, result.Content)
	}
	return payload
}

func TestLiveGetBatteryStatusTool(t *testing.T) {
	requireLiveDBus(t)

	tool := NewGetBatteryStatusTool()
	payload := decodeLiveResult(t, tool.Execute(context.Background(), `{}`))
	if _, ok := payload["present"]; !ok {
		t.Fatalf("expected present flag in payload: %#v", payload)
	}
}

func TestLiveGetNetworkStatusTool(t *testing.T) {
	requireLiveDBus(t)

	tool := NewGetNetworkStatusTool()
	payload := decodeLiveResult(t, tool.Execute(context.Background(), `{}`))
	if _, ok := payload["state"]; !ok {
		t.Fatalf("expected state in payload: %#v", payload)
	}
}

func TestLiveGetCurrentUserTool(t *testing.T) {
	requireLiveDBus(t)

	tool := NewGetCurrentUserTool()
	payload := decodeLiveResult(t, tool.Execute(context.Background(), `{}`))
	if strings.TrimSpace(asString(payload["username"])) == "" {
		t.Fatalf("expected username in payload: %#v", payload)
	}
}

func TestLiveSendNotificationTool(t *testing.T) {
	requireLiveDBus(t)

	tool := NewSendNotificationTool()
	payload := decodeLiveResult(t, tool.Execute(context.Background(), `{"summary":"Codex live test","body":"send_notification OK","expire_seconds":2}`))
	if payload["ok"] != true {
		t.Fatalf("expected ok notification result: %#v", payload)
	}
}

func TestLiveOpenFolderTool(t *testing.T) {
	requireLiveDBus(t)

	tool := NewOpenFolderTool()
	payload := decodeLiveResult(t, tool.Execute(context.Background(), `{"path":"~/Downloads"}`))
	if !strings.HasPrefix(asString(payload["uri"]), "file://") {
		t.Fatalf("expected file URI in payload: %#v", payload)
	}
}

func TestLiveTakeScreenshotTool(t *testing.T) {
	requireLiveDBus(t)

	filename := "codex-live-" + time.Now().Format("20060102-150405") + ".png"
	tool := NewTakeScreenshotTool()
	payload := decodeLiveResult(t, tool.Execute(context.Background(), `{"filename":"`+filename+`"}`))

	path := asString(payload["path"])
	if !strings.HasSuffix(path, ".png") {
		t.Fatalf("expected png screenshot path: %#v", payload)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected screenshot file to exist at %s: %v", path, err)
	}
	_ = os.Remove(filepath.Clean(path))
}

func TestLiveGetActiveWindowTool(t *testing.T) {
	requireLiveDBus(t)

	tool := NewGetActiveWindowTool()
	payload := decodeLiveResult(t, tool.Execute(context.Background(), `{}`))
	if _, ok := payload["active"]; !ok {
		t.Fatalf("expected active flag in payload: %#v", payload)
	}
}

func TestLiveAudioVolumeTool(t *testing.T) {
	requireLiveDBus(t)
	requireLiveCommand(t, "pactl", "wpctl")

	tool := NewAudioVolumeTool()
	payload := decodeLiveResult(t, tool.Execute(context.Background(), `{}`))
	if payload["ok"] != true {
		t.Fatalf("expected ok audio result: %#v", payload)
	}
	if _, ok := payload["volume_percent"]; !ok {
		t.Fatalf("expected volume_percent in payload: %#v", payload)
	}
}

func TestLiveAudioVolumeToolSetAndRestore(t *testing.T) {
	requireLiveDBus(t)
	requireLiveCommand(t, "pactl", "wpctl")

	tool := NewAudioVolumeTool()
	before := decodeLiveResult(t, tool.Execute(context.Background(), `{}`))
	beforePercent, _ := before["volume_percent"].(float64)
	beforeMuted, _ := before["muted"].(bool)

	defer func() {
		_ = tool.Execute(context.Background(), `{"action":"set","volume_percent":`+strconv.Itoa(int(beforePercent))+`}`)
		if beforeMuted {
			_ = tool.Execute(context.Background(), `{"action":"mute"}`)
		} else {
			_ = tool.Execute(context.Background(), `{"action":"unmute"}`)
		}
	}()

	after := decodeLiveResult(t, tool.Execute(context.Background(), `{"action":"set","volume_percent":60}`))
	if after["ok"] != true || after["volume_percent"] != float64(60) {
		t.Fatalf("expected volume to be set to 60: %#v", after)
	}
}

func TestLiveListWiFiNetworksTool(t *testing.T) {
	requireLiveDBus(t)
	requireLiveCommand(t, "nmcli")

	tool := NewListWiFiNetworksTool()
	payload := decodeLiveResult(t, tool.Execute(context.Background(), `{"rescan":false}`))
	if _, ok := payload["networks"]; !ok {
		t.Fatalf("expected networks in payload: %#v", payload)
	}
}

func TestLiveBluetoothDeviceToolList(t *testing.T) {
	requireLiveDBus(t)
	requireLiveCommand(t, "bluetoothctl")

	tool := NewBluetoothDeviceTool()
	payload := decodeLiveResult(t, tool.Execute(context.Background(), `{"action":"list"}`))
	if _, ok := payload["devices"]; !ok {
		t.Fatalf("expected devices in payload: %#v", payload)
	}
}

func TestLiveInhibitSleepTool(t *testing.T) {
	requireLiveDBus(t)
	requireLiveCommand(t, "systemd-inhibit")

	tool := NewInhibitSleepTool()
	payload := decodeLiveResult(t, tool.Execute(context.Background(), `{"reason":"Codex live test","duration_minutes":1}`))
	if payload["ok"] != true {
		t.Fatalf("expected ok inhibition result: %#v", payload)
	}
}

func TestLiveTrashFileTool(t *testing.T) {
	requireLiveDBus(t)
	requireLiveCommand(t, "gio")

	dir := t.TempDir()
	path := filepath.Join(dir, "codex-trash-live.txt")
	if err := os.WriteFile(path, []byte("trash me"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewTrashFileTool()
	payload := decodeLiveResult(t, tool.Execute(context.Background(), `{"path":"`+path+`"}`))
	if payload["ok"] != true {
		t.Fatalf("expected ok trash result: %#v", payload)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected trashed file to disappear from original path, stat err: %v", err)
	}
}

func asString(value any) string {
	text, _ := value.(string)
	return text
}
