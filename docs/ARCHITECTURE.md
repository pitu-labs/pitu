# Pitu — Architecture Reference

This document describes how Pitu is built, why it is built that way, and what every component does. It is intended for two audiences: **humans** who want a mental model of the system before reading code, and **AI agents** implementing new features that need to understand the invariants they must not break.

---

## Table of Contents

1. [System Overview](#1-system-overview)
2. [Project Layout](#2-project-layout)
3. [High-Level Data Flow](#3-high-level-data-flow)
4. [Component Reference](#4-component-reference)
   - 4.1 [Entry Point — `cmd/pitu`](#41-entry-point--cmdpitu)
   - 4.2 [Telegram Layer — `internal/telegram`](#42-telegram-layer--internaltelegram)
   - 4.3 [Rate Limiter — `internal/ratelimit`](#43-rate-limiter--internalratelimit)
   - 4.4 [Dispatch Queue — `internal/queue`](#44-dispatch-queue--internalqueue)
   - 4.5 [Container Manager — `internal/container`](#45-container-manager--internalcontainer)
   - 4.6 [IPC System — `internal/ipc`](#46-ipc-system--internalipc)
   - 4.7 [MCP Server — `cmd/pitu-mcp`](#47-mcp-server--cmdpitu-mcp)
   - 4.8 [Skills System — `internal/skills`](#48-skills-system--internalskilss)
   - 4.9 [Scheduler — `internal/scheduler`](#49-scheduler--internalscheduler)
   - 4.10 [Store — `internal/store`](#410-store--internalstore)
   - 4.11 [Config — `internal/config`](#411-config--internalconfig)
5. [Key Architectural Decisions](#5-key-architectural-decisions)
   - 5.1 [IPC Over Filesystem, Not HTTP](#51-ipc-over-filesystem-not-http)
   - 5.2 [Path-Derived Identity](#52-path-derived-identity)
   - 5.3 [Atomic File Writes via Rename](#53-atomic-file-writes-via-rename)
   - 5.4 [Warm Container Pool with Cold Exec](#54-warm-container-pool-with-cold-exec)
   - 5.5 [Double-Checked Locking on Container Start](#55-double-checked-locking-on-container-start)
   - 5.6 [AGENTS.md vs CONTEXT.md — Identity vs Memory](#56-agentsmd-vs-contextmd--identity-vs-memory)
   - 5.7 [Skills Merge at Startup](#57-skills-merge-at-startup)
   - 5.8 [Task Snapshot Pattern](#58-task-snapshot-pattern)
6. [Message Lifecycle Walkthrough](#6-message-lifecycle-walkthrough)
   - 6.1 [Inbound: User → Agent](#61-inbound-user--agent)
   - 6.2 [Outbound: Agent → Telegram](#62-outbound-agent--telegram)
   - 6.3 [Sub-Agent Spawn and Bubble-Up](#63-sub-agent-spawn-and-bubble-up)
   - 6.4 [Scheduled Task Firing](#64-scheduled-task-firing)
7. [Container Filesystem Layout](#7-container-filesystem-layout)
8. [Host Filesystem Layout](#8-host-filesystem-layout)
9. [IPC Protocol Reference](#9-ipc-protocol-reference)
10. [Skills Specification](#10-skills-specification)
11. [Concurrency Model](#11-concurrency-model)
12. [Extending Pitu](#12-extending-pitu)

---

## 1. System Overview

Pitu is a **local-first agent kernel** that bridges Telegram and AI agents running inside rootless Podman containers. Its job is to:

- Accept messages from Telegram users.
- Dispatch those messages to stateful AI agent containers.
- Relay the agent's replies back to Telegram.
- Expose a sandboxed MCP interface that lets agents schedule tasks, spawn sub-agents, and react to messages — all without touching the network or the host directly.

```
┌──────────────────────────────────────────────────────────────────┐
│                        HOST (operator machine)                    │
│                                                                   │
│  ┌──────────┐    ┌─────────────────────────────────────────────┐  │
│  │ Telegram │    │                 pitu harness                 │  │
│  │   API    │◄──►│  Poller → Queue → Container Manager → IPC   │  │
│  └──────────┘    │                    ↕ filesystem              │  │
│                  └─────────────────────────────────────────────┘  │
│                                        ↕ volume mounts            │
│  ┌─────────────────────────────────────────────────────────────┐  │
│  │              Podman container  (rootless)                    │  │
│  │                                                             │  │
│  │   OpenCode ──MCP──► pitu-mcp ──writes──► /workspace/ipc/   │  │
│  │       ↑                                                     │  │
│  │  reads input file from /workspace/ipc/input/               │  │
│  └─────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

**Key constraint:** Agents never call Telegram directly, never open sockets, and never write outside their mounted directories. All agent→harness communication is filesystem-only.

For the security model and threat analysis, see [`docs/SECURITY.md`](SECURITY.md).

---

## 2. Project Layout

```
pitu/
├── cmd/
│   ├── pitu/               # Main harness binary
│   │   ├── main.go         # Wiring: poller, queue, manager, router, scheduler
│   │   ├── service.go      # `pitu service` subcommand (install/start/stop)
│   │   ├── snapshot.go     # writeTasksSnapshot — atomic tasks.json writer
│   │   └── allowlist_test.go
│   └── pitu-mcp/           # MCP server — runs INSIDE containers
│       ├── main.go         # Reads PITU_CHAT_ID; starts stdio MCP server
│       ├── server.go       # MCP tool registration (sendMessage, scheduleTask, …)
│       └── tools.go        # Tool handlers — write IPC files via atomic rename
│
├── internal/
│   ├── config/             # TOML config loading and permission checking
│   ├── container/          # Container lifecycle, warm pool, OpenCode exec
│   │   ├── manager.go      # Manager: ensureContainer, dispatch, SpawnSubAgent
│   │   ├── inputwriter.go  # WriteInputFile — harness→container message delivery
│   │   ├── opencodecfg.go  # GenerateOpenCodeConfig, APIKeyEnvVar
│   │   └── subagent_test.go
│   ├── ipc/                # IPC types, filesystem watcher, router
│   │   ├── types.go        # InboundMessage, OutboundMessage, TaskFile, …
│   │   ├── watcher.go      # fsnotify-based dir watcher; calls RegisterDir
│   │   └── router.go       # Route: stat → read → unmarshal → chatID override → dispatch
│   ├── queue/              # Per-chat FIFO with global concurrency cap
│   ├── ratelimit/          # Per-chat fixed-interval limiter
│   ├── scheduler/          # Cron-backed task scheduler (robfig/cron)
│   ├── skills/             # Skill discovery, catalog builder, context writer
│   │   ├── discovery.go    # Discover — scans dirs, parses YAML frontmatter
│   │   ├── catalog.go      # BuildCatalog — XML skill list for AGENTS.md
│   │   ├── context.go      # WriteContext — writes AGENTS.md + CONTEXT.md
│   │   ├── agent.go        # LoadAgentConfig — reads SOUL.md, IDENTITY.md, USER.md
│   │   └── types.go        # Skill struct
│   ├── store/              # SQLite persistence (messages, sessions, tasks, groups)
│   │   ├── store.go        # New, migrate — schema bootstrap
│   │   ├── tasks.go        # SaveTask, PauseTask, GetTasksByChatID, …
│   │   ├── messages.go
│   │   ├── sessions.go
│   │   └── groups.go
│   ├── telegram/           # Telegram HTTP client
│   │   ├── poller.go       # Long-poll getUpdates with 429/5xx backoff
│   │   ├── sender.go       # SendMessage, SendChatAction, ReactToMessage
│   │   └── types.go        # Update, Message, From structs
│   └── service/            # Systemd / launchd service installer
│
├── container/
│   └── Containerfile       # Two-stage build: Go builder + Debian runtime
│
├── .agents/skills/         # Bundled operator skills (AgentSkills-compatible)
│   ├── setup/
│   ├── configure-telegram/
│   ├── set-container-ttl/
│   ├── view-active-sessions/
│   ├── add-memory-backend/
│   ├── update-pitu/
│   └── update-model/
│
├── config.example.toml     # Annotated config template
└── docs/
    ├── SECURITY.md
    └── ARCHITECTURE.md     ← this file
```

---

## 3. High-Level Data Flow

### Inbound (Telegram → Agent)

```
Telegram long-poll
      │
      ▼
  isAllowed?  ──No──► drop + log
      │
   limiter.Allow?  ──No──► drop + log
      │
  store.SaveMessage
      │
  skills.WriteContext  (AGENTS.md refreshed; CONTEXT.md created once)
      │
  queue.Enqueue(chatID, fn)
      │
  [worker goroutine acquires global semaphore]
      │
  mgr.Dispatch(chatID, msg)
      │
  container.ensureContainer  ──(warm hit)──► skip start
      │                      ──(cold miss)──► podman run --detach
      │
  WriteInputFile  →  ipcDir/input/{ts}-{msgID}.json
      │
  podman exec opencode run -f /workspace/ipc/input/…
```

### Outbound (Agent → Telegram)

```
OpenCode calls mcp__pitu__sendMessage(text)
      │
  pitu-mcp writes JSON to tmp file, renames into ipcDir/messages/
      │
  fsnotify Create event  →  Watcher.Watch
      │
  router.Route("messages", path, chatID, role, subAgentID)
      │   [chatID field in JSON overwritten with path-derived value]
      │
  onMessage callback
      │
  if subAgentID != ""  →  bubble up: re-enqueue as InboundMessage to parent
  else                 →  sender.SendMessage(chatID, text)
```

---

## 4. Component Reference

### 4.1 Entry Point — `cmd/pitu`

**`main.go`** is the wiring layer. It:

1. Loads config and checks file permissions.
2. Constructs all components in dependency order.
3. Starts three background goroutines: `w.Watch(ctx)`, `sched.Run(ctx)`, and the Telegram poll loop.
4. Installs a `SIGINT`/`SIGTERM` handler that cancels the context, calls `mgr.StopAll()`, and drains the queue.

Because several closures (`onMessage`, `onAgent`, etc.) reference `q` and `mgr` which are declared just before them, Go's forward declaration order matters here. The variables are declared with `var` before the closures that capture them, and assigned after.

**`snapshot.go`** — `writeTasksSnapshot` reads all tasks for a chat from SQLite and atomically writes them to `<dataDir>/<chatID>/memory/tasks.json` using a temp-file + rename pattern. This file is visible to the agent at `/workspace/memory/tasks.json` and is refreshed on every task create or pause event.

**`mergeSkills`** copies all discovered skill directories into a single `<dataDir>/skills/` directory so containers see a unified merged view via one volume mount. Higher-precedence skills (index 0 in the `discovered` slice) are copied last and overwrite lower-precedence copies with the same name.

---

### 4.2 Telegram Layer — `internal/telegram`

**`poller.go`** implements Telegram's long-poll API (`getUpdates?timeout=30`). Key behaviours:

- Checks `resp.StatusCode` **before** decoding. On 429, reads `Retry-After` header and sleeps exactly that duration. On 5xx, sleeps 2 s. On other non-200, sleeps 2 s. This prevents garbage responses from entering the dispatch pipeline.
- Tracks an `offset` counter to acknowledge processed updates and avoid re-delivery.
- Returns a typed `retryError` for 429 so the caller can distinguish rate-limit backoff from transient errors.

**`sender.go`** wraps `sendMessage`, `sendChatAction`, and `setMessageReaction` as simple POST calls.

---

### 4.3 Rate Limiter — `internal/ratelimit`

A per-chat fixed-interval limiter. `Allow(chatID)` returns `true` only if at least `interval` has elapsed since the last accepted message from that chat. A zero interval disables limiting (all messages pass). The `last` map is protected by a single mutex.

This is the **second gate** after the allowlist check — it prevents any single user from monopolising the global concurrency pool.

---

### 4.4 Dispatch Queue — `internal/queue`

```
Queue
 ├── sem: chan struct{}  (capacity = maxConcurrent)
 └── chats: map[chatID] → chan func()  (capacity 256)
```

Each chat gets its own buffered channel and a dedicated goroutine (`worker`). Workers block on the global semaphore before calling their function, ensuring that at most `maxConcurrent` containers run in parallel across all chats. Within a single chat, messages are processed strictly in order.

`Stop()` closes `done`, then closes all per-chat channels, then waits for all workers via `wg.Wait()`. Using `sync.Once` makes `Stop` idempotent.

---

### 4.5 Container Manager — `internal/container`

The manager is the largest and most important component.

#### Warm Pool

Containers are kept alive with `sleep infinity` after their first message. The pool is a `sync.Map` keyed by `chatID`. Each entry is a `*Handle` carrying the Podman container ID, the IPC directory path, session state, and a TTL timer.

```
Handle
 ├── ID           string       # Podman container ID
 ├── IPCDir       string       # host path to ipc root for this chat
 ├── hasSession   bool         # whether -c (continue session) should be passed to OpenCode
 ├── lastActivity time.Time
 └── ttlTimer     *time.Timer  # fires stopContainer after TTL
```

#### Double-Checked Locking

`ensureContainer` uses a classic double-checked locking pattern:

```go
// Fast path — no lock
if v, ok := pool.Load(chatID); ok { return v }

// Slow path — serialised
startMu.Lock()
defer startMu.Unlock()
if v, ok := pool.Load(chatID); ok { return v }  // re-check
return startContainer(ctx, chatID)
```

This prevents two concurrent messages for the same chat from starting two containers for the same slot. The fast path is lock-free thanks to `sync.Map`.

#### Container Startup

`startContainer` creates three host directories (`ipc/`, `memory/`, `opencode/`) under `~/.pitu/data/<chatID>/`, writes a temporary env file (mode `0600`), then calls:

```
podman run --detach --rm
  --memory <limit>
  --env PITU_CHAT_ID=<chatID>
  --env-file <tempfile>        # deleted immediately after run returns
  --volume <ipcDir>:/workspace/ipc:z
  --volume <memDir>:/workspace/memory:z
  --volume <skillsDir>:/workspace/skills:ro,z
  --volume <opencodeDir>:/root/.local/share/opencode:z
  <image>
  sleep infinity
```

The env file carries `OPENCODE_CONFIG_CONTENT` (the JSON config telling OpenCode to use pitu-mcp) and the provider-specific API key (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, etc.). Keeping the key separate from the config JSON means it does not appear in `podman inspect` output.

After start, the container's IPC directory is registered with the watcher (`w.RegisterDir`).

#### Message Delivery

`execOpenCode` calls:

```
podman exec
  --workdir /workspace/memory
  <containerID>
  opencode run [-c] -f /workspace/ipc/input/<filename>
  -- "Process the inbound message from the input file."
```

`--workdir /workspace/memory` places OpenCode in the memory directory so it discovers `AGENTS.md` via its standard upward-traversal logic. The `-c` flag (continue session) is passed on every call except the very first, preserving conversation history inside OpenCode.

#### Sub-Agent Containers

`SpawnSubAgent` starts a **separate isolated container** for the sub-agent. It gets its own `ipc/`, `memory/`, and `skills/` directories under `<dataDir>/<chatID>/agents/<subAgentID>/`. A harness-written `skills/system/SKILL.md` tells the sub-agent its role, chat ID, and sub-agent ID. When the sub-agent calls `sendMessage`, the message bubbles up through the router back to the parent agent as an inbound message.

---

### 4.6 IPC System — `internal/ipc`

The IPC system is the **trust boundary** between containers and the harness.

#### Types

| Type | Direction | Written by | Subdir |
|---|---|---|---|
| `InboundMessage` | harness → container | harness | `ipc/input/` |
| `OutboundMessage` | container → harness | pitu-mcp | `ipc/messages/` |
| `TaskFile` | container → harness | pitu-mcp | `ipc/tasks/` |
| `GroupFile` | container → harness | pitu-mcp | `ipc/groups/` |
| `AgentFile` | container → harness | pitu-mcp | `ipc/agents/` |
| `ReactionFile` | container → harness | pitu-mcp | `ipc/reactions/` |

#### Watcher

`Watcher` wraps `fsnotify` and watches all five output subdirs for each registered container. When a `Create` event arrives:

1. Look up the `dirMeta` (chatID, role, subAgentID) for the event's parent directory using `sync.RWMutex`.
2. Call `router.Route(subdir, path, chatID, role, subAgentID)`.
3. Delete the file.

The `metas` map is protected by `sync.RWMutex` — readers (event dispatch) can proceed concurrently; `RegisterDir` takes a write lock.

#### Router

`router.Route` is the trust enforcement point:

1. `os.Stat` the file — reject if larger than 1 MB.
2. `os.ReadFile` the contents.
3. `json.Unmarshal` into the appropriate struct.
4. **Overwrite** the `ChatID` field with the path-derived authoritative value.
5. Call the registered handler callback.

Step 4 is the critical invariant: a container cannot route a message to a different chat by forging the `chat_id` JSON field. See [§5.2](#52-path-derived-identity).

---

### 4.7 MCP Server — `cmd/pitu-mcp`

`pitu-mcp` runs **inside every container** as a stdio MCP server. OpenCode discovers it via the `OPENCODE_CONFIG_CONTENT` environment variable which declares it as a local MCP server:

```json
{
  "mcp": {
    "pitu": {
      "type": "local",
      "command": ["/usr/local/bin/pitu-mcp"],
      "environment": { "PITU_CHAT_ID": "<chatID>" }
    }
  }
}
```

It reads its identity from environment variables set by the harness:

| Variable | Set by | Purpose |
|---|---|---|
| `PITU_CHAT_ID` | harness | Authoritative chat ID |
| `PITU_ROLE` | harness (sub-agents only) | Sub-agent role string |
| `PITU_SUB_AGENT_ID` | harness (sub-agents only) | UUID for result correlation |

#### Exposed Tools

| MCP Tool | IPC Subdir | Effect |
|---|---|---|
| `sendMessage` | `messages/` | Delivers text to Telegram (or bubbles to parent) |
| `scheduleTask` | `tasks/` | Creates a recurring/one-shot cron task |
| `pauseTask` | `tasks/` | Stops a running task |
| `listTasks` | — | Returns path to `/workspace/memory/tasks.json` |
| `registerGroup` | `groups/` | Registers a named group |
| `spawnAgent` | `agents/` | Requests a sub-agent container spawn |
| `reactToMessage` | `reactions/` | Sets an emoji reaction on a Telegram message |

#### Atomic Write

Every tool handler writes via `writeIPC`:

```
1. os.CreateTemp(ipcDir, ".tmp-")   # parent dir, not the watched subdir
2. tmp.Write(jsonData)
3. tmp.Close()
4. os.Rename(tmp, subdir/<timestamp>.json)
```

The rename is atomic on Linux and fires a single `IN_MOVED_TO` event (mapped to `fsnotify.Create`) with the file already fully written. A direct `os.WriteFile` would fire two events (create + write), causing a spurious read of a partial file.

---

### 4.8 Skills System — `internal/skills`

Skills are Markdown files with YAML frontmatter that teach the agent how to perform tasks. They follow the [AgentSkills specification](https://agentskills.io/specification).

#### Discovery

`Discover(dirs)` scans a list of directories in priority order. For each directory it looks for subdirectories containing a `SKILL.md` with valid YAML frontmatter (`name`, `description`). First occurrence of a name wins — this gives project-level skills (binary-adjacent) higher precedence than user-level skills (`~/.agents/skills/`, `~/.pitu/skills/`).

Search order at startup:
1. `~/.agents/skills/`
2. `~/.pitu/skills/`
3. `<binaryDir>/.agents/skills/`
4. `<binaryDir>/.pitu/skills/`
5. Any `extra_paths` from config

#### Catalog

`BuildCatalog(skills)` returns an XML-formatted list of skills injected into `AGENTS.md`:

```xml
<available_skills>
  <skill>
    <name>setup</name>
    <description>Set up Pitu from scratch…</description>
    <location>/workspace/skills/setup/SKILL.md</location>
  </skill>
  …
</available_skills>
```

The agent reads this catalog to know what skills are available, then reads the full `SKILL.md` at the listed path when it needs to execute one.

#### Context Files

On each inbound message, `skills.WriteContext` creates or refreshes two files in the container's memory directory:

| File | Behaviour | Purpose |
|---|---|---|
| `AGENTS.md` | Always overwritten | Agent identity, skills catalog, mandatory instructions |
| `CONTEXT.md` | Created once, never overwritten | Agent's mutable memory scratch-pad |

Separating these files means the operator can update the agent's identity (by editing `~/.pitu/agent/IDENTITY.md`) and the agent will pick it up on the next message without losing its accumulated notes in `CONTEXT.md`.

#### Agent Persona

`LoadAgentConfig` reads three optional files from `~/.pitu/agent/`:

| File | Injected into AGENTS.md section |
|---|---|
| `IDENTITY.md` | `## Identity` |
| `SOUL.md` | `## Soul` |
| `USER.md` | `## User` |

All three are optional. If present, they shape how the agent presents itself and what it knows about the operator.

---

### 4.9 Scheduler — `internal/scheduler`

The scheduler wraps `robfig/cron/v3` with a thin layer that:

- Uses a single shared `schedulerParser` configured to accept standard 5-field cron expressions plus descriptors (`@daily`, `@hourly`, `@every 5m`, etc.).
- Exposes `Validate(schedule)` for pre-flight checking before persisting to the DB.
- Stores `cron.EntryID`s in a `sync.Map` keyed by task UUID for pause/remove.
- On startup, replays all non-paused tasks from SQLite into the cron engine.

When a task fires, the scheduler calls the harness-supplied `dispatch` function which enqueues a synthetic `InboundMessage` (with `From: "scheduler"`) into the queue — exactly like a real user message.

---

### 4.10 Store — `internal/store`

SQLite (via `modernc.org/sqlite` — a pure-Go port, no CGo) with a single-file embedded schema. Tables:

| Table | Key | Purpose |
|---|---|---|
| `messages` | autoincrement | Message history per chat |
| `sessions` | `chat_id` | Session tracking (created/updated timestamps) |
| `tasks` | `task_id` UUID | Scheduled tasks with cron expressions |
| `groups` | `name` | Named groups with descriptions |

`migrate()` uses `CREATE TABLE IF NOT EXISTS` so it is safe to run on every startup against an existing database. There is intentionally no migration versioning — schema changes require adding new `IF NOT EXISTS` blocks or column additions with `ALTER TABLE`.

---

### 4.11 Config — `internal/config`

TOML config at `~/.pitu/config.toml` (overridable via `PITU_CONFIG` env var). Default values are set in `Load` before TOML parsing so that absent keys use sensible defaults.

```toml
[telegram]
bot_token        = "…"
allowed_chat_ids = []     # empty = accept all
rate_limit       = "5s"

[container]
image          = "localhost/pitu-agent:latest"
ttl            = "5m"
max_concurrent = 5
memory_limit   = "512m"

[model]
provider = "anthropic"
model    = "claude-sonnet-4-5"
api_key  = "…"
base_url = ""             # required for Ollama/custom endpoints

[skills]
extra_paths = []

[db]
path = "~/.pitu/pitu.db"
```

`CheckPermissions` warns (non-fatally) if the config file is group- or world-readable, since it contains the bot token and API key.

---

## 5. Key Architectural Decisions

### 5.1 IPC Over Filesystem, Not HTTP

**Decision:** Agents communicate with the harness exclusively by writing JSON files into mounted directories. There are no sockets, no localhost HTTP servers, no shared memory.

**Why:** Any localhost port opened by the harness is reachable by every process on the host machine, not just the target container. Filesystem IPC scoped to per-container directories, with harness-enforced ownership, provides a much tighter isolation boundary. A compromised agent cannot reach other agents' IPC directories because the container only has `/workspace/ipc` for its own chat mounted.

**Implication for implementors:** New inter-process capabilities must go through the filesystem IPC layer. Do not add socket listeners or HTTP servers to the harness.

---

### 5.2 Path-Derived Identity

**Decision:** The `ChatID` embedded in any IPC JSON payload is always overwritten with the value derived from the filesystem path before dispatch.

```go
// router.go
m.ChatID = chatID // override: never trust container-supplied chatID
```

**Why:** A container can write any JSON it likes. Without this override, a malicious or prompt-injected agent could forge a `chat_id` value pointing to a different user's conversation. The filesystem path is harness-controlled — the agent has no way to influence which directory its writes land in.

**Implication for implementors:** Every new IPC struct that carries a `ChatID` field must have that field overwritten in `router.Route` before dispatch. The current pattern shows this for all five file types.

---

### 5.3 Atomic File Writes via Rename

**Decision:** `pitu-mcp` writes IPC files using `os.CreateTemp` in the parent IPC directory followed by `os.Rename` into the watched subdirectory.

**Why:** `os.WriteFile` (or open + write) generates two inotify events: one `IN_CREATE` when the file is created empty, and one `IN_MODIFY` when data is flushed. The watcher would see the first event and attempt to read an empty or partial file. `os.Rename` generates a single `IN_MOVED_TO` (mapped to `fsnotify.Create`) with the file already complete.

**Implication for implementors:** Any new code that writes IPC files — whether in `pitu-mcp` or in the harness itself for testing — must use the temp-then-rename pattern. Direct `os.WriteFile` into a watched directory will cause spurious or partial reads.

---

### 5.4 Warm Container Pool with Cold Exec

**Decision:** Containers start once (with `sleep infinity`) and stay warm. OpenCode is invoked per-message via `podman exec`, not by restarting the container.

**Why:** Container cold-start (image pull, namespace setup, OpenCode initialisation) takes several seconds. Paying that cost on every message would make the bot feel sluggish. Keeping containers warm means subsequent messages in the same conversation execute in milliseconds of overhead.

The `-c` (continue session) flag passed to `opencode run` on the second and subsequent messages preserves conversation history inside OpenCode's session store, giving the agent memory of the conversation without the harness having to re-inject it.

**Trade-off:** Warm containers consume memory even when idle. The configurable TTL (default 5 minutes) bounds resource usage to active sessions only.

---

### 5.5 Double-Checked Locking on Container Start

**Decision:** `ensureContainer` uses a lock-free fast path (`sync.Map.Load`) and a mutex-protected slow path with a re-check.

```go
if v, ok := pool.Load(chatID); ok { return v }    // fast path
startMu.Lock()
defer startMu.Unlock()
if v, ok := pool.Load(chatID); ok { return v }    // re-check inside lock
return startContainer(ctx, chatID)
```

**Why:** Without the re-check, two concurrent messages for the same chat could both miss the fast path, both acquire the lock in turn, and each start a container — leaking one container permanently (since only the second would be stored in the pool).

**Implication for implementors:** Do not simplify this to a single mutex lock without the load-before-lock check. Under concurrent load (e.g. the user sends two messages quickly), the lock contention is real.

---

### 5.6 AGENTS.md vs CONTEXT.md — Identity vs Memory

**Decision:** `AGENTS.md` is overwritten on every message; `CONTEXT.md` is created once and never touched by the harness thereafter.

**Why:** The operator may want to update the agent's persona (`IDENTITY.md`, `SOUL.md`, `USER.md`) or the available skills list. Refreshing `AGENTS.md` on every message means these changes take effect immediately without disrupting the agent's accumulated notes. If both files were merged into one, updating the persona would wipe the agent's memory.

**Implication for implementors:** Any new "system-level" context the harness injects should go into `AGENTS.md` (refreshed each message). Anything the agent is meant to accumulate over time goes into `CONTEXT.md` (created once, agent-owned).

---

### 5.7 Skills Merge at Startup

**Decision:** All discovered skills are merged into a single `<dataDir>/skills/` directory at startup, and all containers share one read-only volume mount to this directory.

**Why:** Multiple skill sources (user-level, project-level, extra paths) need to be visible to the agent as a single flat namespace. Mounting multiple volumes for skills would require Podman to support variable-length `--volume` lists, complicate the run arguments, and create ordering ambiguity. The merge step happens once at startup; higher-precedence skills are copied last and win.

**Implication for implementors:** If a skill is updated at runtime, the harness must be restarted for the new version to be visible to new containers. Existing warm containers see the old version until they are recycled.

---

### 5.8 Task Snapshot Pattern

**Decision:** The authoritative task store is SQLite, but a JSON snapshot (`tasks.json`) is written atomically to the agent's memory directory on every create or pause event.

**Why:** Agents running inside containers cannot query SQLite directly. Exposing an RPC to query tasks would require a socket or an additional IPC round-trip. Writing a JSON snapshot to a file the agent can read at any time via the filesystem keeps the IPC protocol unidirectional (container writes, harness reads) while still giving the agent an up-to-date task list.

The snapshot uses the same temp-file + rename pattern as IPC writes to prevent the agent from reading a half-written file.

---

## 6. Message Lifecycle Walkthrough

### 6.1 Inbound: User → Agent

```
1.  Telegram sends update to poller (long-poll, timeout=30s)
2.  poller.Poll calls handler(update)
3.  isAllowed(chatID) — drop if not in allowlist
4.  limiter.Allow(chatID) — drop if within rate window
5.  sender.SendChatAction(chatID, "typing")
6.  store.SaveMessage — persist to SQLite
7.  skills.WriteContext — refresh AGENTS.md; create CONTEXT.md if absent
8.  queue.Enqueue(chatID, fn)
9.  [queue worker goroutine acquires global semaphore slot]
10. mgr.Dispatch(ctx, chatID, msg)
11.   ensureContainer(ctx, chatID)
12.     pool.Load(chatID) → hit or miss
13.     [on miss] startContainer → podman run --detach → RegisterDir
14.   WriteInputFile → ~/.pitu/data/<chatID>/ipc/input/<ts>-<msgID>.json
15.   execOpenCode → podman exec … opencode run [-c] -f /workspace/ipc/input/…
16. [OpenCode reads input file, processes with AI model, calls pitu-mcp tools]
```

### 6.2 Outbound: Agent → Telegram

```
1.  OpenCode calls MCP tool mcp__pitu__sendMessage(text, sender)
2.  pitu-mcp.handleSendMessage writes OutboundMessage JSON
3.  writeIPC: CreateTemp → Write → Close → Rename into ipc/messages/<ts>.json
4.  fsnotify fires Create event in Watcher
5.  watcher.Watch reads dirMeta → chatID, role, subAgentID
6.  router.Route("messages", path, chatID, role, subAgentID)
7.    os.Stat — reject if > 1 MB
8.    os.ReadFile
9.    json.Unmarshal → OutboundMessage
10.   m.ChatID = chatID  [path-derived override]
11.   onMessage(m) callback
12. if m.SubAgentID != "":
13.   bubble up — re-enqueue as InboundMessage for the parent container
14. else:
15.   sender.SendMessage(chatID, text) → Telegram API
16. os.Remove(event.Name)
```

### 6.3 Sub-Agent Spawn and Bubble-Up

```
1.  Main agent calls mcp__pitu__spawnAgent(role, prompt)
2.  pitu-mcp writes AgentFile → ipc/agents/<ts>.json
3.  router.Route("agents", …) → onAgent(af)
4.  mgr.SpawnSubAgent(ctx, chatID, role, prompt) [goroutine]
5.    SanitizeRole (strip metacharacters, cap at 64 runes)
6.    SanitizePrompt (cap at 4096 runes)
7.    subAgentID = uuid.NewString()
8.    startSubAgentContainer → separate podman container
9.      writes system/SKILL.md describing role
10.     registers ipc dir with watcher (role, subAgentID metadata attached)
11.   podman exec opencode run --title <role> -- "<role>: <prompt>"
12.
13. Sub-agent calls sendMessage(text)
14. Watcher fires → router.Route with subAgentID != ""
15. onMessage sees subAgentID → builds InboundMessage:
16.   From: "Agent: <role>"
17.   Text: "[Agent: <role> (<subAgentID>)] <text>"
18. queue.Enqueue(chatID, fn)  [parent chat's queue]
19. mgr.Dispatch → parent container receives bubble-up as a new message
```

### 6.4 Scheduled Task Firing

```
1.  Cron engine fires at scheduled time
2.  scheduler dispatch(chatID, prompt)
3.  queue.Enqueue(chatID, fn)
4.  mgr.Dispatch → container receives synthetic InboundMessage
5.     From: "scheduler"
6.     MessageID: "sched-<nanos>"
```

---

## 7. Container Filesystem Layout

Every container sees this filesystem (all paths inside the container):

```
/workspace/
├── ipc/
│   ├── input/          # harness writes inbound message files here
│   ├── messages/       # pitu-mcp writes outbound messages here
│   ├── tasks/          # pitu-mcp writes task create/pause files here
│   ├── groups/         # pitu-mcp writes group registration files here
│   ├── agents/         # pitu-mcp writes sub-agent spawn requests here
│   └── reactions/      # pitu-mcp writes emoji reaction requests here
├── memory/
│   ├── AGENTS.md       # refreshed by harness on every message
│   ├── CONTEXT.md      # created once; agent's mutable scratch-pad
│   └── tasks.json      # atomic snapshot of scheduled tasks (optional)
└── skills/             # read-only; merged skill directories from host
    ├── setup/
    │   └── SKILL.md
    ├── configure-telegram/
    │   └── SKILL.md
    └── …

/root/.local/share/opencode/   # OpenCode session state (warm across messages)
/usr/local/bin/pitu-mcp        # MCP server binary
```

---

## 8. Host Filesystem Layout

```
~/.pitu/
├── config.toml             # operator config (chmod 600)
├── pitu.db                 # SQLite database
├── agent/
│   ├── IDENTITY.md         # optional: agent identity text
│   ├── SOUL.md             # optional: agent personality/voice
│   └── USER.md             # optional: notes about the operator
└── data/
    ├── skills/             # merged skill tree (generated at startup)
    │   ├── setup/
    │   └── …
    └── <chatID>/           # per-chat data (mode 0700)
        ├── ipc/            # host side of the container's /workspace/ipc
        │   ├── input/
        │   ├── messages/
        │   ├── tasks/
        │   ├── groups/
        │   ├── agents/
        │   └── reactions/
        ├── memory/         # host side of /workspace/memory
        │   ├── AGENTS.md
        │   ├── CONTEXT.md
        │   └── tasks.json
        ├── opencode/       # host side of /root/.local/share/opencode
        └── agents/
            └── <subAgentID>/   # per-sub-agent isolated tree
                ├── ipc/
                ├── memory/
                ├── skills/
                │   └── system/
                │       └── SKILL.md
                └── opencode/

~/.agents/skills/           # user-level skills (lower precedence)
~/.pitu/skills/             # user-level skills (lower precedence)
```

---

## 9. IPC Protocol Reference

All IPC files are UTF-8 JSON. File names are `<UnixNano>.json`. Files are deleted by the harness after successful routing.

### InboundMessage (harness → container)

Written to `ipc/input/` by `container.WriteInputFile`. Read by OpenCode via the `-f` flag.

```json
{
  "chat_id":    "123456789",
  "from":       "Alice",
  "text":       "Hello, schedule a daily summary for me",
  "message_id": "42"
}
```

### OutboundMessage (container → harness)

Written by `pitu-mcp` to `ipc/messages/`. `chat_id` is overwritten by the router.

```json
{
  "chat_id": "123456789",
  "text":    "I've scheduled your daily summary for 9am.",
  "type":    "message",
  "role":    "",
  "sub_agent_id": ""
}
```

### TaskFile (container → harness)

```json
{ "action": "create", "id": "<uuid>", "name": "Daily summary",
  "schedule": "0 9 * * *", "prompt": "Send a daily summary", "chat_id": "…" }

{ "action": "pause", "id": "<uuid>", "chat_id": "…" }
```

### AgentFile (container → harness)

```json
{ "action": "spawn", "sub_agent_id": "<uuid>",
  "role": "researcher", "prompt": "Find papers on X", "chat_id": "…" }
```

### ReactionFile (container → harness)

```json
{ "chat_id": "…", "message_id": 42, "emoji": "👍" }
```

### GroupFile (container → harness)

```json
{ "name": "researchers", "description": "Research sub-agents", "chat_id": "…" }
```

---

## 10. Skills Specification

Skills follow the [AgentSkills specification](https://agentskills.io/specification). Each skill is a directory containing at minimum a `SKILL.md` file with YAML frontmatter:

```markdown
---
name: my-skill
description: One-line description the agent uses to decide when to load this skill
---

# My Skill

Full instructions for the agent go here. The agent reads this file
only when the task at hand matches the description above.
```

**Precedence** (highest first):
1. Binary-adjacent skill directories
2. `~/.agents/skills/`
3. `~/.pitu/skills/`
4. `extra_paths` from config

Skills with the same `name` field: first occurrence in the precedence order wins.

**Mounting:** All skills are merged into `~/.pitu/data/skills/` at startup and mounted read-only at `/workspace/skills`. An agent cannot modify skill files.

**Bundled skills** (in `.agents/skills/`):

| Skill | Purpose |
|---|---|
| `setup` | First-time installation walkthrough |
| `configure-telegram` | Bot token and polling setup |
| `set-container-ttl` | Tune idle container TTL |
| `view-active-sessions` | List warm containers |
| `add-memory-backend` | Swap SQLite for another store |
| `update-pitu` | Pull and rebuild from latest |
| `update-model` | Change AI provider/model |

---

## 11. Concurrency Model

```
                     Telegram poller goroutine
                              │
                    ┌─────────▼──────────┐
                    │  limiter / allowlist │  (single goroutine, no lock needed)
                    └─────────┬──────────┘
                              │
                    ┌─────────▼──────────┐
                    │   queue.Enqueue     │  (mutex protects chats map)
                    └─────────┬──────────┘
                              │
              per-chat worker goroutines (one per active chat)
                              │
                    ┌─────────▼──────────┐
                    │    global semaphore │  (capacity = max_concurrent)
                    └─────────┬──────────┘
                              │
                    ┌─────────▼──────────┐
                    │   mgr.Dispatch      │
                    │   (double-checked   │
                    │    lock on start)   │
                    └─────────────────────┘

                    ┌─────────────────────┐
                    │  watcher goroutine  │  (single goroutine)
                    │  → router.Route     │  (sync.RWMutex on metas)
                    │  → onMessage/…      │
                    └─────────────────────┘

                    ┌─────────────────────┐
                    │ scheduler goroutine │  (robfig/cron internal goroutine)
                    │ → dispatch fn       │
                    │ → queue.Enqueue     │
                    └─────────────────────┘
```

**Locks in play:**

| Lock | Scope | Purpose |
|---|---|---|
| `queue.mu` | `chats` map | Create per-chat channels safely |
| `container.startMu` | `startContainer` | Prevent duplicate container starts |
| `ipc.Watcher.mu` (RWMutex) | `metas` map | Safe concurrent dir metadata lookup |
| `ratelimit.Limiter.mu` | `last` map | Per-chat timestamp updates |
| `scheduler.Scheduler.mu` | (unused directly; cron is goroutine-safe) | — |

`sync.Map` is used for the container pool and cron entry ID map — both have high read frequency and occasional writes, fitting the `sync.Map` use case.

---

## 12. Extending Pitu

### Adding a new MCP tool

1. Add a new `IPC` type to `internal/ipc/types.go` with a `ChatID` field.
2. Add a new watched subdir name (e.g. `"widgets"`) to `ipc.Watcher.RegisterDir`.
3. Add a new `case "widgets"` branch in `ipc.Router.Route` that overwrites `ChatID` before dispatch.
4. Add a callback parameter to `ipc.NewRouter` for the new event type.
5. Wire the callback in `cmd/pitu/main.go`.
6. Implement the handler in `cmd/pitu-mcp/tools.go` using `writeIPC` (atomic rename).
7. Register the MCP tool in `cmd/pitu-mcp/server.go`.

### Adding a new skill

Create a directory under `~/.agents/skills/<skill-name>/` with a `SKILL.md` file containing YAML frontmatter (`name`, `description`) and Markdown instructions. Restart the harness; the skill will be picked up by `skills.Discover`, merged into the skills mount, and listed in the agent's `AGENTS.md` catalog.

### Adding a new config section

1. Add a struct and field to `internal/config/config.go`.
2. Set defaults in `config.Load` before TOML decode.
3. Add the section to `config.example.toml` with comments.

### Swapping the persistence backend

The `store.Store` interface is thin — the four SQL tables are the entire schema. A new backend only needs to implement the same method signatures. The `add-memory-backend` skill in `.agents/skills/` guides this process interactively.

### Adding a new platform (beyond Telegram)

The harness's Telegram dependency is isolated to `internal/telegram` (poller + sender) and the wiring in `cmd/pitu/main.go`. The queue, container manager, IPC layer, skills system, and scheduler are all platform-agnostic. A new platform adapter needs to:

1. Produce `ipc.InboundMessage` values and enqueue them via `queue.Enqueue`.
2. Implement an `onMessage` callback that delivers `ipc.OutboundMessage` to the platform.
3. Handle allowlist and rate limiting at the ingress boundary.
