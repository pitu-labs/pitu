# Architecture

Pitu is a local-first agent harness. The harness receives messages from a **frontend** (default: Telegram), dispatches them to AI agents running inside rootless Podman containers, and delivers the agents' responses back through the same frontend. All communication between the harness and agents is filesystem-based (IPC).

This document is intended for two audiences: **humans** who want a mental model of the system before reading code, and **AI agents** implementing new features that need to understand the invariants they must not break.

For the security model and threat analysis, see [`docs/SECURITY.md`](SECURITY.md).

---

## Component Map

```
Frontend (Telegram / custom)
    │  inbound updates
    ▼
cmd/pitu/main.go              ← wires everything together
    │
    ├─ internal/ratelimit      ← per-chat fixed-interval limiter
    ├─ internal/queue          ← per-chat FIFO + global concurrency cap
    ├─ internal/store          ← SQLite: messages, tasks, groups, sessions
    ├─ internal/scheduler      ← cron-style task runner
    ├─ internal/skills         ← discovers and merges SKILL.md files
    │
    └─ internal/container      ← container lifecycle (start / stop / TTL / sub-agents)
           │  writes InboundMessage JSON to /workspace/ipc/input/
           │
           │  [rootless Podman container: OpenCode + pitu-mcp]
           │
           └─ internal/ipc/watcher   ← watches /workspace/ipc/* for outbound files
                  │
                  └─ internal/ipc/router  ← dispatches by subdir to handlers
```

---

## Project Layout

```
cmd/
  pitu/
    main.go         wiring: poller, queue, manager, router, scheduler
    service.go      `pitu service` subcommand (install/start/stop/logs)
    snapshot.go     writeTasksSnapshot — atomic tasks.json writer
  pitu-mcp/
    main.go         reads PITU_CHAT_ID; starts stdio MCP server
    server.go       MCP tool registration (sendMessage, scheduleTask, …)
    tools.go        tool handlers — write IPC files via atomic rename

internal/
  config/           TOML config loading and permission checking
  container/
    manager.go      Manager: ensureContainer, dispatch, SpawnSubAgent
    inputwriter.go  WriteInputFile — harness→container message delivery
    opencodecfg.go  GenerateOpenCodeConfig, APIKeyEnvVar
  ipc/
    types.go        InboundMessage, OutboundMessage, TaskFile, …
    watcher.go      fsnotify-based dir watcher; calls RegisterDir
    router.go       Route: stat → read → unmarshal → chatID override → dispatch
  queue/            per-chat FIFO with global concurrency cap
  ratelimit/        per-chat fixed-interval limiter
  scheduler/        cron-backed task scheduler (robfig/cron)
  skills/
    discovery.go    Discover — scans dirs, parses YAML frontmatter
    catalog.go      BuildCatalog — XML skill list for AGENTS.md
    context.go      WriteContext — writes AGENTS.md + CONTEXT.md
    agent.go        LoadAgentConfig — reads SOUL.md, IDENTITY.md, USER.md
    types.go        Skill struct
  store/
    store.go        New, migrate — schema bootstrap
    tasks.go        SaveTask, PauseTask, GetTasksByChatID, …
    messages.go     SaveMessage
    sessions.go     session tracking
    groups.go       RegisterGroup
  telegram/
    poller.go       long-poll getUpdates with 429/5xx backoff
    sender.go       SendMessage, SendChatAction, ReactToMessage
    types.go        Update, Message, From structs
  service/          systemd / launchd service installer

container/
  Containerfile     two-stage build: Go builder + Debian runtime

.agents/skills/     bundled operator skills (AgentSkills-compatible)
config.example.toml annotated config template
docs/               architecture and security documentation
```

---

## IPC Protocol

All harness ↔ agent communication flows through a mounted directory (`/workspace/ipc` inside the container, a per-chat path on the host). The harness **writes** to `input/`; the agent (via pitu-mcp) **writes** to all other subdirs. The harness watches for new files and acts on them.

### Inbound (harness → agent)

| Subdir | Purpose |
|--------|---------|
| `input/` | Delivers a user message to the agent |

```json
{ "chat_id": "123", "from": "Alice", "text": "Hello", "message_id": "42" }
```

### Outbound (agent → harness)

| Subdir | Go type | Purpose |
|--------|---------|---------|
| `messages/` | `OutboundMessage` | Send text to the frontend |
| `tasks/` | `TaskFile` | Create or pause a scheduled task |
| `groups/` | `GroupFile` | Register a group identity |
| `agents/` | `AgentFile` | Spawn a sub-agent |
| `reactions/` | `ReactionFile` | Set an emoji reaction on a message |

**Trust rule:** the harness derives `chat_id` from the directory path (harness-controlled), then overwrites whatever `chat_id` the container wrote. Containers cannot route to arbitrary chats.

### TaskFile

```json
{ "action": "create", "id": "<uuid>", "name": "Daily summary",
  "schedule": "0 9 * * *", "prompt": "Send a morning briefing", "chat_id": "123" }

{ "action": "pause", "id": "<uuid>", "chat_id": "123" }
```

`schedule` is a standard 5-field cron expression, a descriptor (`@daily`, `@every 5m`), or an RFC3339 timestamp.

### AgentFile

```json
{ "action": "spawn", "sub_agent_id": "<uuid>",
  "role": "researcher", "prompt": "Summarise this doc: …", "chat_id": "123" }
```

The harness sanitises `role` (`[a-zA-Z0-9 _-]`, max 64 runes) and `prompt` (max 4096 runes) before use.

### ReactionFile

```json
{ "chat_id": "123", "message_id": 42, "emoji": "👍" }
```

---

## MCP Server (`cmd/pitu-mcp`)

`pitu-mcp` runs **inside every container** as a stdio MCP server. OpenCode discovers it via `OPENCODE_CONFIG_CONTENT` (an env var injected by the harness) which declares it as a local MCP server.

The server reads its identity from environment variables set by the harness at container start:

| Variable | Set by | Purpose |
|----------|--------|---------|
| `PITU_CHAT_ID` | harness | Authoritative chat ID |
| `PITU_ROLE` | harness (sub-agents only) | Sub-agent role string |
| `PITU_SUB_AGENT_ID` | harness (sub-agents only) | UUID for result correlation |

### Exposed MCP Tools

| Tool | IPC subdir | Effect |
|------|-----------|--------|
| `sendMessage` | `messages/` | Deliver text to the frontend (or bubble to parent agent) |
| `scheduleTask` | `tasks/` | Create a recurring or one-shot cron task |
| `pauseTask` | `tasks/` | Stop a running task |
| `listTasks` | — | Return path to `/workspace/memory/tasks.json` |
| `registerGroup` | `groups/` | Register a named group identity |
| `spawnAgent` | `agents/` | Request a sub-agent container spawn |
| `reactToMessage` | `reactions/` | Set an emoji reaction on a Telegram message |

### Atomic File Write

Every tool handler writes IPC files via an atomic temp-then-rename pattern:

```
1. os.CreateTemp(ipcDir, ".tmp-")   # in the IPC root, not the watched subdir
2. tmp.Write(jsonData)
3. tmp.Close()
4. os.Rename(tmp.Name(), subdir/<timestamp>.json)
```

The rename is atomic on Linux and fires a single `IN_MOVED_TO` event (mapped to `fsnotify.Create`) with the file already fully written. A direct `os.WriteFile` would fire two events (create + write), causing a spurious read of a partial file.

**Implication for implementors:** Any new code that writes IPC files — whether in `pitu-mcp` or in tests — must use the temp-then-rename pattern.

---

## Implementing a New Frontend (Communication Channel)

The Telegram integration is a reference implementation of a **frontend adapter**. Any message source can be wired in by providing two objects:

| Object | Role | Telegram equivalent |
|--------|------|---------------------|
| **Poller** | Reads incoming messages and calls a `handler(msg)` | `internal/telegram.Poller` |
| **Sender** | Delivers outbound text to the user | `internal/telegram.Sender` |

To add a new channel as a Go package (e.g. Discord):

1. Create `internal/discord/` with `Poller` and `Sender` types matching the Telegram signatures.
2. Wire them in `cmd/pitu/main.go` in place of (or alongside) the Telegram adapter.
3. The IPC, queue, container, and scheduler layers are channel-agnostic and require no changes.

Alternatively, a channel can be delivered as an operator skill — a companion process that bridges an external platform into the IPC layer by writing `InboundMessage` JSON files directly to `~/.pitu/data/<chatID>/ipc/input/`.

---

## Skills System

Skills are Markdown files (`SKILL.md`) with YAML frontmatter that follow the [AgentSkills specification](https://agentskills.io/specification). They are loaded into the agent's context at startup.

**Discovery order (highest precedence first):**

1. `<binary-dir>/.agents/skills/` (bundled skills)
2. `<binary-dir>/.pitu/skills/`
3. `~/.agents/skills/`
4. `~/.pitu/skills/`
5. Paths in `cfg.Skills.ExtraPaths`

All discovered skills are merged into `~/.pitu/data/skills/` at startup and mounted read-only into every container. When two skills share the same `name`, the higher-precedence one wins.

**Bundled skills** (in `.agents/skills/`):

| Skill | Purpose |
|-------|---------|
| `setup` | First-time installation walkthrough |
| `configure-telegram` | Bot token and polling setup |
| `set-container-ttl` | Tune idle container TTL |
| `view-active-sessions` | List warm containers |
| `add-memory-backend` | Swap SQLite for another store |
| `update-pitu` | Pull and rebuild from latest |
| `update-model` | Change AI provider/model |

---

## Agent Personalization and Context Files

On each inbound message, `skills.WriteContext` creates or refreshes two files in the container's memory directory:

| File | Behaviour | Purpose |
|------|-----------|---------|
| `AGENTS.md` | Always overwritten | Agent identity, skills catalog, mandatory instructions |
| `CONTEXT.md` | Created once, never overwritten | Agent's mutable memory scratch-pad |

Separating these files means the operator can update the agent's persona without wiping the agent's accumulated notes.

Optional Markdown files in `~/.pitu/agent/` are injected as named sections inside `AGENTS.md` on every message:

| File | Section in AGENTS.md |
|------|---------------------|
| `IDENTITY.md` | `## Identity` |
| `SOUL.md` | `## Soul` |
| `USER.md` | `## User` |

**Rule for implementors:** New system-level context the harness injects should go into `AGENTS.md` (refreshed each message). Anything the agent accumulates over time goes into `CONTEXT.md` (created once, agent-owned).

---

## Data Layout

```
~/.pitu/
  config.toml          operator configuration (mode 0600)
  pitu.db              SQLite store (messages, tasks, groups, sessions)
  agent/               optional personalisation files
    IDENTITY.md
    SOUL.md
    USER.md
  data/
    skills/            merged skill tree (mounted ro into containers)
    <chatID>/
      ipc/
        input/         harness writes InboundMessage files here
        messages/      agent writes OutboundMessage files here
        tasks/         agent writes TaskFile files here
        groups/        agent writes GroupFile files here
        agents/        agent writes AgentFile files here
        reactions/     agent writes ReactionFile files here
      memory/
        AGENTS.md      refreshed by harness on every message
        CONTEXT.md     created once; agent's mutable scratch-pad
        tasks.json     atomic snapshot of scheduled tasks
      opencode/        OpenCode session state (warm across messages)
      agents/
        <subAgentID>/  isolated tree for each sub-agent
          ipc/
          memory/
          skills/
            system/
              SKILL.md  role + chat ID + sub-agent ID injected here
          opencode/
```

---

## Model Configuration

| Provider | `provider` value | `api_key` | `base_url` |
|----------|-----------------|-----------|------------|
| Anthropic | `anthropic` | required | leave empty |
| OpenAI | `openai` | required | leave empty |
| Ollama | `ollama` | leave empty | e.g. `http://localhost:11434/v1` |
| OpenAI-compatible | any other string | required | required |

The API key is written to a temporary env file (mode `0600`) and deleted after container start — it never appears in the JSON config blob passed to Podman.

---

## Key Architectural Decisions

### IPC Over Filesystem, Not HTTP

**Decision:** Agents communicate with the harness exclusively by writing JSON files into mounted directories. No sockets, no localhost HTTP servers, no shared memory.

**Why:** Any localhost port opened by the harness is reachable by every process on the host machine, not just the target container. Filesystem IPC scoped to per-container directories provides a tighter isolation boundary — a container only has its own chat's IPC directory mounted.

**Implication for implementors:** New inter-process capabilities must go through the filesystem IPC layer. Do not add socket listeners or HTTP servers to the harness.

---

### Path-Derived Identity

**Decision:** The `ChatID` embedded in any IPC JSON payload is always overwritten with the value derived from the filesystem path before dispatch.

```go
// router.go
m.ChatID = chatID // override: never trust container-supplied chatID
```

**Why:** A container can write any JSON it likes. Without this override, a prompt-injected agent could forge a `chat_id` pointing to a different user's conversation.

**Implication for implementors:** Every new IPC struct that carries a `ChatID` field must have that field overwritten in `router.Route` before dispatch.

---

### Atomic File Writes via Rename

**Decision:** `pitu-mcp` writes IPC files using `os.CreateTemp` followed by `os.Rename` into the watched subdirectory.

**Why:** `os.WriteFile` generates two inotify events (create empty, then modify). The watcher would attempt to read a partial file on the first event. `os.Rename` generates a single `IN_MOVED_TO` with the file already complete.

**Implication for implementors:** Any new code that writes IPC files must use the temp-then-rename pattern.

---

### Warm Container Pool with Cold Exec

**Decision:** Containers start once (with `sleep infinity`) and stay warm. OpenCode is invoked per-message via `podman exec`, not by restarting the container.

**Why:** Container cold-start (image pull, namespace setup, OpenCode initialisation) takes several seconds. Keeping containers warm means subsequent messages execute with milliseconds of overhead.

The `-c` flag passed to `opencode run` on the second and subsequent messages preserves conversation history inside OpenCode's session store without the harness re-injecting it.

**Trade-off:** Warm containers consume memory when idle. The configurable TTL (default 5 minutes) bounds resource usage to active sessions.

---

### Double-Checked Locking on Container Start

**Decision:** `ensureContainer` uses a lock-free fast path (`sync.Map.Load`) and a mutex-protected slow path with a re-check after acquiring the lock.

**Why:** Without the re-check, two concurrent messages for the same chat could both miss the fast path, both acquire the lock in turn, and each start a container — leaking one permanently.

**Implication for implementors:** Do not simplify this to a single mutex lock without the load-before-lock check.

---

### AGENTS.md vs CONTEXT.md — Identity vs Memory

**Decision:** `AGENTS.md` is overwritten on every message; `CONTEXT.md` is created once and never touched by the harness thereafter.

**Why:** The operator may update the agent's persona or skill list at any time. Refreshing `AGENTS.md` on every message picks up those changes immediately. If both files were merged into one, updating the persona would wipe the agent's accumulated notes.

**Implication for implementors:** New system-level context the harness injects → `AGENTS.md`. Context the agent accumulates over time → `CONTEXT.md` (agent-owned).

---

### Task Snapshot Pattern

**Decision:** The authoritative task store is SQLite, but a JSON snapshot (`tasks.json`) is written atomically to the agent's memory directory on every create or pause event.

**Why:** Agents running inside containers cannot query SQLite directly. Exposing an RPC to query tasks would require a socket or additional IPC round-trip. Writing a JSON snapshot to the memory directory keeps the IPC protocol unidirectional (container writes, harness reads) while still giving the agent an up-to-date task list.

---

## Concurrency Model

| Lock | Protects | Pattern |
|------|----------|---------|
| `queue.mu` (Mutex) | `chats` map | Create per-chat channels safely |
| `container.startMu` (Mutex) | `startContainer` | Prevent duplicate container starts (slow path of double-checked lock) |
| `ipc.Watcher.mu` (RWMutex) | `metas` map | Concurrent reads during dispatch; write lock only on `RegisterDir` |
| `ratelimit.Limiter.mu` (Mutex) | `last` timestamp map | Per-chat timestamp updates |
| `container.pool` (sync.Map) | container handle map | High read frequency, occasional writes |
| `scheduler.entries` (sync.Map) | cron entry IDs | High read frequency, occasional writes |

The global concurrency cap is enforced by the queue's semaphore channel (`cap = max_concurrent`). Per-chat ordering is guaranteed by per-chat worker goroutines draining a buffered channel in FIFO order.

---

## Extending Pitu

### Adding a new MCP tool

1. Add a new IPC type to `internal/ipc/types.go` with a `ChatID` field.
2. Add the new watched subdir to `ipc.Watcher.RegisterDir`.
3. Add a new `case` branch in `ipc.Router.Route` that overwrites `ChatID` before dispatch.
4. Add a callback parameter to `ipc.NewRouter` for the new event type.
5. Wire the callback in `cmd/pitu/main.go`.
6. Implement the handler in `cmd/pitu-mcp/tools.go` using `writeIPC` (atomic rename).
7. Register the MCP tool in `cmd/pitu-mcp/server.go`.

### Adding a new bundled skill

Create `.agents/skills/<name>/SKILL.md` with YAML frontmatter (`name`, `description`) and Markdown instructions. The skill is picked up at next startup, merged into the skills mount, and listed in the agent's `AGENTS.md` catalog.

### Adding a new config section

1. Add a struct and field to `internal/config/config.go`.
2. Set defaults in `config.Load` before TOML decode.
3. Add the section to `config.example.toml` with inline comments.

### Adding a new frontend (communication channel)

1. Add `internal/<channel>/` with a `Poller` (receives messages, calls handler) and a `Sender` (delivers text).
2. Wire them in `cmd/pitu/main.go` alongside or instead of the Telegram adapter.
3. Apply the same allowlist and rate-limiting gates at the ingress boundary.
4. The IPC, queue, container, and scheduler layers require no changes.

---

## Service Management

```bash
./pitu service install    # register with systemd (Linux) or launchd (macOS) and start
./pitu service uninstall  # stop and remove the service registration
./pitu service status     # show service state
./pitu service logs -n N  # tail the last N log lines
```
