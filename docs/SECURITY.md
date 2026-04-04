# Security Architecture

Pitu is designed as a local-first, single-operator AI bot harness. Its threat model centres on two principals: **the operator** (who runs the harness on their own machine) and **AI agents** (untrusted code running inside containers). Every security decision in the codebase is calibrated around these two principals and what each one should — and should not — be able to do.

---

## Design Principles

**Agents are untrusted by construction.**
An AI agent can be prompt-injected, can execute arbitrary skills, and can produce arbitrary output. The harness treats everything written by an agent as potentially adversarial.

**IPC is filesystem-only; agents never touch the network stack.**
Agents communicate with the harness exclusively by writing JSON files into a mounted directory (`/workspace/ipc`). There are no sockets, no localhost HTTP servers, and no shared memory. The harness reads those files, validates them, and acts on them. Agents have no other channel to influence the system.

**Trust comes from structure, not from content.**
The harness never trusts a field inside an IPC JSON payload to identify *who wrote it*. Identity is derived from the filesystem path — a harness-controlled structure — and overlaid onto the payload before dispatch.

**Least privilege at every boundary.**
Every file and directory created by the harness uses the tightest mode that still allows the process to function. Secrets are kept out of the widest-blast-radius locations.

---

## Container Isolation

Agents run inside **rootless Podman containers**, which means:

- The container daemon runs without root privileges on the host.
- Even if a container escape occurred, the attacker would be a non-root user on the host system.
- Each container is launched with `--rm`, so no container state persists after it stops.

**Memory limit** — every container is started with `--memory <limit>` (default `512m`, operator-configurable). An agent that attempts to allocate unbounded memory is killed by the kernel OOM killer before it can impact the host.

**Container TTL** — containers that have been idle longer than the configured TTL (default `5m`) are stopped automatically via a per-container `time.AfterFunc` timer. This bounds the resource footprint to active sessions only.

**Volume mounts are scoped and minimal:**

| Host path | Container path | Mode |
|---|---|---|
| Chat IPC directory | `/workspace/ipc` | read-write |
| Chat memory directory | `/workspace/memory` | read-write |
| Skills directory | `/workspace/skills` | **read-only** |
| OpenCode data directory | `/root/.local/share/opencode` | read-write |

Skills are mounted read-only (`ro,z`). An agent cannot modify the skill files it is running from.

**`PITU_CHAT_ID` is injected by the harness.** The container's own identity is set as an environment variable at start time, not read from any file the agent controls. An agent cannot change its own `PITU_CHAT_ID`.

---

## IPC Trust Model

### Path-Derived Identity

Every IPC directory is registered with a harness-side metadata record that stores the authoritative `chatID` — derived from the directory path, not from any JSON payload. When the router processes an IPC file, it:

1. Looks up the directory's registered `chatID`.
2. Overwrites the `ChatID` field in the parsed struct with the authoritative value.
3. Dispatches with the overwritten value.

This means a container cannot deliver a message, create a task, register a group, spawn a sub-agent, or set a reaction in a chat other than the one it was assigned to — even if it writes a JSON file with a forged `chat_id` field. The forgery is silently corrected before anything acts on it.

### File Size Cap

Before reading any IPC file into memory, the router calls `os.Stat` and rejects files larger than **1 MB** (`maxIPCFileSize = 1 << 20`). A compromised or misbehaving agent cannot cause an out-of-memory condition by writing a large file.

### Sub-Agent Input Sanitization

When an agent requests a sub-agent spawn, the `role` and `prompt` fields from the IPC payload are sanitized before use:

- `role` is filtered to `[a-zA-Z0-9 _-]`, capped at **64 runes**, and defaults to `"agent"` if nothing survives. This prevents injection of arbitrary Markdown or shell metacharacters into the sub-agent's system skill file.
- `prompt` is capped at **4096 runes**. This limits the surface area for prompt injection into the sub-agent's initial context.

Both sanitizations are applied at the `SpawnSubAgent` entry point, before the values are written to any file or passed as a shell argument.

---

## Access Control

### Telegram Sender Allowlist

The harness supports an operator-configured allowlist of Telegram chat IDs:

```toml
[telegram]
allowed_chat_ids = [123456789, 987654321]
```

The allowlist check is the **first action** taken on every incoming update — before any database write, container dispatch, or typing indicator. Messages from chat IDs not on the list are silently dropped and logged. If the list is empty, all senders are accepted (backward-compatible default suitable for private deployments).

### Per-Chat Rate Limiting

A per-chat fixed-interval limiter prevents any single user from flooding the dispatch queue:

```toml
[telegram]
rate_limit = "5s"  # minimum gap between accepted messages per chat
```

Messages that arrive within the rate window are dropped and logged. This protects the global concurrency pool (`max_concurrent`) from being monopolised by a single chat. Omitting the key or setting it to `""` disables limiting.

---

## Secret Management

### API Key Isolation

The model API key is written to a **temporary env file** (mode `0600`) rather than embedded in the `OPENCODE_CONFIG_CONTENT` JSON blob. This matters because:

- The JSON blob may appear in container logs, error output, or `podman inspect` output.
- The env file is read by Podman at container start, then deleted from the host (`defer os.Remove`).
- Inside the container, the key is present as a standard provider-specific environment variable (e.g. `ANTHROPIC_API_KEY`) rather than buried inside a JSON string.

The Vercel AI SDK reads these standard variables natively, so no functionality is lost.

### Config File Permissions

At startup, the harness checks whether `~/.pitu/config.toml` is readable by group or world (`mode & 0o077 != 0`). If it is, a warning is logged:

```
pitu: security warning: config file /home/user/.pitu/config.toml has permissions 0644; tighten with: chmod 600 /home/user/.pitu/config.toml
```

This is non-fatal — the operator retains control — but it surfaces the issue immediately rather than silently.

---

## Filesystem Permissions

Every directory and file created by the harness uses restrictive modes:

| Resource | Mode | Reason |
|---|---|---|
| Data directories (`ipc/`, `memory/`, `opencode/`, `agents/`) | `0700` | Owner-only traversal; no other local user can list or enter |
| Input message files | `0600` | Contains user message text; owner-read/write only |
| Env files (ephemeral) | `0600` | Contains API key; deleted after container start |
| Skill files written by harness | `0600` | Sub-agent system skill content |
| Database directory | `0700` | Prevents enumeration of the SQLite file |

For directories that may have been created by an older version of the harness with looser permissions, `os.Chmod(dir, 0700)` is called after `os.MkdirAll` to tighten existing directories on upgrade.

---

## Concurrency Safety

The container pool is protected by a **double-checked locking pattern**:

- A fast, lock-free read path (`sync.Map.Load`) handles warm containers with zero contention.
- A serialised slow path (`sync.Mutex`) gates new container starts, preventing two goroutines from racing to start the same container for the same chat and leaking the first one.

The IPC directory metadata map is protected by a `sync.RWMutex` to allow concurrent reads during message routing while serialising directory registration.

---

## Telegram API Resilience

The Telegram poller checks `resp.StatusCode` before decoding:

- **429 Too Many Requests** — returns a typed `retryError` carrying the `Retry-After` header value (default 10 s). The poll loop applies this exact backoff before retrying.
- **5xx Server Error** — returns a plain error; the poll loop applies a 2 s backoff before retrying.
- **Other non-200** — treated as a transient error; same 2 s backoff.

In all error cases, the response body is not decoded, preventing garbage updates from entering the dispatch pipeline.

---

## Graceful Shutdown

On `SIGINT` or `SIGTERM`, the harness:

1. Cancels the root context (stops the poller and watcher).
2. Calls `mgr.StopAll()`, which sends `podman stop` to every warm container.
3. Drains the dispatch queue.

This ensures containers are not left running as orphans after the harness exits.

---

## Auditable by Design

The codebase is intentionally small (~4,000 lines, ~15 source files in `internal/`). A small, focused codebase is easier to audit than a large one. The project does not accept feature pull requests upstream; new capability comes through user-authored skills, which run inside the same container sandbox as the agent.

For ongoing dependency security, run:

```bash
govulncheck ./...
```

This checks the versions declared in `go.mod` against the Go vulnerability database and reports any known advisories.
