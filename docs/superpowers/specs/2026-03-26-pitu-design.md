# Pitu — Design Specification

**Date:** 2026-03-26
**Status:** Approved

---

## Overview

Pitu is a local-first agent harness for running AI agents that respond to messaging platforms. It is architecturally inspired by NanoClaw but built entirely on open-source components, implemented in Go, and designed to be small enough that a coding agent (the operator's OpenCode instance) can read, understand, and safely modify the entire codebase.

The core design philosophy:

- **Lean, auditable core** — target ~4,000 lines across ~15 files, modelled after NanoClaw's approach
- **Skills-first extensibility** — new features are added by users via skills and OpenCode, not via pull requests
- **Open standards** — skills follow the [AgentSkills](https://agentskills.io/) specification; project context follows [AGENTS.md](https://agents.md/)
- **Hard sandbox boundary** — the only way for an agent to affect the outside world is through structured IPC files consumed by the host process
- **Collaboration model** — the repository accepts security/bug fixes and new skill proposals only; feature development is the user's responsibility via their own coding agent

---

## Technology Stack

| Dimension | Choice | Rationale |
|---|---|---|
| Language | Go | Single compiled binary, fast startup, strong concurrency primitives, easy deployment |
| Container runtime | Podman (rootless) | No daemon, no root requirement, systemd-native, identical behavior on home server and cloud VPS |
| Agent runtime | OpenCode | Open-source, provider-agnostic, AgentSkills-compatible, client/server architecture |
| Messaging | Telegram (primary) | Simple bot API, no phone number required, easy self-hosting |
| Storage | SQLite (`modernc.org/sqlite`) | Pure-Go, no CGO, single binary, sufficient for single-node deployment |
| IPC | Filesystem (fsnotify) | Simple, auditable, no network surface, directly follows NanoClaw's model |
| Config | TOML | Human-readable, well-supported in Go |

---

## Architecture

### Data Flow

```
Telegram Bot API
      │  (long-poll)
      ▼
┌─────────────┐
│   Poller    │──── stores ────► SQLite
└─────────────┘                  (messages, sessions, tasks)
      │
      ▼
┌─────────────┐
│  Chat Queue │  (per-chat FIFO, global concurrency cap)
└─────────────┘
      │
      ▼
┌──────────────────────────────────┐
│       Container Manager          │
│  warm pool map: chatID → state   │
│  - container exists & warm?      │
│    → write to /workspace/ipc/    │
│  - container cold/absent?        │
│    → podman run, then write      │
│  - TTL timer per container       │
└──────────────────────────────────┘
      │
      ▼
┌──────────────────────────────────┐
│  Podman Container (per chat)     │
│  ┌───────────────────────────┐   │
│  │  OpenCode (subprocess)    │   │
│  │  └── pitu-mcp (stdio MCP) │   │
│  └───────────────────────────┘   │
│  /workspace/ipc/    (RW)         │
│  /workspace/memory/ (RW, persisted across runs) │
│  /workspace/skills/ (RO, shared) │
└──────────────────────────────────┘
      │  (fsnotify on ipc/)
      ▼
┌──────────────┐
│  IPC Watcher │──► Telegram Sender / Scheduler / Store
└──────────────┘
```

### Goroutine Map

```
main
 ├── telegram.Poller        long-polls Telegram, pushes to store + queue
 ├── queue.Dispatcher       one goroutine per active chat, bounded by semaphore
 ├── ipc.Watcher            fsnotify loop → routes IPC files to appropriate handler
 ├── container.TTLReaper    ticks every 30s, stops expired containers
 └── scheduler.Runner       fires cron/interval tasks
```

---

## Container Lifecycle — Warm Pool with TTL

### State Machine (per chat)

```
COLD ──(message arrives)──► STARTING ──(container ready)──► WARM
                                                              │
                             ◄──────(TTL expires, no msgs)───┘
                           STOPPING ──► COLD
```

A `sync.Map` holds `chatID → ContainerHandle`:

```go
type ContainerHandle struct {
    ID          string        // Podman container ID
    InputDir    string        // /workspace/ipc/input/ host path
    OutputDir   string        // /workspace/ipc/ host path
    LastActivity time.Time
    TTLTimer    *time.Timer
}
```

**On incoming message:**
1. Look up `chatID` in warm pool
2. Hit → reset TTL timer, write input file
3. Miss → `podman run` with mounts, register handle, write input file

**Podman is called via `exec.Command("podman", ...)`** — no Go bindings. This keeps the orchestration code readable and the binary small.

### Container Mounts

| Mount | Mode | Purpose |
|---|---|---|
| `/workspace/ipc/` | RW | All agent↔host communication |
| `/workspace/memory/` | RW | Persistent per-chat state: AGENTS.md, memory files |
| `/workspace/skills/` | RO | Merged view of all discovered skills |

---

## IPC Design

### Workspace Layout (inside container)

```
/workspace/
├── ipc/
│   ├── input/        # host writes inbound messages here
│   ├── messages/     # agent writes outbound Telegram messages here
│   ├── tasks/        # agent writes schedule/pause/list task requests here
│   └── groups/       # agent writes registerGroup calls here (main agent only)
├── memory/
└── skills/
```

### Inbound Message Format (host → container)

```json
{
  "chat_id": "123456",
  "from": "alice",
  "text": "Hello, agent",
  "message_id": "789"
}
```

File name: `{unix_timestamp_ns}-{message_id}.json`

### Outbound Message Format (container → host)

```json
{
  "chat_id": "123456",
  "text": "Hello back",
  "type": "message"
}
```

File name: `{unix_timestamp_ns}-{seq}.json`

The `chat_id` is the only routing key. No session tokens, no agent identity headers. The host IPC watcher reads the file, routes to the appropriate Telegram chat, and deletes the file.

### Agent Swarms

Swarm coordination is fully delegated to the agent SDK (OpenCode + LLM). When an agent spawns a sub-agent, it uses the `TeamCreate`/`SendMessage` SDK tools. The host treats all messages uniformly — there is no swarm-specific code in the harness.

---

## Tool Architecture

### Layer 1 — Built-in SDK Tools

Registered in the OpenCode config the harness generates per container. Always available to all agents:

```
Bash, Read, Write, Edit, Glob, Grep
WebSearch, WebFetch
Task, TaskOutput, TaskStop
TeamCreate, TeamDelete, SendMessage
TodoWrite, ToolSearch, Skill, NotebookEdit
mcp__pitu__*
```

### Layer 2 — Pitu MCP Server (`cmd/pitu-mcp/`)

A small Go binary built alongside the main harness and bundled into the container image. Registered as a stdio MCP server in the OpenCode config that the harness generates at container start — not user-facing config.

**Exposed tools:**

| Tool | Description |
|---|---|
| `mcp__pitu__sendMessage(text, sender)` | Send a message to the originating chat mid-run |
| `mcp__pitu__scheduleTask(...)` | Create a recurring or one-shot scheduled task |
| `mcp__pitu__listTasks()` | List current scheduled tasks |
| `mcp__pitu__pauseTask(id)` | Pause a scheduled task |
| `mcp__pitu__registerGroup(...)` | Register a new chat group (main agent only) |

**Design rule:** MCP tools write JSON files to `/workspace/ipc/` subdirectories. They perform no side effects themselves. The host process is the only actor with real side effects (Telegram delivery, SQLite writes, scheduler updates).

**Security property:** The MCP server is a stdio child process of OpenCode. It has no network surface and cannot be reached by any other process in the container.

### Layer 3 — Skills (`/workspace/skills/`)

Skills follow the [AgentSkills specification](https://agentskills.io/specification). They are not MCP servers and cannot register new MCP tools.

| Kind | How invoked |
|---|---|
| Bash scripts/executables | `Bash` tool or direct execution |
| Global npm packages (e.g. `agent-browser`) | Installed in `Containerfile`, run via `Bash` |
| Documentation-only | `Skill` tool reads `SKILL.md` instructions |

Skills may declare `allowed-tools` in `SKILL.md` frontmatter to pre-approve specific Bash sub-commands. The MCP surface is fixed and not extensible by skills.

---

## Skills System

### Discovery

At startup, the harness scans these directories (project-level overrides user-level on name collision):

```
~/.agents/skills/          # cross-client user skills
~/.pitu/skills/            # pitu-native user skills
./.agents/skills/          # project-level cross-client
./.pitu/skills/            # project-level pitu-native
```

The `/workspace/skills/` mount is a merged read-only view of all discovered skills.

### Catalog Injection

At first container boot, the harness generates and writes an `AGENTS.md` to `/workspace/memory/`. This file contains:

1. **Skills catalog** — name + description per skill (~50–100 tokens each), with path to `SKILL.md`
2. **Behavioral instruction** — activate skills by reading the listed `SKILL.md` (OpenCode is AgentSkills-compatible natively)
3. **Chat context** — `chat_id`, platform name, summary of any prior memory

### Bundled Operator Skills

Shipped in `.pitu/skills/`, these cover operator workflows. Users invoke them through OpenCode acting on the Pitu project directory:

| Skill | Purpose |
|---|---|
| `configure-telegram` | Set bot token, webhook vs. polling mode |
| `set-container-ttl` | Tune warm-pool TTL |
| `add-memory-backend` | Swap SQLite for another store |
| `view-active-sessions` | Inspect warm pool state |
| `update-pitu` | Guided self-update via OpenCode |

### OpenCode as Operator Surface

The operator runs OpenCode in the Pitu project directory. The root `AGENTS.md` provides:

- Build command: `go build ./cmd/pitu && go build ./cmd/pitu-mcp`
- Config file location and schema
- Skills conventions and discovery paths
- Collaboration constraint: security/bug fixes and skill proposals only — features are added by users via skills, not upstream PRs

Any AgentSkills-compatible tool (Gemini CLI, Cursor, etc.) can serve as the operator surface.

---

## Project Structure

```
pitu/
├── cmd/
│   ├── pitu/                  # main harness binary
│   │   └── main.go
│   └── pitu-mcp/              # MCP server binary (bundled in container image)
│       └── main.go
├── internal/
│   ├── config/                # TOML config loading + validation
│   ├── store/                 # SQLite: messages, sessions, tasks
│   ├── telegram/              # long-poll + sender (raw HTTP)
│   ├── queue/                 # per-chat FIFO, global concurrency cap
│   ├── container/             # podman exec wrapper, warm-pool, OpenCode config generator
│   ├── ipc/                   # fsnotify watcher, inbound writer, outbound router
│   ├── skills/                # AgentSkills discovery, catalog builder, AGENTS.md generator
│   └── scheduler/             # cron/interval task runner
├── container/
│   └── Containerfile          # installs opencode, pitu-mcp binary, global npm packages
├── .pitu/skills/              # bundled operator skills
│   ├── configure-telegram/
│   │   └── SKILL.md
│   ├── set-container-ttl/
│   │   └── SKILL.md
│   ├── add-memory-backend/
│   │   └── SKILL.md
│   ├── view-active-sessions/
│   │   └── SKILL.md
│   └── update-pitu/
│       └── SKILL.md
├── AGENTS.md                  # operator guide for OpenCode
├── config.example.toml
└── go.mod
```

### Go Dependencies

| Package | Purpose |
|---|---|
| `modernc.org/sqlite` | Pure-Go SQLite, no CGO |
| `github.com/fsnotify/fsnotify` | Filesystem events for IPC watching |
| `github.com/BurntSushi/toml` | Config file parsing |
| `gopkg.in/yaml.v3` | SKILL.md YAML frontmatter parsing |

### Configuration (`~/.pitu/config.toml`)

```toml
[telegram]
bot_token = "..."

[container]
image          = "ghcr.io/youruser/pitu-agent:latest"
ttl            = "5m"
max_concurrent = 5
memory_limit   = "512m"

[skills]
extra_paths = []   # additional skill directories beyond defaults

[db]
path = "~/.pitu/pitu.db"
```

---

## Security Properties

1. **No daemon, no root** — Podman runs rootless; the harness binary needs no elevated privileges
2. **Fixed MCP surface** — the only agent→host channel is the Pitu MCP server; it writes files only, performs no side effects
3. **Read-only skills mount** — agents can read skills but not modify them
4. **No network from containers** — containers have no direct network access to Telegram or any external service; all outbound calls go through the host
5. **Auditable core** — ~4,000 lines, ~15 files; the entire codebase is readable in one sitting
6. **Deterministic IPC routing** — `chat_id` is the only routing key; no ambient authority

---

## Non-Goals

- Additional messaging channels at launch (Telegram only; others added via skills/OpenCode)
- Kubernetes or multi-node orchestration (single-node only; cloud adaptation is operator responsibility)
- Web UI or REST API surface (operator management is via OpenCode + AGENTS.md)
- Feature PRs (collaboration limited to security/bug fixes and skill proposals)
