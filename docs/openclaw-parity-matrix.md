# OpenClaw Parity Matrix For elementary-claw

## Goal

This document maps the OpenClaw capability surface into three buckets:

- port now
- port later
- do not copy 1:1

The objective is not to fork OpenClaw blindly. The objective is to replicate the parts that matter for an install-first native elementary OS product.

## Port Now

### Gateway and control plane

- Why: this is the real runtime heart of the system
- OpenClaw references:
  - `openclaw/src/gateway/server.ts`
  - `openclaw/src/gateway/auth.ts`
  - `openclaw/docs/gateway/protocol.md`
- elementary-claw target:
  - Go service with local API and session state

### Auth profiles and providers

- Why: the installer flow already depends on GitHub Copilot device login
- OpenClaw references:
  - `openclaw/extensions/github-copilot/login.ts`
  - `openclaw/src/agents/auth-profiles/`
  - `openclaw/src/commands/auth-choice.ts`
- elementary-claw target:
  - provider catalog
  - per-user auth state
  - runtime token exchange hooks

### Sessions and transcript persistence

- Why: the first-login experience needs to resume a setup-created bootstrap session
- OpenClaw references:
  - `openclaw/src/config/sessions/`
  - `openclaw/src/sessions/`
- elementary-claw target:
  - JSON session store in `~/.openclaw/state/sessions`
  - bootstrap session created during setup

### Tools runtime

- Why: the assistant is not useful without local capabilities
- OpenClaw references:
  - `openclaw/src/agents/tools/`
  - `openclaw/src/agents/bash-tools.ts`
  - `openclaw/src/infra/exec-approvals-policy.ts`
- elementary-claw target:
  - initial tools: exec, read_file, list_dir, apply_patch equivalent, notifications

### Skills loader

- Why: local customization is part of the product identity
- OpenClaw references:
  - `openclaw/src/agents/skills/`
  - `openclaw/skills/`
- elementary-claw target:
  - workspace and user-level skills loading

### Bootstrap and onboarding handoff

- Why: this is the main differentiator for the project
- OpenClaw references:
  - `openclaw/src/wizard/setup.finalize.ts`
  - `openclaw/src/cli/program/register.setup.ts`
- elementary-claw target:
  - setup-created agent
  - setup-created bootstrap session
  - runtime resume on first login

## Port Later

### MCP connectors

- OpenClaw references:
  - `openclaw/src/mcp/`
  - `openclaw/src/config/types.mcp.ts`
- Later because:
  - not required to prove the initial account-creation flow

### Media and attachment pipeline

- OpenClaw references:
  - `openclaw/src/media/`
- Later because:
  - useful, but not blocking the install-first AI setup flow

### Channel integrations

- OpenClaw references:
  - `openclaw/src/channels/`
  - `openclaw/docs/channels/`
  - `openclaw/extensions/`
- Later because:
  - elementary-claw v1 is local-first, not multi-channel-first

### Advanced diagnostics and doctor flows

- OpenClaw references:
  - `openclaw/src/commands/doctor*`
  - `openclaw/src/cli/program/register.status-health-sessions.ts`
- Later because:
  - minimal health/status is enough at first

## Do Not Copy 1:1

### Browser-first runtime assumptions

- OpenClaw has a broad web and multi-device shape
- elementary-claw must stay install-first and local-runtime-first

### All bundled plugins and channels

- OpenClaw supports many external channels
- elementary-claw should only add them when the local core is already stable

### Node.js as the runtime center

- OpenClaw uses TypeScript/Node deeply
- elementary-claw will use Go as the core runtime language

### Platform clients unrelated to elementary OS

- OpenClaw includes macOS, iOS and Android clients
- elementary-claw should focus on native elementary OS surfaces first

## Immediate Porting Order

1. Go CLI and config/state model
2. bootstrap session generation
3. gateway service skeleton
4. auth/provider layer for GitHub Copilot
5. tools runtime
6. skills loader
7. native app connection to gateway
8. MCP and extra integrations