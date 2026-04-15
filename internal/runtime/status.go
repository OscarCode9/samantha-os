package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/session"
)

type Status struct {
	ConfigPresent    bool
	AuthPresent      bool
	BootstrapPresent bool
	SessionCount     int
	ProviderPending  bool
	WorkspacePath    string
}

func Inspect(paths config.Paths, store *session.Store) (Status, error) {
	status := Status{
		ConfigPresent:    fileExists(paths.ConfigPath),
		AuthPresent:      fileExists(paths.AuthPath),
		BootstrapPresent: fileExists(paths.BootstrapPath),
		WorkspacePath:    paths.WorkspaceDir,
	}

	if status.ConfigPresent {
		data, err := os.ReadFile(paths.ConfigPath)
		if err != nil {
			return Status{}, fmt.Errorf("read config: %w", err)
		}
		var payload struct {
			Setup struct {
				ProviderPending bool `json:"providerPending"`
			} `json:"setup"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return Status{}, fmt.Errorf("decode config: %w", err)
		}
		status.ProviderPending = payload.Setup.ProviderPending
	}

	sessions, err := store.List()
	if err != nil {
		return Status{}, err
	}
	status.SessionCount = len(sessions)

	return status, nil
}

func (status Status) String() string {
	lines := []string{
		fmt.Sprintf("config_present=%t", status.ConfigPresent),
		fmt.Sprintf("auth_present=%t", status.AuthPresent),
		fmt.Sprintf("bootstrap_present=%t", status.BootstrapPresent),
		fmt.Sprintf("provider_pending=%t", status.ProviderPending),
		fmt.Sprintf("session_count=%d", status.SessionCount),
		fmt.Sprintf("workspace=%s", status.WorkspacePath),
	}
	return strings.Join(lines, "\n")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
