package runtime

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/oscarcode/elementary-claw/internal/config"
)

func TestBuildUpstreamURLKeepsSingleV1ForProxyBase(t *testing.T) {
	target := upstreamTarget{
		BaseURL: "http://localhost:4000/openai/v1",
		Headers: make(http.Header),
	}

	got := buildUpstreamURL(target, "/v1/models")
	want := "http://localhost:4000/openai/v1/models"
	if got != want {
		t.Fatalf("unexpected upstream URL: got %s want %s", got, want)
	}
}

func TestBuildUpstreamURLKeepsV1ForNativeBase(t *testing.T) {
	target := upstreamTarget{
		BaseURL: "https://api.individual.githubcopilot.com",
		Headers: make(http.Header),
	}

	got := buildUpstreamURL(target, "/v1/chat/completions")
	want := "https://api.individual.githubcopilot.com/chat/completions"
	if got != want {
		t.Fatalf("unexpected upstream URL: got %s want %s", got, want)
	}
}

func TestResolveUpstreamTargetOpenAI(t *testing.T) {
	root := t.TempDir()
	paths := config.Paths{
		HomeDir:               root,
		StateDir:              filepath.Join(root, ".samantha"),
		WorkspaceDir:          filepath.Join(root, ".samantha", "workspace"),
		CredentialsDir:        filepath.Join(root, ".samantha", "state", "credentials"),
		SessionsDir:           filepath.Join(root, ".samantha", "state", "sessions"),
		ConfigPath:            filepath.Join(root, ".samantha", "samantha.json"),
		AuthPath:              filepath.Join(root, ".samantha", "agents", "main", "agent", "auth-profiles.json"),
		CopilotTokenCachePath: filepath.Join(root, ".samantha", "state", "credentials", "github-copilot.token.json"),
	}
	if err := os.MkdirAll(filepath.Dir(paths.AuthPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.AuthPath, []byte("{\n  \"version\": 1,\n  \"profiles\": {\n    \"openai:default\": {\n      \"provider\": \"openai\",\n      \"mode\": \"active\",\n      \"type\": \"api_key\",\n      \"token\": \"sk-openai-test\"\n    }\n  }\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	target, err := resolveUpstreamTarget(paths, config.FileConfig{
		Agent: config.AgentConfig{
			Provider: "openai",
			BaseURL:  "https://api.openai.com/v1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := target.Headers.Get("Authorization"); got != "Bearer sk-openai-test" {
		t.Fatalf("unexpected auth header: %q", got)
	}
	if target.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected base URL: %s", target.BaseURL)
	}
}
