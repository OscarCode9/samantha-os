package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func decodeToolJSONResult(t *testing.T, result Result) map[string]any {
	t.Helper()
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode result JSON: %v\n%s", err, result.Content)
	}
	return payload
}

func TestGetBatteryStatusToolNoBattery(t *testing.T) {
	tool := &getBatteryStatusTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "busctl" {
				t.Fatalf("unexpected command: %s", name)
			}
			switch {
			case reflect.DeepEqual(args, []string{"--json=short", "call", upowerBus, upowerPath, upowerBus, "GetDisplayDevice"}):
				return []byte(`{"type":"o","data":["/org/freedesktop/UPower/devices/DisplayDevice"]}`), nil
			case reflect.DeepEqual(args, []string{"--json=short", "get-property", upowerBus, "/org/freedesktop/UPower/devices/DisplayDevice", upowerDevIface, "IsPresent"}):
				return []byte(`{"type":"b","data":false}`), nil
			default:
				t.Fatalf("unexpected busctl args: %#v", args)
				return nil, nil
			}
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{}`))
	if payload["present"] != false {
		t.Fatalf("expected no battery, got %#v", payload)
	}
	if payload["status"] != "no battery detected" {
		t.Fatalf("unexpected status: %#v", payload["status"])
	}
}

func TestGetBatteryStatusToolDetails(t *testing.T) {
	tool := &getBatteryStatusTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "busctl" {
				t.Fatalf("unexpected command: %s", name)
			}
			prop := args[len(args)-1]
			switch {
			case reflect.DeepEqual(args, []string{"--json=short", "call", upowerBus, upowerPath, upowerBus, "GetDisplayDevice"}):
				return []byte(`{"type":"o","data":["/org/freedesktop/UPower/devices/DisplayDevice"]}`), nil
			case prop == "IsPresent":
				return []byte(`{"type":"b","data":true}`), nil
			case prop == "Percentage":
				return []byte(`{"type":"d","data":84.28}`), nil
			case prop == "State":
				return []byte(`{"type":"u","data":1}`), nil
			case prop == "TimeToFull":
				return []byte(`{"type":"x","data":3600}`), nil
			case prop == "TimeToEmpty":
				return []byte(`{"type":"x","data":0}`), nil
			default:
				t.Fatalf("unexpected property request: %#v", args)
				return nil, nil
			}
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{}`))
	if payload["state"] != "charging" {
		t.Fatalf("unexpected state: %#v", payload["state"])
	}
	if payload["percentage"] != 84.3 {
		t.Fatalf("unexpected percentage: %#v", payload["percentage"])
	}
	if payload["time_to_full_minutes"] != float64(60) {
		t.Fatalf("unexpected time_to_full_minutes: %#v", payload["time_to_full_minutes"])
	}
}

func TestSendNotificationToolCallsDBus(t *testing.T) {
	var gotArgs []string
	tool := &sendNotificationTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			gotArgs = append([]string{name}, args...)
			return []byte(`{"type":"u","data":[42]}`), nil
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"summary":"Hola","body":"Mundo"}`))
	if payload["id"] != float64(42) {
		t.Fatalf("unexpected notification id: %#v", payload["id"])
	}
	if !reflect.DeepEqual(gotArgs[:5], []string{"busctl", "--user", "--json=short", "call", notificationsBus}) {
		t.Fatalf("unexpected busctl prefix: %#v", gotArgs)
	}
	if gotArgs[len(gotArgs)-1] != "5000" {
		t.Fatalf("expected default 5s timeout, got %#v", gotArgs[len(gotArgs)-1])
	}
}

func TestGetNetworkStatusToolIncludesActiveConnections(t *testing.T) {
	tool := &getNetworkStatusTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "busctl" {
				t.Fatalf("unexpected command: %s", name)
			}
			joined := strings.Join(args, " ")
			switch {
			case strings.Contains(joined, "Get ss org.freedesktop.NetworkManager State"):
				return []byte(`{"type":"v","data":[{"type":"u","data":70}]}`), nil
			case strings.Contains(joined, "Get ss org.freedesktop.NetworkManager Connectivity"):
				return []byte(`{"type":"v","data":[{"type":"u","data":4}]}`), nil
			case strings.Contains(joined, "Get ss org.freedesktop.NetworkManager ActiveConnections"):
				return []byte(`{"type":"v","data":[{"type":"ao","data":["/conn/1","/conn/2"]}]}`), nil
			case strings.Contains(joined, "get-property "+nmBus+" /conn/1 "+nmActiveConnIface+" Id"):
				return []byte(`{"type":"s","data":"Office WiFi"}`), nil
			case strings.Contains(joined, "get-property "+nmBus+" /conn/1 "+nmActiveConnIface+" Type"):
				return []byte(`{"type":"s","data":"802-11-wireless"}`), nil
			case strings.Contains(joined, "get-property "+nmBus+" /conn/2 "+nmActiveConnIface+" Id"):
				return []byte(`{"type":"s","data":"VPN"}`), nil
			case strings.Contains(joined, "get-property "+nmBus+" /conn/2 "+nmActiveConnIface+" Type"):
				return []byte(`{"type":"s","data":"vpn"}`), nil
			default:
				t.Fatalf("unexpected busctl args: %#v", args)
				return nil, nil
			}
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{}`))
	if payload["state"] != "connected_global" || payload["connectivity"] != "full" {
		t.Fatalf("unexpected network summary: %#v", payload)
	}
	connections, ok := payload["active_connections"].([]any)
	if !ok || len(connections) != 2 {
		t.Fatalf("unexpected active connections: %#v", payload["active_connections"])
	}
	first := connections[0].(map[string]any)
	if first["id"] != "Office WiFi" || first["type"] != "802-11-wireless" {
		t.Fatalf("unexpected first connection: %#v", first)
	}
}

func TestOpenFolderToolResolvesRelativePathIntoUserHome(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	var gotURI string

	tool := &openFolderTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "busctl" {
				t.Fatalf("unexpected command: %s", name)
			}
			gotURI = args[len(args)-2]
			return []byte("ok"), nil
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"path":"Downloads/Test Space"}`))
	if gotURI != "file:///home/tester/Downloads/Test%20Space" {
		t.Fatalf("unexpected URI: %s", gotURI)
	}
	if payload["uri"] != gotURI {
		t.Fatalf("result did not echo URI: %#v", payload)
	}
}

func TestGetCurrentUserToolReadsAccountsService(t *testing.T) {
	t.Setenv("USER", "tester")
	t.Setenv("HOME", "/home/tester")
	t.Setenv("LANG", "es_MX.UTF-8")

	tool := &getCurrentUserTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "busctl" {
				t.Fatalf("unexpected command: %s", name)
			}
			joined := strings.Join(args, " ")
			switch {
			case strings.Contains(joined, "FindUserByName s tester"):
				return []byte(`{"type":"o","data":["/org/freedesktop/Accounts/User9999"]}`), nil
			case strings.HasSuffix(joined, "RealName"):
				return []byte(`{"type":"s","data":"Test User"}`), nil
			case strings.HasSuffix(joined, "UserName"):
				return []byte(`{"type":"s","data":"tester"}`), nil
			case strings.HasSuffix(joined, "HomeDirectory"):
				return []byte(`{"type":"s","data":"/home/tester"}`), nil
			case strings.HasSuffix(joined, "Language"):
				return []byte(`{"type":"s","data":"es_MX.UTF-8"}`), nil
			case strings.HasSuffix(joined, "IconFile"):
				return []byte(`{"type":"s","data":"/home/tester/.face"}`), nil
			case strings.HasSuffix(joined, "Email"):
				return []byte(`{"type":"s","data":"tester@example.com"}`), nil
			case strings.HasSuffix(joined, "SessionType"):
				return []byte(`{"type":"s","data":"wayland"}`), nil
			default:
				t.Fatalf("unexpected busctl args: %#v", args)
				return nil, nil
			}
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{}`))
	if payload["real_name"] != "Test User" || payload["icon_file"] != "/home/tester/.face" {
		t.Fatalf("unexpected user payload: %#v", payload)
	}
}

func TestGetCurrentUserToolFallsBackWithoutAccountsService(t *testing.T) {
	t.Setenv("USER", "tester")
	t.Setenv("HOME", "/home/tester")
	t.Setenv("LANG", "es_MX.UTF-8")

	tool := &getCurrentUserTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("%s unavailable", name)
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{}`))
	if payload["username"] != "tester" || payload["home"] != "/home/tester" {
		t.Fatalf("unexpected fallback payload: %#v", payload)
	}
}

func TestTakeScreenshotToolSanitizesFilenameIntoPictures(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	var screenshotCall []string

	tool := &takeScreenshotTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			switch name {
			case "mkdir":
				return []byte{}, nil
			case "busctl":
				screenshotCall = append([]string{name}, args...)
				return []byte(`{"type":"bs","data":[true,"/home/tester/Pictures/report.png"]}`), nil
			default:
				t.Fatalf("unexpected command: %s %#v", name, args)
				return nil, nil
			}
		},
	}

	result := tool.Execute(context.Background(), `{"filename":"nested/report","include_cursor":true}`)
	payload := decodeToolJSONResult(t, result)
	if payload["path"] != "/home/tester/Pictures/report.png" {
		t.Fatalf("unexpected screenshot path: %#v", payload)
	}
	if screenshotCall[len(screenshotCall)-1] != "/home/tester/Pictures/report.png" {
		t.Fatalf("unexpected screenshot destination: %#v", screenshotCall)
	}
	if screenshotCall[len(screenshotCall)-3] != "true" {
		t.Fatalf("expected include_cursor=true in busctl call: %#v", screenshotCall)
	}
	if len(result.Attachments) != 1 || result.Attachments[0].Path != "/home/tester/Pictures/report.png" || result.Attachments[0].MimeType != "image/png" {
		t.Fatalf("expected screenshot image attachment, got %#v", result.Attachments)
	}
}

func TestGetActiveWindowToolUsesGnomeShell(t *testing.T) {
	tool := &getActiveWindowTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "busctl" {
				t.Fatalf("unexpected command: %s", name)
			}
			if !reflect.DeepEqual(args[:5], []string{"--user", "--json=short", "call", gnomeShellBus, gnomeShellPath}) {
				t.Fatalf("unexpected GNOME Shell call: %#v", args)
			}
			return []byte(`{"type":"(bs)","data":[true,"{\"active\":true,\"source\":\"gnome-shell\",\"title\":\"Budget.ods\",\"app_name\":\"LibreOffice Calc\",\"app_id\":\"libreoffice-calc.desktop\",\"pid\":4242}"]}`), nil
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{}`))
	if payload["app_name"] != "LibreOffice Calc" || payload["title"] != "Budget.ods" {
		t.Fatalf("unexpected active window payload: %#v", payload)
	}
}

func TestGetActiveWindowToolFallsBackToATSPI(t *testing.T) {
	const atspiAddress = "unix:path=/tmp/at-spi-test"

	tool := &getActiveWindowTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "busctl" {
				t.Fatalf("unexpected command: %s", name)
			}
			joined := strings.Join(args, " ")
			switch {
			case strings.Contains(joined, gnomeShellBus):
				return nil, fmt.Errorf("gnome shell eval unavailable")
			case reflect.DeepEqual(args, []string{"--user", "--json=short", "call", atspiBus, atspiBusPath, atspiBusIface, "GetAddress"}):
				return []byte(`{"type":"s","data":["` + atspiAddress + `"]}`), nil
			case reflect.DeepEqual(args, []string{"--address=" + atspiAddress, "--json=short", "call", atspiRegistryBus, atspiRootPath, atspiAccessibleIface, "GetChildren"}):
				return []byte(`{"type":"a(so)","data":[[[":1.26","/org/a11y/atspi/accessible/root"]]]}`), nil
			case reflect.DeepEqual(args, []string{"--address=" + atspiAddress, "--json=short", "get-property", ":1.26", "/org/a11y/atspi/accessible/root", atspiAccessibleIface, "Name"}):
				return []byte(`{"type":"s","data":"io.elementary.files"}`), nil
			case reflect.DeepEqual(args, []string{"--address=" + atspiAddress, "--json=short", "call", ":1.26", "/org/a11y/atspi/accessible/root", atspiAccessibleIface, "GetChildren"}):
				return []byte(`{"type":"a(so)","data":[[[":1.26","/org/a11y/atspi/accessible/1"]]]}`), nil
			case reflect.DeepEqual(args, []string{"--address=" + atspiAddress, "--json=short", "call", ":1.26", "/org/a11y/atspi/accessible/1", atspiAccessibleIface, "GetRoleName"}):
				return []byte(`{"type":"s","data":["frame"]}`), nil
			case reflect.DeepEqual(args, []string{"--address=" + atspiAddress, "--json=short", "call", ":1.26", "/org/a11y/atspi/accessible/1", atspiAccessibleIface, "GetState"}):
				return []byte(`{"type":"au","data":[[2,0]]}`), nil
			case reflect.DeepEqual(args, []string{"--address=" + atspiAddress, "--json=short", "get-property", ":1.26", "/org/a11y/atspi/accessible/1", atspiAccessibleIface, "Name"}):
				return []byte(`{"type":"s","data":"/home/tester/Downloads"}`), nil
			default:
				t.Fatalf("unexpected busctl args: %#v", args)
				return nil, nil
			}
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{}`))
	if payload["source"] != "at-spi" || payload["app_name"] != "io.elementary.files" || payload["title"] != "/home/tester/Downloads" {
		t.Fatalf("unexpected AT-SPI active window payload: %#v", payload)
	}
}

func TestGetActiveWindowToolFallsBackToXProp(t *testing.T) {
	tool := &getActiveWindowTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			switch name {
			case "busctl", "xdotool":
				return nil, fmt.Errorf("%s unavailable", name)
			case "sh":
				if !reflect.DeepEqual(args[:1], []string{"-c"}) {
					t.Fatalf("unexpected shell args: %#v", args)
				}
				command := args[1]
				switch {
				case strings.Contains(command, "xprop -root _NET_ACTIVE_WINDOW"):
					return []byte("_NET_ACTIVE_WINDOW(WINDOW): window id # 0x1200007\n"), nil
				case strings.Contains(command, "xprop -id '0x1200007'"):
					return []byte("_NET_WM_NAME(UTF8_STRING) = \"Files\"\nWM_CLASS(STRING) = \"io.elementary.files\", \"Io.elementary.files\"\n_NET_WM_PID(CARDINAL) = 2222\n"), nil
				default:
					t.Fatalf("unexpected xprop command: %s", command)
				}
			default:
				t.Fatalf("unexpected command: %s %#v", name, args)
			}
			return nil, nil
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{}`))
	if payload["source"] != "xprop" || payload["title"] != "Files" || payload["pid"] != float64(2222) {
		t.Fatalf("unexpected xprop active window payload: %#v", payload)
	}
}

func TestAudioVolumeToolSetsVolumeAndReportsState(t *testing.T) {
	var setCall []string
	tool := &audioVolumeTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "pactl" {
				t.Fatalf("unexpected command: %s", name)
			}
			joined := strings.Join(args, " ")
			switch {
			case joined == "set-sink-volume @DEFAULT_SINK@ 60%":
				setCall = append([]string{name}, args...)
				return []byte{}, nil
			case joined == "get-sink-volume @DEFAULT_SINK@":
				return []byte("Volume: front-left: 39322 / 60% / -13.30 dB, front-right: 39322 / 60% / -13.30 dB\n"), nil
			case joined == "get-sink-mute @DEFAULT_SINK@":
				return []byte("Mute: no\n"), nil
			default:
				t.Fatalf("unexpected pactl args: %#v", args)
				return nil, nil
			}
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"action":"set","volume_percent":60}`))
	if payload["volume_percent"] != float64(60) || payload["muted"] != false {
		t.Fatalf("unexpected audio payload: %#v", payload)
	}
	if !reflect.DeepEqual(setCall, []string{"pactl", "set-sink-volume", "@DEFAULT_SINK@", "60%"}) {
		t.Fatalf("unexpected set call: %#v", setCall)
	}
}

func TestAudioVolumeToolFallsBackToWPCTL(t *testing.T) {
	var wpSetCall []string
	tool := &audioVolumeTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			joined := strings.Join(append([]string{name}, args...), " ")
			switch joined {
			case "pactl set-sink-volume @DEFAULT_SINK@ 60%":
				return nil, fmt.Errorf("pactl unavailable")
			case "wpctl set-volume @DEFAULT_AUDIO_SINK@ 0.60":
				wpSetCall = append([]string{name}, args...)
				return []byte{}, nil
			case "pactl get-sink-volume @DEFAULT_SINK@":
				return nil, fmt.Errorf("pactl unavailable")
			case "pactl get-sink-mute @DEFAULT_SINK@":
				return nil, fmt.Errorf("pactl unavailable")
			case "wpctl get-volume @DEFAULT_AUDIO_SINK@":
				return []byte("Volume: 0.60 [MUTED]\n"), nil
			default:
				t.Fatalf("unexpected command: %s %#v", name, args)
				return nil, nil
			}
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"action":"set","volume_percent":60}`))
	if payload["backend"] != "wpctl" || payload["volume_percent"] != float64(60) || payload["muted"] != true {
		t.Fatalf("unexpected wpctl audio payload: %#v", payload)
	}
	if !reflect.DeepEqual(wpSetCall, []string{"wpctl", "set-volume", "@DEFAULT_AUDIO_SINK@", "0.60"}) {
		t.Fatalf("unexpected wpctl set call: %#v", wpSetCall)
	}
}

func TestMediaControlToolPausesRequestedPlayer(t *testing.T) {
	var gotCall []string
	tool := &mediaControlTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "busctl" {
				t.Fatalf("unexpected command: %s", name)
			}
			if reflect.DeepEqual(args, []string{"--user", "--no-legend", "list"}) {
				return []byte("org.mpris.MediaPlayer2.firefox 123 user :1.1 - -\norg.mpris.MediaPlayer2.spotify 456 user :1.2 - -\n"), nil
			}
			gotCall = append([]string{name}, args...)
			return []byte{}, nil
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"action":"pause","player":"spotify"}`))
	if payload["player"] != "org.mpris.MediaPlayer2.spotify" {
		t.Fatalf("unexpected media player: %#v", payload)
	}
	wantSuffix := []string{"org.mpris.MediaPlayer2.spotify", mprisPath, mprisPlayer, "Pause"}
	if !reflect.DeepEqual(gotCall[len(gotCall)-4:], wantSuffix) {
		t.Fatalf("unexpected pause call: %#v", gotCall)
	}
}

func TestInhibitSleepToolStartsBackgroundInhibitor(t *testing.T) {
	var gotCommand string
	tool := &inhibitSleepTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "sh" || !reflect.DeepEqual(args[:1], []string{"-c"}) {
				t.Fatalf("unexpected command: %s %#v", name, args)
			}
			gotCommand = args[1]
			return []byte("12345\n"), nil
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"reason":"Estoy en llamada","duration_minutes":45}`))
	if payload["pid"] != float64(12345) || payload["duration_minutes"] != float64(45) {
		t.Fatalf("unexpected inhibition payload: %#v", payload)
	}
	if !strings.Contains(gotCommand, "systemd-inhibit --what=sleep:idle") || !strings.Contains(gotCommand, "--why='Estoy en llamada'") || !strings.Contains(gotCommand, "sleep 2700") {
		t.Fatalf("unexpected inhibition command: %s", gotCommand)
	}
}

func TestConcentrationModeToolEnablesNativeDNDAndSleepInhibition(t *testing.T) {
	var calls [][]string
	tool := &concentrationModeTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			call := append([]string{name}, args...)
			calls = append(calls, call)
			switch name {
			case "gsettings":
				return []byte{}, nil
			case "sh":
				return []byte("4242\n"), nil
			default:
				return nil, fmt.Errorf("unexpected command: %s", name)
			}
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{}`))
	if payload["ok"] != true || payload["enabled"] != true || payload["do_not_disturb"] != true {
		t.Fatalf("unexpected concentration payload: %#v", payload)
	}

	inhibition, ok := payload["sleep_inhibition"].(map[string]any)
	if !ok {
		t.Fatalf("missing sleep inhibition payload: %#v", payload)
	}
	if inhibition["pid"] != float64(4242) || inhibition["duration_minutes"] != float64(120) {
		t.Fatalf("unexpected sleep inhibition payload: %#v", inhibition)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %#v", calls)
	}
	if !reflect.DeepEqual(calls[0], []string{"gsettings", "set", elementaryNotificationsSchema, elementaryNotificationsDoNotDisturbKey, "true"}) {
		t.Fatalf("unexpected DND call: %#v", calls[0])
	}
	if calls[1][0] != "sh" || len(calls[1]) != 3 || calls[1][1] != "-c" {
		t.Fatalf("unexpected inhibition call: %#v", calls[1])
	}
	if !strings.Contains(calls[1][2], "systemd-inhibit --what=sleep:idle") || !strings.Contains(calls[1][2], "--why='Concentration mode requested by Samantha'") || !strings.Contains(calls[1][2], "sleep 7200") {
		t.Fatalf("unexpected inhibition command: %s", calls[1][2])
	}
}

func TestConcentrationModeToolDisablesNativeDNDWithoutSleepInhibition(t *testing.T) {
	var calls [][]string
	tool := &concentrationModeTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			call := append([]string{name}, args...)
			calls = append(calls, call)
			if name != "gsettings" {
				return nil, fmt.Errorf("unexpected command: %s", name)
			}
			return []byte{}, nil
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"enabled":false}`))
	if payload["ok"] != true || payload["enabled"] != false || payload["do_not_disturb"] != false {
		t.Fatalf("unexpected disable payload: %#v", payload)
	}
	if len(calls) != 1 {
		t.Fatalf("expected a single DND call, got %#v", calls)
	}
	if !reflect.DeepEqual(calls[0], []string{"gsettings", "set", elementaryNotificationsSchema, elementaryNotificationsDoNotDisturbKey, "false"}) {
		t.Fatalf("unexpected disable call: %#v", calls[0])
	}
}

func TestConcentrationModeToolRollsBackDNDWhenSleepInhibitionFails(t *testing.T) {
	var calls [][]string
	tool := &concentrationModeTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			call := append([]string{name}, args...)
			calls = append(calls, call)
			switch {
			case name == "gsettings" && len(args) == 4 && args[3] == "true":
				return []byte{}, nil
			case name == "sh":
				return nil, fmt.Errorf("systemd-inhibit missing")
			case name == "gsettings" && len(args) == 4 && args[3] == "false":
				return []byte{}, nil
			default:
				return nil, fmt.Errorf("unexpected command: %s %#v", name, args)
			}
		},
	}

	result := tool.Execute(context.Background(), `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result, got %#v", result)
	}
	if !strings.Contains(result.Content, "native Do Not Disturb was reverted") {
		t.Fatalf("expected rollback message, got %q", result.Content)
	}
	if len(calls) != 3 {
		t.Fatalf("expected enable, inhibit, rollback calls; got %#v", calls)
	}
	if !reflect.DeepEqual(calls[0], []string{"gsettings", "set", elementaryNotificationsSchema, elementaryNotificationsDoNotDisturbKey, "true"}) {
		t.Fatalf("unexpected initial DND call: %#v", calls[0])
	}
	if !reflect.DeepEqual(calls[2], []string{"gsettings", "set", elementaryNotificationsSchema, elementaryNotificationsDoNotDisturbKey, "false"}) {
		t.Fatalf("unexpected rollback call: %#v", calls[2])
	}
}

func TestConnectWiFiToolBuildsNMCLICommand(t *testing.T) {
	var gotArgs []string
	tool := &connectWiFiTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "nmcli" {
				t.Fatalf("unexpected command: %s", name)
			}
			gotArgs = append([]string{}, args...)
			return []byte("Device 'wlan0' successfully activated\n"), nil
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"ssid":"Office WiFi","password":"secret","interface":"wlan0","rescan":false}`))
	want := []string{"device", "wifi", "connect", "Office WiFi", "password", "secret", "ifname", "wlan0"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("unexpected nmcli args: %#v", gotArgs)
	}
	if payload["ssid"] != "Office WiFi" {
		t.Fatalf("unexpected Wi-Fi payload: %#v", payload)
	}
}

func TestListWiFiNetworksToolParsesNMCLIOutput(t *testing.T) {
	var gotArgs []string
	tool := &listWiFiNetworksTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "nmcli" {
				t.Fatalf("unexpected command: %s", name)
			}
			gotArgs = append([]string{}, args...)
			return []byte("*:Office\\:Main:82:WPA2\n:Guest:44:\n"), nil
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"rescan":false}`))
	wantArgs := []string{"-t", "-f", "IN-USE,SSID,SIGNAL,SECURITY", "device", "wifi", "list", "--rescan", "no"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("unexpected nmcli args: %#v", gotArgs)
	}
	networks, ok := payload["networks"].([]any)
	if !ok || len(networks) != 2 {
		t.Fatalf("unexpected networks payload: %#v", payload)
	}
	first := networks[0].(map[string]any)
	if first["ssid"] != "Office:Main" || first["signal"] != float64(82) || first["active"] != true {
		t.Fatalf("unexpected first network: %#v", first)
	}
}

func TestBluetoothDeviceToolConnectsByName(t *testing.T) {
	var connectArgs []string
	tool := &bluetoothDeviceTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "bluetoothctl" {
				t.Fatalf("unexpected command: %s", name)
			}
			joined := strings.Join(args, " ")
			switch joined {
			case "devices":
				return []byte("Device AA:BB:CC:DD:EE:01 Keyboard\nDevice AA:BB:CC:DD:EE:02 Oscar Headphones\n"), nil
			case "paired-devices":
				return []byte("Device AA:BB:CC:DD:EE:02 Oscar Headphones\n"), nil
			case "connect AA:BB:CC:DD:EE:02":
				connectArgs = append([]string{}, args...)
				return []byte("Connection successful\n"), nil
			default:
				t.Fatalf("unexpected bluetoothctl args: %#v", args)
				return nil, nil
			}
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"action":"connect","target":"headphones"}`))
	if payload["address"] != "AA:BB:CC:DD:EE:02" || payload["name"] != "Oscar Headphones" {
		t.Fatalf("unexpected Bluetooth payload: %#v", payload)
	}
	if !reflect.DeepEqual(connectArgs, []string{"connect", "AA:BB:CC:DD:EE:02"}) {
		t.Fatalf("unexpected connect args: %#v", connectArgs)
	}
}

func TestTrashFileToolResolvesHomeRelativePath(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	var gotPath string
	tool := &trashFileTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "gio" {
				t.Fatalf("unexpected command: %s", name)
			}
			if !reflect.DeepEqual(args[:1], []string{"trash"}) {
				t.Fatalf("unexpected gio args: %#v", args)
			}
			gotPath = args[1]
			return []byte{}, nil
		},
	}

	payload := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"path":"Downloads/report.pdf"}`))
	if gotPath != "/home/tester/Downloads/report.pdf" || payload["path"] != gotPath {
		t.Fatalf("unexpected trash path: got %q payload %#v", gotPath, payload)
	}
}

func TestCleanCacheToolAnalyzeAndDelete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cacheRoot := filepath.Join(home, ".cache")
	flatpakRoot := filepath.Join(home, ".var", "app", "com.example.App", "cache")
	for _, dir := range []string{cacheRoot, flatpakRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cacheRoot, "browser.bin"), bytesOfSize(2048), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flatpakRoot, "thumbs.db"), bytesOfSize(1024), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewCleanCacheTool()
	analyze := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"action":"analyze","limit":10}`))
	if analyze["action"] != "analyze" {
		t.Fatalf("unexpected analyze action: %#v", analyze)
	}
	if analyze["total_bytes"].(float64) < 3000 {
		t.Fatalf("expected cache bytes in analysis, got %#v", analyze["total_bytes"])
	}
	deletePaths := analyze["suggested_delete_paths"].([]any)
	if len(deletePaths) != 2 {
		t.Fatalf("expected two delete roots, got %#v", deletePaths)
	}

	deleteResult := decodeToolJSONResult(t, tool.Execute(context.Background(), fmt.Sprintf(`{"action":"delete","confirm":true,"paths":[%q,%q]}`,
		deletePaths[0].(string), deletePaths[1].(string),
	)))
	if deleteResult["confirmed"] != true {
		t.Fatalf("expected confirmed deletion result: %#v", deleteResult)
	}
	if deleteResult["estimated_bytes_freed"].(float64) < 3000 {
		t.Fatalf("expected freed bytes, got %#v", deleteResult["estimated_bytes_freed"])
	}
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected cache root to be emptied, found %#v", entries)
	}
	entries, err = os.ReadDir(flatpakRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected flatpak cache root to be emptied, found %#v", entries)
	}
}

func TestCleanCacheToolDeleteRequiresConfirmation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cacheRoot := filepath.Join(home, ".cache")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheRoot, "artifact.tmp"), bytesOfSize(128), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewCleanCacheTool()
	result := tool.Execute(context.Background(), fmt.Sprintf(`{"action":"delete","paths":[%q]}`,
		cacheRoot,
	))
	if !result.IsError {
		t.Fatal("expected delete without confirm to fail")
	}
	if !strings.Contains(result.Content, "confirm=true") {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if _, err := os.Stat(filepath.Join(cacheRoot, "artifact.tmp")); err != nil {
		t.Fatalf("expected cache file to remain untouched: %v", err)
	}
}

func TestCleanCacheToolSuperCleanIncludesExtraRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cacheRoot := filepath.Join(home, ".cache")
	trashRoot := filepath.Join(home, ".local", "share", "Trash", "files")
	npmRoot := filepath.Join(home, ".npm", "_cacache")
	for _, dir := range []string{cacheRoot, trashRoot, npmRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cacheRoot, "keep.dat"), bytesOfSize(256), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trashRoot, "old-file.tmp"), bytesOfSize(512), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(npmRoot, "pkg.tgz"), bytesOfSize(1024), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewCleanCacheTool()
	standard := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"action":"analyze"}`))
	standardPaths := toStringSlice(t, standard["suggested_delete_paths"])
	if containsString(standardPaths, trashRoot) || containsString(standardPaths, npmRoot) {
		t.Fatalf("standard mode should not include super-clean roots: %#v", standardPaths)
	}

	super := decodeToolJSONResult(t, tool.Execute(context.Background(), `{"action":"analyze","mode":"super_clean"}`))
	superPaths := toStringSlice(t, super["suggested_delete_paths"])
	if !containsString(superPaths, trashRoot) || !containsString(superPaths, npmRoot) {
		t.Fatalf("super_clean mode should include extra roots: %#v", superPaths)
	}
	if super["mode"] != cleanCacheModeSuperClean {
		t.Fatalf("unexpected mode in result: %#v", super["mode"])
	}
}

func TestCleanCacheToolSuperCleanDeleteRequiresMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	trashRoot := filepath.Join(home, ".local", "share", "Trash", "files")
	if err := os.MkdirAll(trashRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(trashRoot, "old-cache.bin")
	if err := os.WriteFile(target, bytesOfSize(256), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewCleanCacheTool()
	withoutMode := tool.Execute(context.Background(), fmt.Sprintf(`{"action":"delete","confirm":true,"paths":[%q]}`,
		trashRoot,
	))
	if !withoutMode.IsError {
		t.Fatal("expected delete outside standard roots to fail")
	}

	withMode := decodeToolJSONResult(t, tool.Execute(context.Background(), fmt.Sprintf(`{"action":"delete","mode":"super_clean","confirm":true,"paths":[%q]}`,
		trashRoot,
	)))
	if withMode["confirmed"] != true {
		t.Fatalf("expected confirmed deletion: %#v", withMode)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected target to be removed, stat err: %v", err)
	}
}

func bytesOfSize(size int) []byte {
	return []byte(strings.Repeat("x", size))
}

func toStringSlice(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("expected []any, got %#v", value)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		str, ok := item.(string)
		if !ok {
			t.Fatalf("expected string item, got %#v", item)
		}
		result = append(result, str)
	}
	return result
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
