package githubcopilot

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/oscarcode/elementary-claw/internal/config"
)

func TestDeriveAPIBaseURLFromToken(t *testing.T) {
	token := "foo=bar; proxy-ep=proxy.individual.githubcopilot.com; baz=qux"
	got := DeriveAPIBaseURLFromToken(token)
	if got != "https://api.individual.githubcopilot.com" {
		t.Fatalf("unexpected base URL: %s", got)
	}
}

func TestDeriveAPIBaseURLFromTokenFallsBack(t *testing.T) {
	got := DeriveAPIBaseURLFromToken("no proxy endpoint here")
	if got != DefaultCopilotAPIBaseURL {
		t.Fatalf("unexpected fallback base URL: %s", got)
	}
}

func TestResolveAPITokenFetchesAndCaches(t *testing.T) {
	tempDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/copilot_internal/v2/token" {
			response.WriteHeader(http.StatusNotFound)
			return
		}
		if request.Header.Get("Authorization") != "Bearer github-token" {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"token":"foo=bar; proxy-ep=proxy.individual.githubcopilot.com","expires_at":4102444800}`))
	}))
	defer server.Close()

	paths := config.Paths{
		CopilotTokenCachePath: filepath.Join(tempDir, "github-copilot.token.json"),
	}

	result, err := ResolveAPIToken(ResolveParams{
		Paths:       paths,
		GitHubToken: "github-token",
		TokenURL:    server.URL + "/copilot_internal/v2/token",
		UseCache:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.BaseURL != "https://api.individual.githubcopilot.com" {
		t.Fatalf("unexpected base URL: %s", result.BaseURL)
	}
	if result.Source != "fetched:"+server.URL+"/copilot_internal/v2/token" {
		t.Fatalf("unexpected source: %s", result.Source)
	}
	if result.GitHubTokenSource != "flag" {
		t.Fatalf("unexpected github token source: %s", result.GitHubTokenSource)
	}
	if result.Token == "" {
		t.Fatal("expected exchanged token")
	}
	if _, _, err := loadCachedToken(paths.CopilotTokenCachePath); err != nil {
		t.Fatalf("expected cache to be written: %v", err)
	}
}

func TestResolveAPITokenReadsGitHubTokenFromAuthStore(t *testing.T) {
	tempDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/copilot_internal/v2/token" {
			response.WriteHeader(http.StatusNotFound)
			return
		}
		if request.Header.Get("Authorization") != "Bearer github-token-from-auth-store" {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"token":"foo=bar; proxy-ep=proxy.individual.githubcopilot.com","expires_at":4102444800}`))
	}))
	defer server.Close()

	paths := config.Paths{
		AuthPath:              filepath.Join(tempDir, "auth-profiles.json"),
		CopilotTokenCachePath: filepath.Join(tempDir, "github-copilot.token.json"),
	}
	if err := os.WriteFile(paths.AuthPath, []byte(`{
  "version": 1,
  "profiles": {
    "github-copilot:default": {
      "provider": "github-copilot",
      "mode": "token",
      "token": "github-token-from-auth-store"
    }
  }
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := ResolveAPIToken(ResolveParams{
		Paths:    paths,
		TokenURL: server.URL + "/copilot_internal/v2/token",
		UseCache: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GitHubTokenSource != "auth-store:github-copilot:default" {
		t.Fatalf("unexpected github token source: %s", result.GitHubTokenSource)
	}
}
