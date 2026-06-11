package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	DefaultAnthropicSkillsSource = "https://github.com/anthropics/skills/tree/main/skills"
	defaultSkillSetVersion       = 1
	defaultSkillMarkerName       = ".anthropic-default-skills.json"
)

var (
	githubAPIBaseURL = "https://api.github.com"
	githubHTTPClient = http.DefaultClient
	useGitForGitHubInstall = true
	useArchiveForGitHubInstall = true

	restrictedAnthropicSkillNames = map[string]struct{}{
		"docx": {},
		"pdf":  {},
		"pptx": {},
		"xlsx": {},
	}
)

type InstallSourceOptions struct {
	SkipExisting bool
}

type InstallSkip struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type InstallSummary struct {
	Installed []*Skill      `json:"-"`
	Skipped   []InstallSkip `json:"skipped,omitempty"`
}

type defaultSkillMarker struct {
	Version     int           `json:"version"`
	Source      string        `json:"source"`
	InstalledAt string        `json:"installedAt"`
	Installed   []string      `json:"installed"`
	Skipped     []InstallSkip `json:"skipped,omitempty"`
}

type gitHubSource struct {
	Owner string
	Repo  string
	Ref   string
	Path  string
}

type gitHubContentItem struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	URL         string `json:"url"`
	DownloadURL string `json:"download_url"`
	Content     string `json:"content,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
}

func (s *InstallSummary) addInstalled(skill *Skill) {
	if skill == nil {
		return
	}
	s.Installed = append(s.Installed, skill)
}

func (s *InstallSummary) addSkipped(name, reason string) {
	if strings.TrimSpace(name) == "" {
		name = "unknown"
	}
	s.Skipped = append(s.Skipped, InstallSkip{Name: name, Reason: reason})
}

func (s *InstallSummary) InstalledNames() []string {
	if s == nil || len(s.Installed) == 0 {
		return nil
	}
	names := make([]string, 0, len(s.Installed))
	for _, skill := range s.Installed {
		names = append(names, skill.Name)
	}
	sort.Strings(names)
	return names
}

// InstallSource installs one skill or a whole skill collection from a local
// path, a raw SKILL.md URL, or a GitHub repository/tree URL.
func (r *Registry) InstallSource(source string, managedDir string, opts InstallSourceOptions) (*InstallSummary, error) {
	if spec, ok := parseGitHubSource(source); ok {
		return r.installFromGitHub(spec, managedDir, opts)
	}

	if opts.SkipExisting {
		if name, ok := installTargetName(source); ok && skillDirExists(managedDir, name) {
			return &InstallSummary{Skipped: []InstallSkip{{Name: name, Reason: "already installed"}}}, nil
		}
	}

	installed, err := r.Install(source, managedDir)
	if err != nil {
		return nil, err
	}

	summary := &InstallSummary{}
	summary.addInstalled(installed)
	return summary, nil
}

// EnsureDefaultSkills seeds the default Anthropic example skills once into the
// managed skills directory. Skills with restrictive upstream license terms are
// skipped automatically.
func EnsureDefaultSkills(managedDir string, force bool) (*InstallSummary, error) {
	if !force {
		marker, err := readDefaultSkillMarker(managedDir)
		if err == nil && marker.Version == defaultSkillSetVersion && marker.Source == DefaultAnthropicSkillsSource {
			return &InstallSummary{}, nil
		}
	}

	registry := NewRegistry()
	summary, err := registry.InstallSource(DefaultAnthropicSkillsSource, managedDir, InstallSourceOptions{SkipExisting: true})
	if err != nil {
		return nil, err
	}
	if err := writeDefaultSkillMarker(managedDir, summary); err != nil {
		return nil, err
	}
	return summary, nil
}

func (r *Registry) installFromGitHub(spec gitHubSource, managedDir string, opts InstallSourceOptions) (*InstallSummary, error) {
	if useGitForGitHubInstall {
		if _, err := findExecutable("git"); err == nil {
		summary, err := r.installFromGitHubWithGit(spec, managedDir, opts)
		if err == nil {
			return summary, nil
		}
		}
	}
	if useArchiveForGitHubInstall {
		summary, err := r.installFromGitHubArchive(spec, managedDir, opts)
		if err == nil {
			return summary, nil
		}
	}

	items, err := githubListContents(spec, spec.Path)
	if err != nil {
		return nil, err
	}

	summary := &InstallSummary{}
	if containsSkillMarkdown(items) {
		skillName := pathpkg.Base(spec.Path)
		if opts.SkipExisting && skillDirExists(managedDir, skillName) {
			summary.addSkipped(skillName, "already installed")
			return summary, nil
		}

		allowed, reason, err := allowGitHubSkill(spec, spec.Path, items)
		if err != nil {
			return nil, err
		}
		if !allowed {
			summary.addSkipped(skillName, reason)
			return summary, nil
		}

		skill, err := r.installGitHubSkillDir(spec, spec.Path, items, managedDir)
		if err != nil {
			return nil, err
		}
		summary.addInstalled(skill)
		return summary, nil
	}

	for _, item := range items {
		if item.Type != "dir" {
			continue
		}

		childItems, err := githubListContents(spec, item.Path)
		if err != nil {
			return nil, err
		}
		if !containsSkillMarkdown(childItems) {
			continue
		}

		if opts.SkipExisting && skillDirExists(managedDir, item.Name) {
			summary.addSkipped(item.Name, "already installed")
			continue
		}

		allowed, reason, err := allowGitHubSkill(spec, item.Path, childItems)
		if err != nil {
			return nil, err
		}
		if !allowed {
			summary.addSkipped(item.Name, reason)
			continue
		}

		skill, err := r.installGitHubSkillDir(spec, item.Path, childItems, managedDir)
		if err != nil {
			return nil, err
		}
		summary.addInstalled(skill)
	}

	return summary, nil
}

func (r *Registry) installFromGitHubArchive(spec gitHubSource, managedDir string, opts InstallSourceOptions) (*InstallSummary, error) {
	repoDir, cleanup, err := extractGitHubArchive(spec)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	summary := &InstallSummary{}
	targetDir := filepath.Join(repoDir, filepath.FromSlash(spec.Path))
	if fileExists(filepath.Join(targetDir, "SKILL.md")) {
		skillName := filepath.Base(targetDir)
		if opts.SkipExisting && skillDirExists(managedDir, skillName) {
			summary.addSkipped(skillName, "already installed")
			return summary, nil
		}
		allowed, reason, err := allowCheckedOutSkill(spec, targetDir)
		if err != nil {
			return nil, err
		}
		if !allowed {
			summary.addSkipped(skillName, reason)
			return summary, nil
		}

		installed, err := r.Install(targetDir, managedDir)
		if err != nil {
			return nil, err
		}
		summary.addInstalled(installed)
		return summary, nil
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return nil, fmt.Errorf("read extracted skill collection: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		childDir := filepath.Join(targetDir, entry.Name())
		if !fileExists(filepath.Join(childDir, "SKILL.md")) {
			continue
		}
		if opts.SkipExisting && skillDirExists(managedDir, entry.Name()) {
			summary.addSkipped(entry.Name(), "already installed")
			continue
		}
		allowed, reason, err := allowCheckedOutSkill(spec, childDir)
		if err != nil {
			return nil, err
		}
		if !allowed {
			summary.addSkipped(entry.Name(), reason)
			continue
		}

		installed, err := r.Install(childDir, managedDir)
		if err != nil {
			return nil, err
		}
		summary.addInstalled(installed)
	}

	return summary, nil
}

func (r *Registry) installFromGitHubWithGit(spec gitHubSource, managedDir string, opts InstallSourceOptions) (*InstallSummary, error) {
	patterns := []string{
		pathpkg.Join(spec.Path, "SKILL.md"),
		pathpkg.Join(spec.Path, "LICENSE.txt"),
		pathpkg.Join(spec.Path, "license.txt"),
		pathpkg.Join(spec.Path, "LICENSE.md"),
		pathpkg.Join(spec.Path, "license.md"),
		pathpkg.Join(spec.Path, "*", "SKILL.md"),
		pathpkg.Join(spec.Path, "*", "LICENSE.txt"),
		pathpkg.Join(spec.Path, "*", "license.txt"),
		pathpkg.Join(spec.Path, "*", "LICENSE.md"),
		pathpkg.Join(spec.Path, "*", "license.md"),
	}

	repoDir, cleanup, err := sparseCloneGitHubRepo(spec, patterns)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	summary := &InstallSummary{}
	targetDir := filepath.Join(repoDir, filepath.FromSlash(spec.Path))
	if fileExists(filepath.Join(targetDir, "SKILL.md")) {
		skillName := filepath.Base(targetDir)
		if opts.SkipExisting && skillDirExists(managedDir, skillName) {
			summary.addSkipped(skillName, "already installed")
			return summary, nil
		}

		allowed, reason, err := allowCheckedOutSkill(spec, targetDir)
		if err != nil {
			return nil, err
		}
		if !allowed {
			summary.addSkipped(skillName, reason)
			return summary, nil
		}

		if err := addSparseCheckoutPath(repoDir, spec.Path); err != nil {
			return nil, err
		}
		installed, err := r.Install(targetDir, managedDir)
		if err != nil {
			return nil, err
		}
		summary.addInstalled(installed)
		return summary, nil
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return nil, fmt.Errorf("read checked out skill collection: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		childDir := filepath.Join(targetDir, entry.Name())
		if !fileExists(filepath.Join(childDir, "SKILL.md")) {
			continue
		}
		if opts.SkipExisting && skillDirExists(managedDir, entry.Name()) {
			summary.addSkipped(entry.Name(), "already installed")
			continue
		}

		allowed, reason, err := allowCheckedOutSkill(spec, childDir)
		if err != nil {
			return nil, err
		}
		if !allowed {
			summary.addSkipped(entry.Name(), reason)
			continue
		}

		remoteSkillPath := pathpkg.Join(spec.Path, entry.Name())
		if err := addSparseCheckoutPath(repoDir, remoteSkillPath); err != nil {
			return nil, err
		}

		installed, err := r.Install(childDir, managedDir)
		if err != nil {
			return nil, err
		}
		summary.addInstalled(installed)
	}

	return summary, nil
}

func (r *Registry) installGitHubSkillDir(spec gitHubSource, remotePath string, items []gitHubContentItem, managedDir string) (*Skill, error) {
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		return nil, fmt.Errorf("create managed skills directory: %w", err)
	}

	skillDirName := pathpkg.Base(remotePath)
	tempRoot, err := os.MkdirTemp(managedDir, ".skill-install-*")
	if err != nil {
		return nil, fmt.Errorf("create temp skill directory: %w", err)
	}
	defer os.RemoveAll(tempRoot)

	tempSkillDir := filepath.Join(tempRoot, skillDirName)
	if err := downloadGitHubDirectory(spec, remotePath, tempSkillDir, items); err != nil {
		return nil, err
	}

	skillFile := filepath.Join(tempSkillDir, "SKILL.md")
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return nil, fmt.Errorf("read downloaded SKILL.md: %w", err)
	}

	destDir := filepath.Join(managedDir, skillDirName)
	if err := os.RemoveAll(destDir); err != nil {
		return nil, fmt.Errorf("remove existing skill directory: %w", err)
	}
	if err := os.Rename(tempSkillDir, destDir); err != nil {
		if err := copyDir(tempSkillDir, destDir); err != nil {
			return nil, fmt.Errorf("move downloaded skill into place: %w", err)
		}
	}

	skill := parseSkillMarkdown(skillDirName, destDir, string(data))
	skill.Source = SourceManaged
	skill.Enabled = true

	r.mu.Lock()
	r.skills[skill.Name] = skill
	r.mu.Unlock()

	return skill, nil
}

func downloadGitHubDirectory(spec gitHubSource, remotePath, localDir string, items []gitHubContentItem) error {
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return fmt.Errorf("create local skill directory: %w", err)
	}

	if items == nil {
		var err error
		items, err = githubListContents(spec, remotePath)
		if err != nil {
			return err
		}
	}

	for _, item := range items {
		destPath := filepath.Join(localDir, item.Name)
		switch item.Type {
		case "dir":
			childItems, err := githubListContents(spec, item.Path)
			if err != nil {
				return err
			}
			if err := downloadGitHubDirectory(spec, item.Path, destPath, childItems); err != nil {
				return err
			}
		case "file":
			data, err := githubDownloadFile(item)
			if err != nil {
				return err
			}
			if err := os.WriteFile(destPath, data, defaultGitHubFileMode(item.Path)); err != nil {
				return fmt.Errorf("write %s: %w", item.Path, err)
			}
		}
	}

	return nil
}

func parseGitHubSource(raw string) (gitHubSource, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Host, "github.com") {
		return gitHubSource{}, false
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 {
		return gitHubSource{}, false
	}

	spec := gitHubSource{
		Owner: parts[0],
		Repo:  strings.TrimSuffix(parts[1], ".git"),
		Ref:   "main",
		Path:  "skills",
	}

	if len(parts) >= 4 && parts[2] == "tree" {
		spec.Ref = parts[3]
		if len(parts) > 4 {
			spec.Path = strings.Join(parts[4:], "/")
		}
	}

	if spec.Path == "" {
		spec.Path = "skills"
	}

	return spec, true
}

func githubListContents(spec gitHubSource, remotePath string) ([]gitHubContentItem, error) {
	requestURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", githubAPIBaseURL, spec.Owner, spec.Repo, remotePath, url.QueryEscape(spec.Ref))
	body, err := githubGET(requestURL)
	if err != nil {
		return nil, err
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, fmt.Errorf("empty GitHub API response for %s", remotePath)
	}

	if strings.HasPrefix(trimmed, "[") {
		var items []gitHubContentItem
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, fmt.Errorf("decode GitHub contents list: %w", err)
		}
		return items, nil
	}

	var item gitHubContentItem
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, fmt.Errorf("decode GitHub contents item: %w", err)
	}
	return []gitHubContentItem{item}, nil
}

func githubDownloadFile(item gitHubContentItem) ([]byte, error) {
	if item.DownloadURL != "" {
		return githubGET(item.DownloadURL)
	}
	if item.URL == "" {
		return nil, fmt.Errorf("no download URL for %s", item.Path)
	}

	body, err := githubGET(item.URL)
	if err != nil {
		return nil, err
	}

	var detail gitHubContentItem
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("decode GitHub file payload: %w", err)
	}
	if detail.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(detail.Content, "\n", ""))
		if err != nil {
			return nil, fmt.Errorf("decode GitHub file content: %w", err)
		}
		return decoded, nil
	}
	if detail.DownloadURL != "" {
		return githubGET(detail.DownloadURL)
	}
	return nil, fmt.Errorf("unsupported GitHub file encoding for %s", item.Path)
}

func githubGET(requestURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "elementary-claw")

	resp, err := githubHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", requestURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("request %s: HTTP %d: %s", requestURL, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", requestURL, err)
	}
	return body, nil
}

func containsSkillMarkdown(items []gitHubContentItem) bool {
	for _, item := range items {
		if item.Type == "file" && item.Name == "SKILL.md" {
			return true
		}
	}
	return false
}

func allowGitHubSkill(spec gitHubSource, remotePath string, items []gitHubContentItem) (bool, string, error) {
	skillName := pathpkg.Base(remotePath)
	if spec.Owner == "anthropics" && spec.Repo == "skills" {
		if _, restricted := restrictedAnthropicSkillNames[skillName]; restricted {
			return false, "skipped due to restrictive upstream license", nil
		}
	}

	for _, item := range items {
		if item.Type != "file" {
			continue
		}
		lower := strings.ToLower(item.Name)
		if lower != "license.txt" && lower != "license" && lower != "license.md" {
			continue
		}
		data, err := githubDownloadFile(item)
		if err != nil {
			return false, "", err
		}
		if hasRestrictedLicenseTerms(string(data)) {
			return false, "skipped due to restrictive upstream license", nil
		}
	}

	return true, "", nil
}

func hasRestrictedLicenseTerms(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "all rights reserved") ||
		strings.Contains(lower, "additional restrictions") ||
		strings.Contains(lower, "source-available") ||
		strings.Contains(lower, "retain copies outside the services")
}

func defaultGitHubFileMode(remotePath string) os.FileMode {
	clean := strings.ToLower(remotePath)
	if strings.Contains(clean, "/scripts/") || strings.HasSuffix(clean, ".sh") {
		return 0o755
	}
	return 0o644
}

func sparseCloneGitHubRepo(spec gitHubSource, patterns []string) (string, func(), error) {
	tempRoot, err := os.MkdirTemp("", "openclaw-skill-clone-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp clone dir: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tempRoot)
	}

	repoDir := filepath.Join(tempRoot, "repo")
	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", spec.Owner, spec.Repo)
	if err := runGitCommand(tempRoot, "clone", "--depth", "1", "--filter=blob:none", "--sparse", "--branch", spec.Ref, repoURL, repoDir); err != nil {
		cleanup()
		return "", nil, err
	}
	args := append([]string{"-C", repoDir, "sparse-checkout", "set", "--no-cone"}, patterns...)
	if err := runGitCommand(tempRoot, args...); err != nil {
		cleanup()
		return "", nil, err
	}

	return repoDir, cleanup, nil
}

func extractGitHubArchive(spec gitHubSource) (string, func(), error) {
	tempRoot, err := os.MkdirTemp("", "openclaw-skill-archive-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp archive dir: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tempRoot)
	}

	archiveURL := fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/%s.tar.gz", spec.Owner, spec.Repo, spec.Ref)
	data, err := githubGET(archiveURL)
	if err != nil {
		cleanup()
		return "", nil, err
	}

	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("open GitHub archive: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	rootDir := ""
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			cleanup()
			return "", nil, fmt.Errorf("read GitHub archive: %w", err)
		}

		cleanName := filepath.Clean(header.Name)
		if cleanName == "." {
			continue
		}
		parts := strings.Split(cleanName, string(filepath.Separator))
		if len(parts) > 0 && rootDir == "" {
			rootDir = parts[0]
		}

		destPath := filepath.Join(tempRoot, cleanName)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				cleanup()
				return "", nil, fmt.Errorf("create archive directory: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				cleanup()
				return "", nil, fmt.Errorf("create archive parent directory: %w", err)
			}
			file, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				cleanup()
				return "", nil, fmt.Errorf("create archive file: %w", err)
			}
			if _, err := io.Copy(file, tr); err != nil {
				file.Close()
				cleanup()
				return "", nil, fmt.Errorf("extract archive file: %w", err)
			}
			_ = file.Close()
		}
	}

	if rootDir == "" {
		cleanup()
		return "", nil, fmt.Errorf("could not determine extracted archive root")
	}

	return filepath.Join(tempRoot, rootDir), cleanup, nil
}

func addSparseCheckoutPath(repoDir, remotePath string) error {
	return runGitCommand(repoDir, "-C", repoDir, "sparse-checkout", "add", "--no-cone", remotePath)
}

func runGitCommand(workDir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func allowCheckedOutSkill(spec gitHubSource, skillDir string) (bool, string, error) {
	skillName := filepath.Base(skillDir)
	if spec.Owner == "anthropics" && spec.Repo == "skills" {
		if _, restricted := restrictedAnthropicSkillNames[skillName]; restricted {
			return false, "skipped due to restrictive upstream license", nil
		}
	}

	for _, candidate := range []string{"LICENSE.txt", "license.txt", "LICENSE.md", "license.md"} {
		licensePath := filepath.Join(skillDir, candidate)
		if !fileExists(licensePath) {
			continue
		}
		data, err := os.ReadFile(licensePath)
		if err != nil {
			return false, "", fmt.Errorf("read checked out license file: %w", err)
		}
		if hasRestrictedLicenseTerms(string(data)) {
			return false, "skipped due to restrictive upstream license", nil
		}
	}

	return true, "", nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func installTargetName(source string) (string, bool) {
	if spec, ok := parseGitHubSource(source); ok {
		return pathpkg.Base(spec.Path), true
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		parsed, err := url.Parse(source)
		if err != nil {
			return "", false
		}
		base := pathpkg.Base(strings.TrimSuffix(parsed.Path, "/"))
		if strings.EqualFold(base, "SKILL.md") {
			base = pathpkg.Base(pathpkg.Dir(parsed.Path))
		}
		if base == "" || base == "." || base == "/" {
			return "", false
		}
		return base, true
	}

	trimmed := strings.TrimRight(source, string(os.PathSeparator))
	if trimmed == "" {
		return "", false
	}
	return filepath.Base(trimmed), true
}

func skillDirExists(managedDir, name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(managedDir, name, "SKILL.md"))
	return err == nil && !info.IsDir()
}

func readDefaultSkillMarker(managedDir string) (defaultSkillMarker, error) {
	data, err := os.ReadFile(filepath.Join(managedDir, defaultSkillMarkerName))
	if err != nil {
		return defaultSkillMarker{}, err
	}
	var marker defaultSkillMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return defaultSkillMarker{}, err
	}
	return marker, nil
}

func writeDefaultSkillMarker(managedDir string, summary *InstallSummary) error {
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		return fmt.Errorf("create managed skills directory: %w", err)
	}

	marker := defaultSkillMarker{
		Version:     defaultSkillSetVersion,
		Source:      DefaultAnthropicSkillsSource,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
		Installed:   summary.InstalledNames(),
		Skipped:     summary.Skipped,
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal default skill marker: %w", err)
	}
	if err := os.WriteFile(filepath.Join(managedDir, defaultSkillMarkerName), append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write default skill marker: %w", err)
	}
	return nil
}