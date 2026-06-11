package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type cleanCacheTool struct {
	run commandRunner
}

const (
	cleanCacheModeStandard   = "standard"
	cleanCacheModeSuperClean = "super_clean"
)

type cacheRootReport struct {
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	HumanSize string `json:"human_size"`
	ItemCount int    `json:"item_count"`
}

type cacheCandidate struct {
	Path      string `json:"path"`
	Root      string `json:"root"`
	Kind      string `json:"kind"`
	Bytes     int64  `json:"bytes"`
	HumanSize string `json:"human_size"`
}

type skippedCacheLocation struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func NewCleanCacheTool() Tool {
	return &cleanCacheTool{run: defaultCommandRunner}
}

func (t *cleanCacheTool) Name() string { return "clean_cache" }

func (t *cleanCacheTool) Description() string {
	return "Analyze user-accessible cache directories and, only after explicit confirmation, remove selected cache contents. Supports mode='standard' and a broader mode='super_clean'. Default action is analyze; deletion requires confirm=true and explicit paths from the analysis report."
}

func (t *cleanCacheTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"mode": {
				Type:        "string",
				Description: "Cleanup scope. 'standard' scans normal user cache roots. 'super_clean' also includes heavier user-owned cleanup targets like Trash contents and developer package caches.",
				Enum:        []string{cleanCacheModeStandard, cleanCacheModeSuperClean},
				Default:     cleanCacheModeStandard,
			},
			"action": {
				Type:        "string",
				Description: "Action to perform. Use 'analyze' first to inspect cache usage, then 'delete' only after the user confirms.",
				Enum:        []string{"analyze", "delete"},
				Default:     "analyze",
			},
			"paths": {
				Type:        "array",
				Description: "For action='delete', the cache paths to clear. These should come from the prior analysis report.",
				Items:       &SchemaProperty{Type: "string"},
			},
			"confirm": {
				Type:        "boolean",
				Description: "Must be true before any deletion happens.",
				Default:     false,
			},
			"limit": {
				Type:        "integer",
				Description: "For action='analyze', maximum number of largest cache entries to return. Defaults to 50.",
				Default:     50,
			},
		},
	}

}

func (t *cleanCacheTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Mode    string   `json:"mode"`
		Action  string   `json:"action"`
		Paths   []string `json:"paths"`
		Confirm bool     `json:"confirm"`
		Limit   int      `json:"limit"`
	}{
		Mode:   cleanCacheModeStandard,
		Action: "analyze",
		Limit:  50,
	}
	if strings.TrimSpace(arguments) != "" {
		if err := ParseArgs(arguments, &params); err != nil {
			return ErrorResult(err.Error())
		}
	}

	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 200 {
		params.Limit = 200
	}

	action := strings.ToLower(strings.TrimSpace(params.Action))
	if action == "" {
		action = "analyze"
	}
	if action != "analyze" && action != "delete" {
		return ErrorResult("action must be 'analyze' or 'delete'")
	}
	mode := normalizeCleanCacheMode(params.Mode)
	if mode == "" {
		return ErrorResult("mode must be 'standard' or 'super_clean'")
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}
	home := resolveHomeDir(ctx, run)
	if strings.TrimSpace(home) == "" {
		return ErrorResult("could not resolve the current user's home directory")
	}

	roots := discoverCacheRoots(home, mode)
	if action == "delete" {
		return t.deleteCachePaths(ctx, run, roots, params.Paths, params.Confirm)
	}
	return analyzeCachePaths(roots, params.Limit, mode)
}

func analyzeCachePaths(roots []string, limit int, mode string) Result {
	rootReports := make([]cacheRootReport, 0, len(roots))
	candidates := make([]cacheCandidate, 0)
	warnings := make([]string, 0)
	suggestedDeletePaths := make([]string, 0, len(roots))
	var totalBytes int64
	var totalCandidateCount int

	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", root, err))
			continue
		}

		var rootBytes int64
		for _, entry := range entries {
			entryPath := filepath.Join(root, entry.Name())
			entryBytes, err := cacheEntrySize(entryPath)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", entryPath, err))
				continue
			}
			rootBytes += entryBytes
			totalCandidateCount++
			candidates = append(candidates, cacheCandidate{
				Path:      entryPath,
				Root:      root,
				Kind:      cacheEntryKind(entryPath),
				Bytes:     entryBytes,
				HumanSize: humanBytes(entryBytes),
			})
		}

		if len(entries) == 0 && rootBytes == 0 {
			continue
		}
		rootReports = append(rootReports, cacheRootReport{
			Path:      root,
			Bytes:     rootBytes,
			HumanSize: humanBytes(rootBytes),
			ItemCount: len(entries),
		})
		totalBytes += rootBytes
		suggestedDeletePaths = append(suggestedDeletePaths, root)
	}

	sort.Slice(rootReports, func(i, j int) bool {
		if rootReports[i].Bytes == rootReports[j].Bytes {
			return rootReports[i].Path < rootReports[j].Path
		}
		return rootReports[i].Bytes > rootReports[j].Bytes
	})
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Bytes == candidates[j].Bytes {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].Bytes > candidates[j].Bytes
	})

	truncated := false
	if limit < len(candidates) {
		candidates = candidates[:limit]
		truncated = true
	}

	return JSONResult(map[string]any{
		"ok":                     true,
		"mode":                   mode,
		"action":                 "analyze",
		"scope":                  cleanCacheScope(mode),
		"cache_roots":            rootReports,
		"candidates":             candidates,
		"candidate_count":        totalCandidateCount,
		"displayed_candidates":   len(candidates),
		"truncated":              truncated,
		"total_bytes":            totalBytes,
		"total_human_size":       humanBytes(totalBytes),
		"suggested_delete_paths": suggestedDeletePaths,
		"excluded_locations": []skippedCacheLocation{
			{Path: "/var/cache", Reason: "system-owned cache is not touched by this tool"},
			{Path: "/tmp", Reason: "temporary files are excluded for safety"},
			{Path: "/var/tmp", Reason: "temporary files are excluded for safety"},
		},
		"warnings": warnings,
		"note":     cleanCacheNote(mode),
	})
}

func (t *cleanCacheTool) deleteCachePaths(ctx context.Context, run commandRunner, roots []string, rawPaths []string, confirm bool) Result {
	if !confirm {
		return ErrorResult("delete requires confirm=true after the user reviews the analysis")
	}
	if len(rawPaths) == 0 {
		return ErrorResult("delete requires at least one path from the analysis report")
	}

	paths := make([]string, 0, len(rawPaths))
	invalid := make([]string, 0)
	seen := map[string]struct{}{}
	for _, rawPath := range rawPaths {
		path := resolveUserPath(ctx, run, rawPath)
		if path == "" {
			continue
		}
		if !isWithinAnyCacheRoot(path, roots) {
			invalid = append(invalid, path)
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	if len(invalid) > 0 {
		sort.Strings(invalid)
		return ErrorResult("refusing to delete paths outside the allowed cache roots: " + strings.Join(invalid, ", "))
	}
	if len(paths) == 0 {
		return ErrorResult("no valid cache paths to delete")
	}

	deletedPaths := make([]string, 0)
	clearedRoots := make([]string, 0)
	missingPaths := make([]string, 0)
	errors := make([]string, 0)
	var estimatedBytesFreed int64

	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				missingPaths = append(missingPaths, path)
				continue
			}
			errors = append(errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}

		if isExactCacheRoot(path, roots) && info.IsDir() {
			freed, removed, clearErrs := clearDirectoryContents(path)
			estimatedBytesFreed += freed
			clearedRoots = append(clearedRoots, path)
			deletedPaths = append(deletedPaths, removed...)
			errors = append(errors, clearErrs...)
			continue
		}

		size, sizeErr := cacheEntrySize(path)
		if sizeErr == nil {
			estimatedBytesFreed += size
		}
		if err := os.RemoveAll(path); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		deletedPaths = append(deletedPaths, path)
	}

	sort.Strings(deletedPaths)
	sort.Strings(clearedRoots)
	sort.Strings(missingPaths)
	sort.Strings(errors)

	return JSONResult(map[string]any{
		"ok":                   len(errors) == 0,
		"mode":                 detectDeleteMode(paths, roots),
		"action":               "delete",
		"confirmed":            true,
		"deleted_paths":        deletedPaths,
		"cleared_roots":        clearedRoots,
		"missing_paths":        missingPaths,
		"errors":               errors,
		"estimated_bytes_freed": estimatedBytesFreed,
		"estimated_human_freed": humanBytes(estimatedBytesFreed),
		"note":                 "Only the selected user-accessible cleanup paths were touched.",
	})
}

func discoverCacheRoots(home string, mode string) []string {
	seen := map[string]struct{}{}
	roots := make([]string, 0)
	addRoot := func(path string) {
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		info, err := os.Stat(clean)
		if err != nil || !info.IsDir() {
			return
		}
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		roots = append(roots, clean)
	}

	addRoot(filepath.Join(home, ".cache"))
	flatpakRoots, _ := filepath.Glob(filepath.Join(home, ".var", "app", "*", "cache"))
	for _, root := range flatpakRoots {
		addRoot(root)
	}
	if mode == cleanCacheModeSuperClean {
		for _, root := range superCleanRoots(home) {
			addRoot(root)
		}
	}

	sort.Strings(roots)
	return roots
}

func superCleanRoots(home string) []string {
	return []string{
		filepath.Join(home, ".local", "share", "Trash", "files"),
		filepath.Join(home, ".npm", "_cacache"),
		filepath.Join(home, ".cargo", "registry", "cache"),
	}
}

func normalizeCleanCacheMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", cleanCacheModeStandard:
		return cleanCacheModeStandard
	case cleanCacheModeSuperClean:
		return cleanCacheModeSuperClean
	default:
		return ""
	}
}

func cleanCacheScope(mode string) string {
	if mode == cleanCacheModeSuperClean {
		return "standard cache roots plus selected user-owned heavy cleanup targets"
	}
	return "user-accessible cache directories only"
}

func cleanCacheNote(mode string) string {
	if mode == cleanCacheModeSuperClean {
		return "Run action='delete' only after the user confirms. In super_clean mode, deleting a cleanup root clears its contents but preserves the root directory, including Trash contents and developer package caches if they were analyzed."
	}
	return "Run action='delete' only after the user confirms. Deleting a cache root clears its contents but preserves the root directory."
}

func detectDeleteMode(paths []string, roots []string) string {
	for _, path := range paths {
		if filepath.Base(filepath.Clean(path)) == "files" && strings.Contains(filepath.Clean(path), filepath.Join(".local", "share", "Trash", "files")) {
			return cleanCacheModeSuperClean
		}
	}
	for _, root := range roots {
		clean := filepath.Clean(root)
		if strings.Contains(clean, filepath.Join(".npm", "_cacache")) || strings.Contains(clean, filepath.Join(".cargo", "registry", "cache")) {
			return cleanCacheModeSuperClean
		}
	}
	return cleanCacheModeStandard
}

func cacheEntrySize(path string) (int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return info.Size(), nil
	}

	var total int64
	err = filepath.WalkDir(path, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		currentInfo, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if currentInfo.Mode().IsRegular() || currentInfo.Mode()&os.ModeSymlink != 0 {
			total += currentInfo.Size()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

func clearDirectoryContents(root string) (int64, []string, []string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, nil, []string{fmt.Sprintf("%s: %v", root, err)}
	}
	removed := make([]string, 0, len(entries))
	errors := make([]string, 0)
	var freed int64
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		size, sizeErr := cacheEntrySize(path)
		if sizeErr == nil {
			freed += size
		}
		if err := os.RemoveAll(path); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		removed = append(removed, path)
	}
	return freed, removed, errors
}

func cacheEntryKind(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return "unknown"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "symlink"
	}
	if info.IsDir() {
		return "directory"
	}
	return "file"
}

func isWithinAnyCacheRoot(path string, roots []string) bool {
	for _, root := range roots {
		if isSameOrDescendant(path, root) {
			return true
		}
	}
	return false
}

func isExactCacheRoot(path string, roots []string) bool {
	for _, root := range roots {
		if filepath.Clean(path) == filepath.Clean(root) {
			return true
		}
	}
	return false
}

func isSameOrDescendant(path string, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	if cleanPath == cleanRoot {
		return true
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func humanBytes(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(size)
	unit := "B"
	for _, next := range units {
		value /= 1024.0
		unit = next
		if value < 1024.0 {
			break
		}
	}
	if value >= 10 {
		return fmt.Sprintf("%.0f %s", value, unit)
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}