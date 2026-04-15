package tools

import (
	"path/filepath"
	"strings"
)

// resolvePath resolves a potentially relative path against a workspace root.
// If the path is absolute, it is returned as-is. If it's relative, it is
// joined with the workspace root.
func resolvePath(path string, workspaceRoot string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return workspaceRoot
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(workspaceRoot, path))
}
