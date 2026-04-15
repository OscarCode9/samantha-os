package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/tools"
)

// LoadTools connects to all remote MCP servers declared in cfg.Mcp.Servers,
// resolves auth (stored token or pre-set headers), performs MCP initialization,
// lists tools, and registers them in registry.  Servers with no URL are
// silently skipped (stdio servers are not yet supported).
// Each server is loaded independently; errors are collected and returned as a
// combined error but do not prevent other servers from loading.
func LoadTools(ctx context.Context, paths config.Paths, cfg config.FileConfig, registry *tools.Registry) error {
	var errs []string
	for name, srv := range cfg.Mcp.Servers {
		if srv.URL == "" {
			continue
		}
		if err := loadServerTools(ctx, paths, name, srv, registry); err != nil {
			errs = append(errs, fmt.Sprintf("MCP server %q: %v", name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func loadServerTools(ctx context.Context, paths config.Paths, name string, srv config.McpServerConfig, registry *tools.Registry) error {
	authValue, err := resolveAuth(paths, name, srv)
	if err != nil {
		return err
	}

	client := NewClient(srv.URL, authValue)

	initCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := client.Initialize(initCtx); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	listCtx, cancel2 := context.WithTimeout(ctx, 15*time.Second)
	defer cancel2()

	toolInfos, err := client.ListTools(listCtx)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}

	for _, info := range toolInfos {
		t := NewTool(client, info)
		// Prefix tool names with the server name to avoid collisions:
		// "logired__get_shipment" instead of "get_shipment".
		prefixed := prefixedTool{Tool: t, prefix: name + "__"}
		registry.Register(prefixed)
	}
	fmt.Fprintf(os.Stderr, "loaded %d MCP tool(s) from server %q\n", len(toolInfos), name)
	return nil
}

// ResolveAuth returns the Authorization header value to use for the given
// server config.  Priority:
// 1. Cached OAuth token file (if OAuthClientID is set).
// 2. Explicit headers["Authorization"].
// 3. Empty string (no auth).
func ResolveAuth(paths config.Paths, name string, srv config.McpServerConfig) (string, error) {
	return resolveAuth(paths, name, srv)
}

// ExtractIssuer derives an OAuth2 issuer base URL from an MCP endpoint URL.
func ExtractIssuer(mcpURL string) string {
	return extractIssuerFromURL(mcpURL)
}

func resolveAuth(paths config.Paths, name string, srv config.McpServerConfig) (string, error) {
	if srv.OAuthClientID != "" {
		tok, err := LoadToken(paths.McpTokenPath(name))
		if err != nil {
			return "", fmt.Errorf("load token: %w", err)
		}
		if tok != nil && tok.Valid() {
			return "Bearer " + tok.AccessToken, nil
		}
		// Try refresh if we have a refresh token.
		if tok != nil && tok.RefreshToken != "" {
			tokenURL := srv.OAuthTokenURL
			if tokenURL == "" {
				_, tokenURL, err = DiscoverMetadata(extractIssuerFromURL(srv.URL))
				if err != nil {
					return "", fmt.Errorf("discover OAuth metadata: %w", err)
				}
			}
			refreshed, err := RefreshAccessToken(tokenURL, srv.OAuthClientID, srv.OAuthClientSecret, tok.RefreshToken)
			if err == nil && refreshed.Valid() {
				_ = SaveToken(paths.McpTokenPath(name), refreshed)
				return "Bearer " + refreshed.AccessToken, nil
			}
		}
		return "", fmt.Errorf("no valid token for MCP server %q — run: claw mcp auth %s", name, name)
	}

	if srv.Headers != nil {
		for h, v := range srv.Headers {
			if strings.EqualFold(h, "authorization") {
				return v, nil
			}
		}
	}
	return "", nil
}

// extractIssuerFromURL derives an issuer base URL from the MCP endpoint URL
// (scheme + host, dropping path).
func extractIssuerFromURL(mcpURL string) string {
	for _, scheme := range []string{"https://", "http://"} {
		if idx := strings.Index(mcpURL, scheme); idx == 0 {
			rest := mcpURL[len(scheme):]
			if slash := strings.IndexByte(rest, '/'); slash >= 0 {
				return scheme + rest[:slash]
			}
			return mcpURL
		}
	}
	return mcpURL
}

// prefixedTool wraps a Tool and prepends a prefix to its Name() so that tools
// from different MCP servers don't collide.
type prefixedTool struct {
	tools.Tool
	prefix string
}

func (p prefixedTool) Name() string {
	return p.prefix + p.Tool.Name()
}
