package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/oscarcode/elementary-claw/internal/bootstrap"
	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/mcp"
	"github.com/oscarcode/elementary-claw/internal/providers/copilotproxy"
	"github.com/oscarcode/elementary-claw/internal/providers/githubcopilot"
	"github.com/oscarcode/elementary-claw/internal/runtime"
	"github.com/oscarcode/elementary-claw/internal/session"
	"github.com/oscarcode/elementary-claw/internal/skills"
	"github.com/oscarcode/elementary-claw/internal/tools"
)

type App struct {
	paths config.Paths
	store *session.Store
}

func New() (*App, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, fmt.Errorf("resolve default paths: %w", err)
	}

	return &App{
		paths: paths,
		store: session.NewStore(paths),
	}, nil
}

func (app *App) Run(args []string) error {
	if len(args) == 0 {
		return app.printRootHelp()
	}

	switch args[0] {
	case "setup":
		return app.runSetup(args[1:])
	case "gateway":
		return app.runGateway(args[1:])
	case "bootstrap":
		return app.runBootstrap(args[1:])
	case "providers":
		return app.runProviders(args[1:])
	case "sessions":
		return app.runSessions(args[1:])
	case "tools":
		return app.runTools(args[1:])
	case "skills":
		return app.runSkills(args[1:])
	case "mcp":
		return app.runMcp(args[1:])
	case "version", "--version", "-v":
		fmt.Println("elementary-claw dev")
		return nil
	case "help", "--help", "-h":
		return app.printRootHelp()
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], rootHelp)
	}
}

func (app *App) runSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(discardWriter{})

	workspace := fs.String("workspace", app.paths.WorkspaceDir, "workspace directory")
	username := fs.String("user", "", "user account name")
	preferredName := fs.String("preferred-name", "", "preferred name for the user")
	assistantName := fs.String("assistant-name", "Claw", "assistant display name")
	assistantNature := fs.String("assistant-nature", "A local AI teammate for this computer", "assistant nature")
	assistantVibe := fs.String("assistant-vibe", "direct, warm, and pragmatic", "assistant vibe")
	provider := fs.String("provider", "github-copilot", "provider identifier")
	providerBaseURL := fs.String("provider-base-url", "", "provider base URL override")
	providerPending := fs.Bool("provider-pending", false, "mark provider as pending")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*username) == "" {
		return errors.New("setup requires --user")
	}

	options := config.SetupOptions{
		WorkspaceDir:    *workspace,
		UserName:        *username,
		PreferredName:   firstNonEmpty(*preferredName, *username),
		AssistantName:   *assistantName,
		AssistantNature: *assistantNature,
		AssistantVibe:   *assistantVibe,
		Provider:        *provider,
		ProviderBaseURL: resolveProviderBaseURL(*provider, *providerBaseURL),
		ProviderPending: *providerPending,
	}

	if err := config.InitializeWorkspace(app.paths, options); err != nil {
		return err
	}

	fmt.Printf("initialized workspace at %s\n", *workspace)
	return nil
}

func (app *App) runGateway(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return app.printGatewayHelp()
	}

	switch args[0] {
	case "serve":
		fs := flag.NewFlagSet("gateway serve", flag.ContinueOnError)
		fs.SetOutput(discardWriter{})
		listen := fs.String("listen", "127.0.0.1:4389", "listen address")
		workdir := fs.String("workdir", "", "workspace root for tool execution")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		registry := app.buildToolRegistry(*workdir)
		skillsRegistry := app.buildSkillsRegistry()
		return runtime.Serve(*listen, app.paths, app.store, registry, skillsRegistry)
	case "status":
		status, err := runtime.Inspect(app.paths, app.store)
		if err != nil {
			return err
		}

		fmt.Println(status.String())
		return nil
	default:
		return fmt.Errorf("unknown gateway subcommand %q", args[0])
	}
}

func (app *App) runBootstrap(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return app.printBootstrapHelp()
	}

	switch args[0] {
	case "first-message":
		fs := flag.NewFlagSet("bootstrap first-message", flag.ContinueOnError)
		fs.SetOutput(discardWriter{})
		message := fs.String("message", "", "explicit assistant message override")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		sessionRecord, err := bootstrap.GenerateFirstMessage(app.paths, app.store, *message)
		if err != nil {
			return err
		}

		fmt.Printf("bootstrap session written: %s\n", sessionRecord.Path)
		return nil
	case "ensure":
		fs := flag.NewFlagSet("bootstrap ensure", flag.ContinueOnError)
		fs.SetOutput(discardWriter{})
		message := fs.String("message", "", "explicit assistant message override")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		result, created, err := bootstrap.EnsureFirstMessage(app.paths, app.store, *message)
		if err != nil {
			return err
		}
		if created {
			fmt.Printf("bootstrap session written: %s\n", result.Path)
		} else {
			fmt.Println("bootstrap session already satisfied")
		}
		return nil
	default:
		return fmt.Errorf("unknown bootstrap subcommand %q", args[0])
	}
}

func (app *App) runSessions(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return app.printSessionsHelp()
	}

	switch args[0] {
	case "list":
		sessions, err := app.store.List()
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Println("no sessions found")
			return nil
		}

		for _, item := range sessions {
			fmt.Printf("%s\t%s\t%s\n", item.ID, item.Kind, item.Title)
		}
		return nil
	default:
		return fmt.Errorf("unknown sessions subcommand %q", args[0])
	}
}

func (app *App) runProviders(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return app.printProvidersHelp()
	}

	switch args[0] {
	case "github-copilot":
		return app.runGitHubCopilotProvider(args[1:])
	case "copilot-proxy":
		return app.runCopilotProxyProvider(args[1:])
	default:
		return fmt.Errorf("unknown providers subcommand %q", args[0])
	}
}

func (app *App) runGitHubCopilotProvider(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return app.printProvidersHelp()
	}

	switch args[0] {
	case "exchange":
		fs := flag.NewFlagSet("providers github-copilot exchange", flag.ContinueOnError)
		fs.SetOutput(discardWriter{})
		githubToken := fs.String("github-token", "", "GitHub token override")
		printToken := fs.Bool("print-token", false, "print the exchanged Copilot API token")
		noCache := fs.Bool("no-cache", false, "skip reading the cached Copilot token")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		result, err := githubcopilot.ResolveAPIToken(githubcopilot.ResolveParams{
			Paths:       app.paths,
			GitHubToken: *githubToken,
			UseCache:    !*noCache,
		})
		if err != nil {
			return err
		}

		fmt.Printf("provider=github-copilot\n")
		fmt.Printf("base_url=%s\n", result.BaseURL)
		fmt.Printf("expires_at=%d\n", result.ExpiresAt)
		fmt.Printf("source=%s\n", result.Source)
		fmt.Printf("github_token_source=%s\n", result.GitHubTokenSource)
		if *printToken {
			fmt.Printf("token=%s\n", result.Token)
		}
		return nil
	default:
		return fmt.Errorf("unknown github-copilot subcommand %q", args[0])
	}
}

func (app *App) runCopilotProxyProvider(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return app.printProvidersHelp()
	}

	switch args[0] {
	case "probe":
		fs := flag.NewFlagSet("providers copilot-proxy probe", flag.ContinueOnError)
		fs.SetOutput(discardWriter{})
		baseURL := fs.String("base-url", copilotproxy.DefaultBaseURL, "Copilot proxy base URL")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		result, err := copilotproxy.Probe(*baseURL)
		if err != nil {
			return err
		}

		fmt.Printf("provider=copilot-proxy\n")
		fmt.Printf("normalized_base_url=%s\n", result.NormalizedBaseURL)
		fmt.Printf("probe_path=%s\n", result.ProbePath)
		fmt.Printf("status_code=%d\n", result.StatusCode)
		fmt.Printf("ready=%t\n", result.Ready)
		fmt.Printf("detail=%s\n", result.Detail)
		return nil
	default:
		return fmt.Errorf("unknown copilot-proxy subcommand %q", args[0])
	}
}

// buildToolRegistry creates a Registry with all built-in tools wired to the
// given workspace root directory.  It also loads any remote MCP tools
// declared in the config file.
func (app *App) buildToolRegistry(workdir string) *tools.Registry {
	if workdir == "" {
		workdir = app.paths.HomeDir
	}

	registry := tools.NewRegistry()
	registry.Register(tools.NewExecTool(tools.ExecToolOptions{DefaultWorkdir: workdir}))
	registry.Register(tools.NewReadFileTool(workdir))
	registry.Register(tools.NewWriteFileTool(workdir))
	registry.Register(tools.NewListDirTool(workdir))
	registry.Register(tools.NewGrepSearchTool(workdir))
	registry.Register(tools.NewGlobTool(workdir))
	registry.Register(tools.NewEditFileTool(workdir))
	registry.Register(tools.NewWebFetchTool())
	registry.Register(tools.NewNotifyTool())

	// Load remote MCP tools from config (best-effort: log on error, don't abort).
	cfg, err := config.LoadFileConfig(app.paths)
	if err == nil && len(cfg.Mcp.Servers) > 0 {
		if err := mcp.LoadTools(context.Background(), app.paths, cfg, registry); err != nil {
			// Some servers may have failed but others loaded fine.
			fmt.Fprintf(os.Stderr, "warning: loading MCP tools: %v\n", err)
		}
	}

	return registry
}

// buildSkillsRegistry loads skills from all configured directories (bundled,
// managed, workspace) with proper precedence.
func (app *App) buildSkillsRegistry() *skills.Registry {
	registry := skills.NewRegistry()
	dirs := map[skills.Source]string{
		skills.SourceBundled:   app.paths.BundledSkillsDir,
		skills.SourceManaged:   app.paths.ManagedSkillsDir,
		skills.SourceWorkspace: app.paths.WorkspaceSkillsDir,
	}
	n, err := registry.LoadMultiSource(dirs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: loading skills: %v\n", err)
	}
	if n > 0 {
		fmt.Fprintf(os.Stderr, "loaded %d skill(s)\n", n)
	}
	return registry
}

func (app *App) runSkills(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return app.printSkillsHelp()
	}

	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("skills list", flag.ContinueOnError)
		fs.SetOutput(discardWriter{})
		jsonOutput := fs.Bool("json", false, "output as JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		registry := app.buildSkillsRegistry()

		if *jsonOutput {
			data, err := json.MarshalIndent(registry.ToJSON(), "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		items := registry.List()
		if len(items) == 0 {
			fmt.Println("no skills loaded")
			return nil
		}

		for _, s := range items {
			enabled := "✓"
			if !s.Enabled {
				enabled = "✗"
			}
			emoji := ""
			if s.Manifest != nil && s.Manifest.Metadata.OpenClaw != nil && s.Manifest.Metadata.OpenClaw.Emoji != "" {
				emoji = s.Manifest.Metadata.OpenClaw.Emoji + " "
			}
			fmt.Printf("[%s] %s%-16s %-8s %s\n", enabled, emoji, s.Name, s.Source, s.Description)
		}
		return nil

	case "info":
		if len(args) < 2 {
			return errors.New("skills info requires a skill name")
		}
		registry := app.buildSkillsRegistry()
		s, ok := registry.Get(args[1])
		if !ok {
			return fmt.Errorf("skill %q not found", args[1])
		}

		fmt.Printf("name:        %s\n", s.Name)
		fmt.Printf("title:       %s\n", s.Title)
		fmt.Printf("description: %s\n", s.Description)
		fmt.Printf("source:      %s\n", s.Source)
		fmt.Printf("enabled:     %t\n", s.Enabled)
		fmt.Printf("path:        %s\n", s.Path)

		if s.Manifest != nil {
			if s.Manifest.Homepage != "" {
				fmt.Printf("homepage:    %s\n", s.Manifest.Homepage)
			}
			if s.Manifest.Metadata.OpenClaw != nil {
				oc := s.Manifest.Metadata.OpenClaw
				if oc.Emoji != "" {
					fmt.Printf("emoji:       %s\n", oc.Emoji)
				}
				if oc.Requires != nil {
					if len(oc.Requires.Bins) > 0 {
						fmt.Printf("requires:    bins=%s\n", strings.Join(oc.Requires.Bins, ", "))
					}
					if len(oc.Requires.Env) > 0 {
						fmt.Printf("requires:    env=%s\n", strings.Join(oc.Requires.Env, ", "))
					}
				}
			}
		}

		// Check requirements
		unmet := skills.CheckRequirements(s)
		if len(unmet) > 0 {
			fmt.Println("\nunmet requirements:")
			for _, u := range unmet {
				fmt.Printf("  - %s\n", u)
			}
		}

		return nil

	case "check":
		registry := app.buildSkillsRegistry()
		items := registry.List()
		if len(items) == 0 {
			fmt.Println("no skills to check")
			return nil
		}

		allOK := true
		for _, s := range items {
			unmet := skills.CheckRequirements(s)
			if len(unmet) > 0 {
				allOK = false
				fmt.Printf("%s: FAIL\n", s.Name)
				for _, u := range unmet {
					fmt.Printf("  - %s\n", u)
				}
			} else {
				fmt.Printf("%s: OK\n", s.Name)
			}
		}
		if allOK {
			fmt.Println("\nall skills satisfied")
		}
		return nil

	case "install":
		if len(args) < 2 {
			return errors.New("skills install requires a source (path or URL)")
		}
		registry := app.buildSkillsRegistry()
		installed, err := registry.Install(args[1], app.paths.ManagedSkillsDir)
		if err != nil {
			return err
		}
		fmt.Printf("installed skill %q at %s\n", installed.Name, installed.Path)
		return nil

	case "remove":
		if len(args) < 2 {
			return errors.New("skills remove requires a skill name")
		}
		registry := app.buildSkillsRegistry()
		if err := registry.Uninstall(args[1], app.paths.ManagedSkillsDir); err != nil {
			return err
		}
		fmt.Printf("removed skill %q\n", args[1])
		return nil

	default:
		return fmt.Errorf("unknown skills subcommand %q", args[0])
	}
}

func (app *App) runTools(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return app.printToolsHelp()
	}

	switch args[0] {
	case "list":
		registry := app.buildToolRegistry("")
		for _, t := range registry.List() {
			fmt.Printf("%-16s %s\n", t.Name(), t.Description())
		}
		return nil
	case "inspect":
		if len(args) < 2 {
			return errors.New("tools inspect requires a tool name")
		}
		registry := app.buildToolRegistry("")
		t, ok := registry.Get(args[1])
		if !ok {
			return fmt.Errorf("unknown tool %q", args[1])
		}
		fmt.Printf("name:        %s\n", t.Name())
		fmt.Printf("description: %s\n", t.Description())
		schema := t.Parameters()
		for propName, prop := range schema.Properties {
			required := ""
			for _, r := range schema.Required {
				if r == propName {
					required = " (required)"
					break
				}
			}
			fmt.Printf("  %-12s %-8s %s%s\n", propName, prop.Type, prop.Description, required)
		}
		return nil
	default:
		return fmt.Errorf("unknown tools subcommand %q", args[0])
	}
}

func (app *App) runMcp(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Print(mcpHelp)
		return nil
	}

	switch args[0] {
	case "auth":
		if len(args) < 2 {
			return fmt.Errorf("usage: claw mcp auth <server-name>")
		}
		return app.runMcpAuth(args[1])

	case "list-tools":
		if len(args) < 2 {
			return fmt.Errorf("usage: claw mcp list-tools <server-name>")
		}
		return app.runMcpListTools(args[1])

	default:
		return fmt.Errorf("unknown mcp subcommand %q\n\n%s", args[0], mcpHelp)
	}
}

// runMcpAuth runs the OAuth2 authorization code + PKCE browser flow for the
// named MCP server and caches the resulting token.
func (app *App) runMcpAuth(serverName string) error {
	cfg, err := config.LoadFileConfig(app.paths)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	srv, ok := cfg.Mcp.Servers[serverName]
	if !ok {
		return fmt.Errorf("MCP server %q not found in config — add it to ~/.openclaw/openclaw.json first", serverName)
	}
	if srv.OAuthClientID == "" {
		return fmt.Errorf("MCP server %q has no oauthClientId configured", serverName)
	}

	authEndpoint, tokenEndpoint := srv.OAuthTokenURL, srv.OAuthTokenURL
	if authEndpoint == "" {
		authEndpoint, tokenEndpoint, err = mcp.DiscoverMetadata(mcp.ExtractIssuer(srv.URL))
		if err != nil {
			return fmt.Errorf("discover OAuth metadata: %w", err)
		}
	}

	scopes := "mcp:tools"
	if len(srv.OAuthScopes) > 0 {
		scopes = strings.Join(srv.OAuthScopes, " ")
	}

	return mcp.RunAuthFlow(mcp.AuthFlowParams{
		ServerName:    serverName,
		ClientID:      srv.OAuthClientID,
		ClientSecret:  srv.OAuthClientSecret,
		AuthEndpoint:  authEndpoint,
		TokenEndpoint: tokenEndpoint,
		Scopes:        scopes,
		TokenPath:     app.paths.McpTokenPath(serverName),
		RedirectURI:   srv.OAuthRedirectURI,
	})
}

// runMcpListTools lists all tools exposed by the named MCP server.
func (app *App) runMcpListTools(serverName string) error {
	cfg, err := config.LoadFileConfig(app.paths)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	srv, ok := cfg.Mcp.Servers[serverName]
	if !ok {
		return fmt.Errorf("MCP server %q not found in config", serverName)
	}

	authValue, err := mcp.ResolveAuth(app.paths, serverName, srv)
	if err != nil {
		return err
	}

	client := mcp.NewClient(srv.URL, authValue)
	ctx := context.Background()

	if err := client.Initialize(ctx); err != nil {
		return err
	}

	toolInfos, err := client.ListTools(ctx)
	if err != nil {
		return err
	}

	if len(toolInfos) == 0 {
		fmt.Println("no tools exposed by this server")
		return nil
	}

	fmt.Printf("Tools from MCP server %q (%d):\n", serverName, len(toolInfos))
	for _, t := range toolInfos {
		desc := t.Description
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		fmt.Printf("  %-32s %s\n", t.Name, desc)
	}
	return nil
}

func (app *App) printRootHelp() error {
	fmt.Print(rootHelp)
	return nil
}

func (app *App) printGatewayHelp() error {
	fmt.Print(gatewayHelp)
	return nil
}

func (app *App) printBootstrapHelp() error {
	fmt.Print(bootstrapHelp)
	return nil
}

func (app *App) printSessionsHelp() error {
	fmt.Print(sessionsHelp)
	return nil
}

func (app *App) printProvidersHelp() error {
	fmt.Print(providersHelp)
	return nil
}

func (app *App) printToolsHelp() error {
	fmt.Print(toolsHelp)
	return nil
}

func (app *App) printSkillsHelp() error {
	fmt.Print(skillsHelp)
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func resolveProviderBaseURL(provider string, explicit string) string {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed
	}
	if strings.TrimSpace(provider) == "copilot-proxy" {
		return copilotproxy.DefaultBaseURL
	}
	return ""
}

type discardWriter struct{}

func (discardWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

const mcpHelp = `elementary-claw mcp

Subcommands:
  auth <name>           Run OAuth2 browser flow and cache token for a remote MCP server
  list-tools <name>     List tools exposed by a remote MCP server
`

const rootHelp = `elementary-claw CLI

Commands:
  setup                 Initialize user-local state and workspace
	gateway serve         Run the local runtime gateway HTTP server
  gateway status        Inspect whether config, auth and sessions exist
  bootstrap first-message
                        Generate the first bootstrap session from workspace files
	providers             Validate provider layers like github-copilot and copilot-proxy
  sessions list         List persisted sessions
  skills list           List loaded skills
  skills info <name>    Show details for a specific skill
  skills install <src>  Install a skill from path or URL
  skills remove <name>  Remove a managed skill
  skills check          Verify skill requirements
`

const gatewayHelp = `elementary-claw gateway

Subcommands:
	serve                 Run the local runtime gateway server
  status                Show current local runtime readiness
`

const bootstrapHelp = `elementary-claw bootstrap

Subcommands:
  first-message         Persist the first assistant exchange for first login resume
	ensure                Create the bootstrap session once when BOOTSTRAP.md exists
`

const sessionsHelp = `elementary-claw sessions

Subcommands:
  list                  List persisted sessions
`

const providersHelp = `elementary-claw providers

Subcommands:
	github-copilot exchange
												Exchange a GitHub token for a native Copilot API token
	copilot-proxy probe   Probe an OpenAI-compatible Copilot proxy /v1 endpoint
`

const toolsHelp = `elementary-claw tools

Subcommands:
  list                  List all registered tools
  inspect <name>        Show details and parameters for a specific tool
`

const skillsHelp = `elementary-claw skills

Subcommands:
  list [--json]         List all loaded skills
  info <name>           Show details and requirements for a specific skill
  check                 Verify that all skill requirements are satisfied
  install <path|url>    Install a skill from a local path or URL
  remove <name>         Remove a managed skill
`
