package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	nmBus             = "org.freedesktop.NetworkManager"
	nmPath            = "/org/freedesktop/NetworkManager"
	nmIface           = "org.freedesktop.NetworkManager"
	nmDBus            = "org.freedesktop.DBus.Properties"
	nmActiveConnIface = "org.freedesktop.NetworkManager.Connection.Active"
)

type getNetworkStatusTool struct {
	run commandRunner
}

func NewGetNetworkStatusTool() Tool {
	return &getNetworkStatusTool{run: defaultCommandRunner}
}

func (t *getNetworkStatusTool) Name() string { return "get_network_status" }

func (t *getNetworkStatusTool) Description() string {
	return "Get the current network connectivity status: whether the device is connected to internet, Wi-Fi, or ethernet, and which network is active."
}

func (t *getNetworkStatusTool) Parameters() Schema {
	return Schema{Type: "object", Properties: map[string]SchemaProperty{}}
}

func nmStateLabel(state float64) string {
	switch int(state) {
	case 0:
		return "unknown"
	case 10:
		return "asleep"
	case 20:
		return "disconnected"
	case 30:
		return "disconnecting"
	case 40:
		return "connecting"
	case 50:
		return "connected_local"
	case 60:
		return "connected_site"
	case 70:
		return "connected_global"
	default:
		return fmt.Sprintf("unknown(%d)", int(state))
	}
}

func nmConnectivityLabel(c float64) string {
	switch int(c) {
	case 1:
		return "none"
	case 2:
		return "portal"
	case 3:
		return "limited"
	case 4:
		return "full"
	default:
		return "unknown"
	}
}

// nmGetProp fetches a single NM property (via Get ss) and returns the inner data value.
func nmGetProp(ctx context.Context, run commandRunner, prop string) (float64, error) {
	out, err := run(ctx, "busctl", "--json=short", "call",
		nmBus, nmPath, nmDBus,
		"Get",
		"ss",
		nmIface,
		prop,
	)
	if err != nil {
		return 0, err
	}
	// busctl call returns variant as: {"type":"v","data":[{"type":"u","data":70}]}
	var resp struct {
		Data []struct {
			Data float64 `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, err
	}
	if len(resp.Data) == 0 {
		return 0, fmt.Errorf("empty response for property %s", prop)
	}
	return resp.Data[0].Data, nil
}

func (t *getNetworkStatusTool) Execute(ctx context.Context, _ string) Result {
	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	state, err := nmGetProp(ctx, run, "State")
	if err != nil {
		return ErrorResult(fmt.Sprintf("NetworkManager unavailable: %v", err))
	}

	connectivity, _ := nmGetProp(ctx, run, "Connectivity")

	result := map[string]any{
		"connected":    int(state) >= 50,
		"internet":     int(connectivity) == 4,
		"state":        nmStateLabel(state),
		"connectivity": nmConnectivityLabel(connectivity),
	}

	// Get active connections (best-effort).
	out, err := run(ctx, "busctl", "--json=short", "call",
		nmBus, nmPath, nmDBus,
		"Get",
		"ss",
		nmIface,
		"ActiveConnections",
	)
	if err == nil {
		// Response: {"type":"v","data":[{"type":"ao","data":["/path1","/path2"]}]}
		var resp struct {
			Data []struct {
				Data []string `json:"data"`
			} `json:"data"`
		}
		if json.Unmarshal(out, &resp) == nil && len(resp.Data) > 0 {
			var connections []map[string]any
			for _, connPath := range resp.Data[0].Data {
				connPath = strings.TrimSpace(connPath)
				if connPath == "" || connPath == "/" {
					continue
				}
				connID := nmActiveConnProp(ctx, run, connPath, "Id")
				connType := nmActiveConnProp(ctx, run, connPath, "Type")
				conn := map[string]any{"path": connPath}
				if connID != "" {
					conn["id"] = connID
				}
				if connType != "" {
					conn["type"] = connType
				}
				connections = append(connections, conn)
			}
			if len(connections) > 0 {
				result["active_connections"] = connections
			}
		}
	}

	return JSONResult(result)
}

// nmActiveConnProp reads a string property from an active connection object.
func nmActiveConnProp(ctx context.Context, run commandRunner, path, prop string) string {
	out, err := run(ctx, "busctl", "--json=short", "get-property",
		nmBus, path, nmActiveConnIface, prop,
	)
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
