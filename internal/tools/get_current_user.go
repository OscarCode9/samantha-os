package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	accountsBus       = "org.freedesktop.Accounts"
	accountsPath      = "/org/freedesktop/Accounts"
	accountsIface     = "org.freedesktop.Accounts"
	accountsUserIface = "org.freedesktop.Accounts.User"
)

type getCurrentUserTool struct {
	run commandRunner
}

func NewGetCurrentUserTool() Tool {
	return &getCurrentUserTool{run: defaultCommandRunner}
}

func (t *getCurrentUserTool) Name() string { return "get_current_user" }

func (t *getCurrentUserTool) Description() string {
	return "Get information about the currently logged-in user: username, display name, home directory, language, and avatar path."
}

func (t *getCurrentUserTool) Parameters() Schema {
	return Schema{Type: "object", Properties: map[string]SchemaProperty{}}
}

func (t *getCurrentUserTool) Execute(ctx context.Context, _ string) Result {
	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	// Determine current username.
	username := strings.TrimSpace(os.Getenv("USER"))
	if username == "" {
		username = strings.TrimSpace(os.Getenv("LOGNAME"))
	}
	if username == "" {
		out, err := run(ctx, "id", "-un")
		if err != nil {
			return ErrorResult(fmt.Sprintf("could not determine current user: %v", err))
		}
		username = strings.TrimSpace(string(out))
	}

	// Find the user's DBus object path.
	out, err := run(ctx, "busctl", "--json=short", "call",
		accountsBus, accountsPath, accountsIface,
		"FindUserByName",
		"s",
		username,
	)
	if err != nil {
		// Fallback: return basic info from env.
		return JSONResult(map[string]any{
			"username": username,
			"home":     os.Getenv("HOME"),
			"language": os.Getenv("LANG"),
		})
	}

	// busctl call returns: {"type":"o","data":["/path/..."]}
	var pathResp struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(out, &pathResp); err != nil || len(pathResp.Data) == 0 || pathResp.Data[0] == "" {
		return ErrorResult("could not parse user object path")
	}
	userPath := pathResp.Data[0]

	// readUserProp reads a string property from the user object.
	readUserProp := func(prop string) string {
		data, err := run(ctx, "busctl", "--json=short", "get-property",
			accountsBus, userPath, accountsUserIface, prop,
		)
		if err != nil {
			return ""
		}
		var result struct {
			Data string `json:"data"`
		}
		if json.Unmarshal(data, &result) != nil {
			return ""
		}
		return result.Data
	}

	realName := readUserProp("RealName")
	if realName == "" || realName == username {
		realName = readUserProp("UserName")
	}

	result := map[string]any{
		"username":     username,
		"real_name":    realName,
		"home":         readUserProp("HomeDirectory"),
		"language":     readUserProp("Language"),
		"icon_file":    readUserProp("IconFile"),
		"email":        readUserProp("Email"),
		"session_type": readUserProp("SessionType"),
	}

	return JSONResult(result)
}
