package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oscarcode/elementary-claw/internal/config"
)

const (
	DefaultAPIBaseURL = "https://api.openai.com/v1"
	DefaultCodexModel = "gpt-5.4"
)

type authStore struct {
	Profiles map[string]authProfile `json:"profiles"`
}

type authProfile struct {
	Provider string `json:"provider"`
	Mode     string `json:"mode"`
	Type     string `json:"type"`
	Token    string `json:"token"`
}

func ResolveAPIKey(paths config.Paths, explicit string) (string, string, error) {
	if key := strings.TrimSpace(explicit); key != "" {
		return key, "flag", nil
	}
	if key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); key != "" {
		return key, "env:OPENAI_API_KEY", nil
	}

	data, err := os.ReadFile(paths.AuthPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", errors.New("no OpenAI API key found in environment or auth store")
		}
		return "", "", fmt.Errorf("read auth store: %w", err)
	}

	var store authStore
	if err := json.Unmarshal(data, &store); err != nil {
		return "", "", fmt.Errorf("decode auth store: %w", err)
	}

	profile, ok := store.Profiles["openai:default"]
	if !ok {
		return "", "", errors.New("no OpenAI API key found in environment or auth store")
	}

	if profile.Provider != "openai" || strings.TrimSpace(profile.Token) == "" {
		return "", "", errors.New("no OpenAI API key found in environment or auth store")
	}

	return strings.TrimSpace(profile.Token), "auth-store:openai:default", nil
}

func SaveAPIKey(paths config.Paths, apiKey string) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return errors.New("OpenAI API key must not be empty")
	}

	var store map[string]any
	data, err := os.ReadFile(paths.AuthPath)
	if err == nil {
		_ = json.Unmarshal(data, &store)
	}
	if store == nil {
		store = map[string]any{
			"version":  1,
			"profiles": map[string]any{},
		}
	}

	profiles, _ := store["profiles"].(map[string]any)
	if profiles == nil {
		profiles = map[string]any{}
	}

	profiles["openai:default"] = map[string]any{
		"provider": "openai",
		"mode":     "active",
		"type":     "api_key",
		"token":    apiKey,
	}
	store["profiles"] = profiles

	if err := os.MkdirAll(filepath.Dir(paths.AuthPath), 0o700); err != nil {
		return fmt.Errorf("create auth directory: %w", err)
	}

	payload, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encode auth store: %w", err)
	}

	if err := os.WriteFile(paths.AuthPath, append(payload, '\n'), 0o600); err != nil {
		return fmt.Errorf("write auth store: %w", err)
	}

	return nil
}
