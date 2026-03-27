# Pitu — Operator Guide

Pitu is a local-first Telegram agent harness. Agents run inside rootless Podman containers (OpenCode + pitu-mcp). This file is your guide to working with the codebase.

## Build Commands

```bash
go build ./cmd/pitu       # main harness binary
go build ./cmd/pitu-mcp   # MCP server (bundled into container image)
go test ./...             # run all tests
podman build -t pitu-agent:latest -f container/Containerfile .
```

## Configuration

Config lives at `~/.pitu/config.toml`. Copy `config.example.toml` to get started.

## Project Layout

```
cmd/pitu/        main harness binary
cmd/pitu-mcp/    MCP server (runs inside containers)
internal/        all library packages (one responsibility each)
container/       Containerfile for the agent container image
.agents/skills/  bundled operator skills (AgentSkills-compatible)
```

## Collaboration Policy

**This repository accepts:**
- Security fixes
- Bug fixes
- New skill proposals (as `.agents/skills/<name>/SKILL.md`)

**Features are added by users** using their own coding agents (OpenCode, Gemini CLI, etc.) acting on their own forks. Do not open feature PRs upstream.

## Skills

User skills go in `~/.agents/skills/` or `~/.pitu/skills/`. Skills follow the [AgentSkills specification](https://agentskills.io/specification). See `.agents/skills/` for examples.

## Key Design Constraints

- Core target: ~4,000 lines, ~15 files — keep it auditable
- IPC is filesystem-only — agents never call Telegram directly
- `PITU_CHAT_ID` is injected by the harness — agents cannot route to arbitrary chats
- `CONTEXT.md` in each chat's memory dir is created once and preserved
