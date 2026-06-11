package githubcopilot

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// CopilotClientID is the public GitHub OAuth client ID used by Copilot.
	CopilotClientID = "Iv1.b507a08c87ecfe98"

	deviceCodeURL    = "https://github.com/login/device/code"
	accessTokenURL   = "https://github.com/login/oauth/access_token"
	deviceFlowScope  = "read:user"
)

// DeviceCodeResponse holds the initial device code from GitHub.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// RequestDeviceCode initiates the device flow with GitHub.
// Returns the device code response which includes the user code to show.
func RequestDeviceCode() (*DeviceCodeResponse, error) {
	params := url.Values{
		"client_id": {CopilotClientID},
		"scope":     {deviceFlowScope},
	}

	req, err := http.NewRequest(http.MethodPost, deviceCodeURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read device code response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("device code request failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result DeviceCodeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode device code response: %w", err)
	}
	if result.DeviceCode == "" {
		return nil, fmt.Errorf("empty device code in response: %s", string(body))
	}
	if result.Interval <= 0 {
		result.Interval = 5
	}

	return &result, nil
}

// PollForAccessToken polls GitHub's OAuth endpoint until the user
// authorizes the device code, the code expires, or the context deadline
// is reached. Returns the GitHub access token (gho_ prefix).
func PollForAccessToken(deviceCode string, interval int, expiresIn int) (string, error) {
	if interval <= 0 {
		interval = 5
	}

	client := &http.Client{Timeout: 20 * time.Second}
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(time.Duration(interval) * time.Second)

		params := url.Values{
			"client_id":   {CopilotClientID},
			"device_code": {deviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}

		req, err := http.NewRequest(http.MethodPost, accessTokenURL, strings.NewReader(params.Encode()))
		if err != nil {
			return "", fmt.Errorf("build access token request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			continue // transient error, retry
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		var tokenResp struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
			Scope       string `json:"scope"`
			Error       string `json:"error"`
		}
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			continue
		}

		switch tokenResp.Error {
		case "":
			if tokenResp.AccessToken != "" {
				return tokenResp.AccessToken, nil
			}
			return "", fmt.Errorf("empty access token in response: %s", string(body))

		case "authorization_pending":
			continue

		case "slow_down":
			interval += 2
			continue

		case "expired_token":
			return "", fmt.Errorf("device code expired — user did not authorize in time")

		case "access_denied":
			return "", fmt.Errorf("user denied the authorization request")

		default:
			return "", fmt.Errorf("device flow error: %s", tokenResp.Error)
		}
	}

	return "", fmt.Errorf("device flow timed out after %d seconds", expiresIn)
}
