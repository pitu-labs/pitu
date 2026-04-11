# Pitú — Operator Guide

Pitú is a local-first agent harness. Messages arrive from a **frontend** (default: Telegram), agents run inside rootless Podman containers (OpenCode + pitu-mcp), and all harness ↔ agent communication is filesystem-based IPC. New frontends and capabilities can be added as skills or new packages without touching the core IPC layer.

For architecture details see @docs/ARCHITECTURE.md. For the security model see @docs/SECURITY.md.

---

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
cmd/pitu/        main harness binary (wires all packages together)
cmd/pitu-mcp/    MCP server (runs inside containers; writes IPC files)
internal/        all library packages (one responsibility each)
  config/        config loader and permission check
  container/     container lifecycle — start, stop, TTL, sub-agent spawn
  ipc/           filesystem IPC: watcher, router, types
  queue/         per-chat FIFO dispatch with global concurrency cap
  ratelimit/     per-chat fixed-interval rate limiter
  scheduler/     cron-style task runner
  skills/        skill discovery, merging, and agent context injection
  store/         SQLite persistence (messages, tasks, groups, sessions)
  telegram/      Telegram frontend adapter (Poller + Sender)
container/       Containerfile for the agent container image
.agents/skills/  bundled operator skills (AgentSkills-compatible)
docs/            architecture and security documentation
```

## Core Features

| Feature | IPC subdir | Description |
|---------|-----------|-------------|
| Send message | `messages/` | Agent writes text; harness delivers to frontend |
| Scheduled tasks | `tasks/` | Agent creates/pauses cron jobs; harness runs them |
| Group registration | `groups/` | Agent registers a named group identity |
| Sub-agent spawn | `agents/` | Agent requests a child agent with a role and prompt |
| Emoji reactions | `reactions/` | Agent sets a reaction on a specific message |

See @docs/ARCHITECTURE.md for the full IPC type definitions and data layout.

## Communication Channels

Telegram is the **default** frontend. The harness is channel-agnostic above the `internal/telegram/` adapter. To implement a new channel:

- **As a Go package:** add `internal/<channel>/` with a `Poller` (receives messages) and `Sender` (delivers responses), then wire them in `cmd/pitu/main.go` alongside or instead of the Telegram adapter.
- **As an operator skill:** write a companion process that bridges an external platform into the IPC layer by writing `InboundMessage` JSON files to `~/.pitu/data/<chatID>/ipc/input/`.

The IPC, queue, container, and scheduler layers require no changes in either case.

## Skills

Bundled operator skills live in `.agents/skills/`. User skills go in `~/.agents/skills/` or `~/.pitu/skills/`. Skills follow the [AgentSkills specification](https://agentskills.io/specification).

Add a new bundled skill by creating `.agents/skills/<name>/SKILL.md`.

## Agent Personalization

Optional files in `~/.pitu/agent/` are injected into each agent's `CONTEXT.md` at the start of a new chat:

| File | Purpose |
|------|---------|
| `IDENTITY.md` | Agent name and role |
| `SOUL.md` | Personality, tone, and hard limits |
| `USER.md` | Operator profile (name, timezone, preferences) |

## Collaboration Policy

**This repository accepts:**
- Security fixes
- Bug fixes
- New skill proposals (as `.agents/skills/<name>/SKILL.md`)

**Features are added by users** using their own coding agents (OpenCode, Gemini CLI, etc.) acting on their own forks. Do not open feature PRs upstream.

## Key Design Constraints

- Core target: ~4,000 lines, ~15 files — keep it auditable
- IPC is filesystem-only — agents never call external APIs directly
- `PITU_CHAT_ID` is injected by the harness — agents cannot route to arbitrary chats
- `CONTEXT.md` in each chat's memory dir is created once and preserved
