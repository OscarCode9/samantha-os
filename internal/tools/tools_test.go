package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- exec tool tests ---

func TestExecToolEcho(t *testing.T) {
	tool := NewExecTool(ExecToolOptions{})
	result := tool.Execute(context.Background(), `{"command":"echo hello world"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if strings.TrimSpace(result.Content) != "hello world" {
		t.Fatalf("unexpected output: %q", result.Content)
	}
}

func TestExecToolEmptyCommand(t *testing.T) {
	tool := NewExecTool(ExecToolOptions{})
	result := tool.Execute(context.Background(), `{"command":""}`)
	if !result.IsError {
		t.Fatal("expected error for empty command")
	}
}

func TestExecToolExitCode(t *testing.T) {
	tool := NewExecTool(ExecToolOptions{})
	result := tool.Execute(context.Background(), `{"command":"exit 42"}`)
	if !result.IsError {
		t.Fatal("expected error for non-zero exit")
	}
	if !strings.Contains(result.Content, "exit code: 42") {
		t.Fatalf("expected exit code 42 in output: %s", result.Content)
	}
}

func TestExecToolWorkdir(t *testing.T) {
	dir := t.TempDir()
	tool := NewExecTool(ExecToolOptions{DefaultWorkdir: dir})
	result := tool.Execute(context.Background(), `{"command":"pwd"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if strings.TrimSpace(result.Content) != dir {
		t.Fatalf("unexpected workdir: %q (expected %q)", strings.TrimSpace(result.Content), dir)
	}
}

func TestExecToolInvalidJSON(t *testing.T) {
	tool := NewExecTool(ExecToolOptions{})
	result := tool.Execute(context.Background(), `not json`)
	if !result.IsError {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- read_file tool tests ---

func TestReadFileToolBasic(t *testing.T) {
	dir := t.TempDir()
	content := "line one\nline two\nline three\n"
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewReadFileTool(dir)
	result := tool.Execute(context.Background(), `{"path":"test.txt"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "1: line one") {
		t.Fatalf("expected numbered line 1: %s", result.Content)
	}
	if !strings.Contains(result.Content, "3: line three") {
		t.Fatalf("expected numbered line 3: %s", result.Content)
	}
}

func TestReadFileToolOffset(t *testing.T) {
	dir := t.TempDir()
	content := "a\nb\nc\nd\ne\n"
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewReadFileTool(dir)
	result := tool.Execute(context.Background(), `{"path":"test.txt","offset":3,"limit":2}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "3: c") {
		t.Fatalf("expected line 3: %s", result.Content)
	}
	if !strings.Contains(result.Content, "4: d") {
		t.Fatalf("expected line 4: %s", result.Content)
	}
	if strings.Contains(result.Content, "5: e") {
		t.Fatalf("should not contain line 5: %s", result.Content)
	}
}

func TestReadFileToolNotFound(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())
	result := tool.Execute(context.Background(), `{"path":"nonexistent.txt"}`)
	if !result.IsError {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(result.Content, "not found") {
		t.Fatalf("expected 'not found' in error: %s", result.Content)
	}
}

func TestReadFileToolDirectory(t *testing.T) {
	dir := t.TempDir()
	tool := NewReadFileTool(dir)
	result := tool.Execute(context.Background(), `{"path":"."}`)
	if !result.IsError {
		t.Fatal("expected error for directory")
	}
	if !strings.Contains(result.Content, "directory") {
		t.Fatalf("expected 'directory' in error: %s", result.Content)
	}
}

// --- write_file tool tests ---

func TestWriteFileToolBasic(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFileTool(dir)

	result := tool.Execute(context.Background(), `{"path":"output.txt","content":"hello world"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	data, err := os.ReadFile(filepath.Join(dir, "output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Fatalf("unexpected file content: %q", string(data))
	}
}

func TestWriteFileToolCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFileTool(dir)

	result := tool.Execute(context.Background(), `{"path":"sub/dir/file.txt","content":"nested"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	data, err := os.ReadFile(filepath.Join(dir, "sub", "dir", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "nested" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

// --- edit_file tool tests ---

func TestEditFileToolBasicReplace(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "code.go"), []byte("func main() {\n\treturn 42\n}\n"), 0o644)

	tool := NewEditFileTool(dir)
	result := tool.Execute(context.Background(), `{"path":"code.go","old_string":"return 42","new_string":"return 0"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "1 replacement") {
		t.Fatalf("expected 1 replacement in output: %s", result.Content)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "code.go"))
	content := string(data)
	if !strings.Contains(content, "return 0") {
		t.Fatalf("expected 'return 0' in edited file: %s", content)
	}
}

func TestEditFileToolReplaceAll(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("foo bar foo baz foo"), 0o644)

	tool := NewEditFileTool(dir)
	result := tool.Execute(context.Background(), `{"path":"test.txt","old_string":"foo","new_string":"qux","replace_all":"true"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "3 replacement") {
		t.Fatalf("expected 3 replacements: %s", result.Content)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "test.txt"))
	if string(data) != "qux bar qux baz qux" {
		t.Fatalf("unexpected content after replace all: %q", string(data))
	}
}

func TestEditFileToolMultipleMatchesWithoutReplaceAll(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("aaa bbb aaa"), 0o644)

	tool := NewEditFileTool(dir)
	result := tool.Execute(context.Background(), `{"path":"test.txt","old_string":"aaa","new_string":"ccc"}`)
	if !result.IsError {
		t.Fatal("expected error for multiple matches without replace_all")
	}
	if !strings.Contains(result.Content, "2 matches") {
		t.Fatalf("expected '2 matches' in error: %s", result.Content)
	}
}

func TestEditFileToolNotFound(t *testing.T) {
	tool := NewEditFileTool(t.TempDir())
	result := tool.Execute(context.Background(), `{"path":"missing.txt","old_string":"a","new_string":"b"}`)
	if !result.IsError {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(result.Content, "not found") {
		t.Fatalf("expected 'not found': %s", result.Content)
	}
}

func TestEditFileToolOldStringNotFound(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world"), 0o644)

	tool := NewEditFileTool(dir)
	result := tool.Execute(context.Background(), `{"path":"test.txt","old_string":"nonexistent","new_string":"replacement"}`)
	if !result.IsError {
		t.Fatal("expected error when old_string not found")
	}
	if !strings.Contains(result.Content, "not found in file") {
		t.Fatalf("expected 'not found in file': %s", result.Content)
	}
}

func TestEditFileToolEmptyOldString(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("content"), 0o644)

	tool := NewEditFileTool(dir)
	result := tool.Execute(context.Background(), `{"path":"test.txt","old_string":"","new_string":"x"}`)
	if !result.IsError {
		t.Fatal("expected error for empty old_string")
	}
}

func TestEditFileToolIdenticalStrings(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("same"), 0o644)

	tool := NewEditFileTool(dir)
	result := tool.Execute(context.Background(), `{"path":"test.txt","old_string":"same","new_string":"same"}`)
	if !result.IsError {
		t.Fatal("expected error for identical old/new strings")
	}
	if !strings.Contains(result.Content, "identical") {
		t.Fatalf("expected 'identical' in error: %s", result.Content)
	}
}

func TestEditFileToolDeleteString(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("keep remove keep"), 0o644)

	tool := NewEditFileTool(dir)
	result := tool.Execute(context.Background(), `{"path":"test.txt","old_string":" remove","new_string":""}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "test.txt"))
	if string(data) != "keep keep" {
		t.Fatalf("unexpected content after delete: %q", string(data))
	}
}

func TestEditFileToolInvalidJSON(t *testing.T) {
	tool := NewEditFileTool(t.TempDir())
	result := tool.Execute(context.Background(), `not json`)
	if !result.IsError {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestEditFileToolDirectory(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditFileTool(dir)
	result := tool.Execute(context.Background(), `{"path":".","old_string":"a","new_string":"b"}`)
	if !result.IsError {
		t.Fatal("expected error for directory path")
	}
	if !strings.Contains(result.Content, "directory") {
		t.Fatalf("expected 'directory' in error: %s", result.Content)
	}
}

// --- list_dir tool tests ---

func TestListDirToolBasic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("b"), 0o644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0o755)

	tool := NewListDirTool(dir)
	result := tool.Execute(context.Background(), `{"path":"."}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "subdir/") {
		t.Fatalf("expected subdir/ in output: %s", result.Content)
	}
	if !strings.Contains(result.Content, "a.txt") {
		t.Fatalf("expected a.txt in output: %s", result.Content)
	}
}

func TestListDirToolNotFound(t *testing.T) {
	tool := NewListDirTool(t.TempDir())
	result := tool.Execute(context.Background(), `{"path":"nonexistent"}`)
	if !result.IsError {
		t.Fatal("expected error for missing directory")
	}
}

func TestListDirToolOnFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644)

	tool := NewListDirTool(dir)
	result := tool.Execute(context.Background(), `{"path":"file.txt"}`)
	if !result.IsError {
		t.Fatal("expected error for file path")
	}
	if !strings.Contains(result.Content, "not a directory") {
		t.Fatalf("expected 'not a directory' error: %s", result.Content)
	}
}

// --- grep_search tool tests ---

func TestGrepSearchToolBasic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.go"), []byte("func main() {\n\tfmt.Println(\"hello\")\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "other.txt"), []byte("no match here\n"), 0o644)

	tool := NewGrepSearchTool(dir)
	result := tool.Execute(context.Background(), `{"pattern":"Println"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "hello.go:2") {
		t.Fatalf("expected match in hello.go:2: %s", result.Content)
	}
}

func TestGrepSearchToolNoMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("nothing here"), 0o644)

	tool := NewGrepSearchTool(dir)
	result := tool.Execute(context.Background(), `{"pattern":"zzznomatch"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "no matches") {
		t.Fatalf("expected 'no matches': %s", result.Content)
	}
}

func TestGrepSearchToolInclude(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("func main() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("func main() {}\n"), 0o644)

	tool := NewGrepSearchTool(dir)
	result := tool.Execute(context.Background(), `{"pattern":"func","include":"*.go"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "main.go") {
		t.Fatalf("expected main.go in results: %s", result.Content)
	}
	if strings.Contains(result.Content, "readme.md") {
		t.Fatalf("should not include readme.md: %s", result.Content)
	}
}

func TestGrepSearchToolInvalidRegex(t *testing.T) {
	tool := NewGrepSearchTool(t.TempDir())
	result := tool.Execute(context.Background(), `{"pattern":"[invalid"}`)
	if !result.IsError {
		t.Fatal("expected error for invalid regex")
	}
}

// --- glob tool tests ---

func TestGlobToolBasic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte(""), 0o644)

	tool := NewGlobTool(dir)
	result := tool.Execute(context.Background(), `{"pattern":"*.go"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "a.go") {
		t.Fatalf("expected a.go: %s", result.Content)
	}
	if !strings.Contains(result.Content, "b.go") {
		t.Fatalf("expected b.go: %s", result.Content)
	}
	if strings.Contains(result.Content, "c.txt") {
		t.Fatalf("should not contain c.txt: %s", result.Content)
	}
}

func TestGlobToolRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	os.WriteFile(filepath.Join(dir, "root.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(sub, "nested.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(sub, "other.txt"), []byte(""), 0o644)

	tool := NewGlobTool(dir)
	result := tool.Execute(context.Background(), `{"pattern":"**/*.go"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "root.go") {
		t.Fatalf("expected root.go: %s", result.Content)
	}
	if !strings.Contains(result.Content, "nested.go") {
		t.Fatalf("expected nested.go: %s", result.Content)
	}
	if strings.Contains(result.Content, "other.txt") {
		t.Fatalf("should not contain other.txt: %s", result.Content)
	}
}

func TestGlobToolNoMatch(t *testing.T) {
	tool := NewGlobTool(t.TempDir())
	result := tool.Execute(context.Background(), `{"pattern":"*.xyz"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "no files matched") {
		t.Fatalf("expected 'no files matched': %s", result.Content)
	}
}

// --- web_fetch tool tests ---

func TestWebFetchToolBasic(t *testing.T) {
	srv := newTestHTTPServer(t, http.StatusOK, "text/plain", "hello from server")
	defer srv.Close()

	tool := NewWebFetchTool()
	result := tool.Execute(context.Background(), `{"url":"`+srv.URL+`"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "HTTP 200") {
		t.Fatalf("expected HTTP 200 in output: %s", result.Content)
	}
	if !strings.Contains(result.Content, "hello from server") {
		t.Fatalf("expected body content: %s", result.Content)
	}
}

func TestWebFetchToolJSON(t *testing.T) {
	srv := newTestHTTPServer(t, http.StatusOK, "application/json", `{"key":"value"}`)
	defer srv.Close()

	tool := NewWebFetchTool()
	result := tool.Execute(context.Background(), `{"url":"`+srv.URL+`"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "application/json") {
		t.Fatalf("expected Content-Type in output: %s", result.Content)
	}
}

func TestWebFetchToolHTTPError(t *testing.T) {
	srv := newTestHTTPServer(t, http.StatusNotFound, "text/plain", "not found")
	defer srv.Close()

	tool := NewWebFetchTool()
	result := tool.Execute(context.Background(), `{"url":"`+srv.URL+`"}`)
	if !result.IsError {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(result.Content, "HTTP 404") {
		t.Fatalf("expected HTTP 404 in output: %s", result.Content)
	}
}

func TestWebFetchToolEmptyURL(t *testing.T) {
	tool := NewWebFetchTool()
	result := tool.Execute(context.Background(), `{"url":""}`)
	if !result.IsError {
		t.Fatal("expected error for empty URL")
	}
}

func TestWebFetchToolInvalidURL(t *testing.T) {
	tool := NewWebFetchTool()
	result := tool.Execute(context.Background(), `{"url":"not-a-url"}`)
	if !result.IsError {
		t.Fatal("expected error for invalid URL")
	}
}

func TestWebFetchToolCustomHeaders(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tool := NewWebFetchTool()
	result := tool.Execute(context.Background(), `{"url":"`+srv.URL+`","headers":"Authorization: Bearer test-token"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if receivedAuth != "Bearer test-token" {
		t.Fatalf("expected Authorization header, got: %q", receivedAuth)
	}
}

func TestWebFetchToolInvalidJSON(t *testing.T) {
	tool := NewWebFetchTool()
	result := tool.Execute(context.Background(), `not json`)
	if !result.IsError {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestWebFetchToolLargeResponse(t *testing.T) {
	largeBody := strings.Repeat("x", webFetchMaxBody+1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(largeBody))
	}))
	defer srv.Close()

	tool := NewWebFetchTool()
	result := tool.Execute(context.Background(), `{"url":"`+srv.URL+`"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "truncated") {
		t.Fatalf("expected 'truncated' in result for large response")
	}
}

// --- notify tool tests ---

func TestNotifyToolEmptySummary(t *testing.T) {
	tool := NewNotifyTool()
	result := tool.Execute(context.Background(), `{"summary":""}`)
	if !result.IsError {
		t.Fatal("expected error for empty summary")
	}
}

func TestNotifyToolInvalidJSON(t *testing.T) {
	tool := NewNotifyTool()
	result := tool.Execute(context.Background(), `not json`)
	if !result.IsError {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNotifyToolSendsNotification(t *testing.T) {
	tool := NewNotifyTool()
	result := tool.Execute(context.Background(), `{"summary":"Test notification","body":"from unit tests","urgency":"low"}`)
	if result.IsError && !strings.Contains(result.Content, "no notification command") {
		t.Logf("notify returned error (acceptable in CI): %s", result.Content)
	}
	if !result.IsError && !strings.Contains(result.Content, "notification sent") {
		t.Fatalf("expected 'notification sent' in output: %s", result.Content)
	}
}

func TestNotifyToolParameters(t *testing.T) {
	tool := NewNotifyTool()
	params := tool.Parameters()
	if params.Type != "object" {
		t.Fatalf("expected object type, got: %s", params.Type)
	}
	if _, ok := params.Properties["summary"]; !ok {
		t.Fatal("expected summary property")
	}
	if _, ok := params.Properties["body"]; !ok {
		t.Fatal("expected body property")
	}
	if _, ok := params.Properties["urgency"]; !ok {
		t.Fatal("expected urgency property")
	}
}

// --- path resolution tests ---

func TestResolvePathAbsolute(t *testing.T) {
	result := resolvePath("/absolute/path", "/workspace")
	if result != "/absolute/path" {
		t.Fatalf("expected absolute path unchanged: %s", result)
	}
}

func TestResolvePathRelative(t *testing.T) {
	result := resolvePath("relative/path", "/workspace")
	if result != "/workspace/relative/path" {
		t.Fatalf("expected joined path: %s", result)
	}
}

func TestResolvePathEmpty(t *testing.T) {
	result := resolvePath("", "/workspace")
	if result != "/workspace" {
		t.Fatalf("expected workspace root: %s", result)
	}
}

// --- helpers ---

func newTestHTTPServer(t *testing.T, status int, contentType string, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}
