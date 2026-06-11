package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGmailCredentialsJSONInstalled(t *testing.T) {
	creds, err := parseGmailCredentialsJSON(`{
		"installed": {
			"client_id": "client-id.apps.googleusercontent.com",
			"client_secret": "secret-value"
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if creds.ClientID != "client-id.apps.googleusercontent.com" {
		t.Fatalf("unexpected client id: %q", creds.ClientID)
	}
	if creds.ClientSecret != "secret-value" {
		t.Fatalf("unexpected client secret: %q", creds.ClientSecret)
	}
}

func TestParseGmailCredentialsJSONWeb(t *testing.T) {
	creds, err := parseGmailCredentialsJSON(`{
		"web": {
			"client_id": "web-client-id.apps.googleusercontent.com",
			"client_secret": "web-secret"
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if creds.ClientID != "web-client-id.apps.googleusercontent.com" {
		t.Fatalf("unexpected client id: %q", creds.ClientID)
	}
	if creds.ClientSecret != "web-secret" {
		t.Fatalf("unexpected client secret: %q", creds.ClientSecret)
	}
}

func TestResolveConnectGmailCredentialsRejectsMultipleSources(t *testing.T) {
	_, _, err := resolveConnectGmailCredentials(connectGmailParams{
		ClientID:        "client-id",
		ClientSecret:    "secret",
		CredentialsJSON: `{"installed":{"client_id":"json-id","client_secret":"json-secret"}}`,
	})
	if err == nil {
		t.Fatal("expected an error when multiple credential sources are provided")
	}
	if !strings.Contains(err.Error(), "usa sólo una fuente") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveConnectGmailCredentialsExpandsHomePath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	jsonPath := filepath.Join(homeDir, "gmail-client.json")
	writeErr := os.WriteFile(jsonPath, []byte(`{
		"installed": {
			"client_id": "path-client-id.apps.googleusercontent.com",
			"client_secret": "path-secret"
		}
	}`), 0o600)
	if writeErr != nil {
		t.Fatal(writeErr)
	}

	creds, source, err := resolveConnectGmailCredentials(connectGmailParams{
		CredentialsPath: "~/gmail-client.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if source != jsonPath {
		t.Fatalf("unexpected source: %q", source)
	}
	if creds.ClientID != "path-client-id.apps.googleusercontent.com" {
		t.Fatalf("unexpected client id: %q", creds.ClientID)
	}
}

func TestResolveConnectGmailCredentialsReturnsNotConfiguredError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	_, _, err := resolveConnectGmailCredentials(connectGmailParams{})
	if !errors.Is(err, errGmailNotConfigured) {
		t.Fatalf("expected errGmailNotConfigured, got %v", err)
	}
}
