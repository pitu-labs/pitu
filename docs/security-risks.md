# Security Risk Report

**Date:** 2026-04-03  
**Scope:** Full codebase — `cmd/`, `internal/`, `go.mod`  
**Reviewer:** Claude Code (claude-sonnet-4-6)  
**Last updated:** 2026-04-03 — M2, M3, M4 remediated; M2 review fix (pre-existing dir permissions); M3 boundary test added

---

## Summary

| ID | Severity | Status | Location | Issue |
|---|---|---|---|---|
| H1 | **HIGH** | ✅ Fixed | `ipc/router.go`, `cmd/pitu/main.go` | Cross-chat injection via forged `chat_id` in IPC JSON |
| H2 | **HIGH** | ✅ Fixed | `cmd/pitu/main.go` | No Telegram sender authorization (open bot) |
| H3 | **HIGH** | ⚠️ Mitigated | `container/opencodecfg.go`, `container/manager.go` | API key exposed in container environment |
| M1 | **MEDIUM** | 🔴 Open | `container/inputwriter.go:27` | Input file world-readable (0644 → 0600) |
| M2 | **MEDIUM** | ✅ Fixed | `ipc/watcher.go:44` | IPC directories world-traversable (0755 → 0700) |
| M3 | **MEDIUM** | ✅ Fixed | `ipc/router.go:23` | Unbounded IPC file read (potential OOM/DoS) |
| M4 | **MEDIUM** | ✅ Fixed | `container/manager.go:77-82` | TOCTOU in container pool → container leak |
| M5 | **MEDIUM** | 🔴 Open | `cmd/pitu/main.go` | No per-chat rate limiting |
| L1 | **LOW** | 🔴 Open | `container/manager.go:160,309` | Prompt injection via `role`/`prompt` fields |
| L2 | **LOW** | 🔴 Open | `config/config.go` | Bot token stored in plaintext config |
| L3 | **LOW** | 🔴 Open | `telegram/poller.go` | HTTP status not checked before decoding |
| L4 | **LOW** | 🔴 Open | `go.mod:3` | `go 1.25.0` is a non-existent Go version |

---

## High Severity

### H1 — Cross-chat message injection via forged `chat_id` in IPC files

> ✅ **Fixed** — commits `3d538bd`, `6413581`

**Files:** `internal/ipc/router.go:23-35`, `cmd/pitu/main.go:115`

The harness trusts the `chat_id` field **inside the JSON** written by a container. A container mounted at `/workspace/ipc` for chat `123` can write a message file containing `{"chat_id": "456", "text": "..."}`. The harness will call `sender.SendMessage("456", ...)` — delivering a message to a completely different user's chat.

The same applies to `TaskFile.ChatID`, `GroupFile.ChatID`, `AgentFile.ChatID`, and `ReactionFile.ChatID`: a container can create tasks, register groups, spawn sub-agents, or set reactions in any other chat by forging the field.

**Risk:** One compromised or misbehaving AI agent can exfiltrate data to any Telegram chat, impersonate the bot to other users, or trigger operations on other users' task lists.

**Remediation applied:** `chatID` is now derived from the IPC directory path (harness-controlled) and stored in `dirMeta` at `RegisterDir` time. `Router.Route` accepts it as a trusted parameter and overwrites the JSON-supplied `ChatID` field in every IPC struct before dispatching. A `sync.RWMutex` was also added to protect the `metas` map against concurrent `RegisterDir` + `Watch` access. Forge-and-override tests exist for all five IPC subdirs.

---

### H2 — No Telegram sender authorization (open bot)

> ✅ **Fixed** — commit `a6f07d2`

**File:** `cmd/pitu/main.go:227-258`

Any Telegram user who discovers the bot's username can send messages. There is no allowlist, authentication, or verification of the sender's identity. Every incoming message unconditionally spawns a container and runs an AI agent at the operator's expense.

**Risk:** Unauthorized usage, resource exhaustion (containers + AI API costs), and potential data exfiltration if the bot has access to sensitive skills or data.

**Remediation applied:** `TelegramConfig` now accepts `allowed_chat_ids = [...]` (a list of `int64` Telegram chat IDs). The poller callback checks this list as its very first action — before any DB write, container dispatch, or `SendChatAction` call. Rejected senders are silently dropped and logged. If the list is empty, all senders are accepted (backward-compatible default). Configure in `~/.pitu/config.toml`.

---

### H3 — API key exposed in container environment

> ⚠️ **Mitigated** — commit `2378596` — residual risk remains (see below)

**Files:** `internal/container/opencodecfg.go:47`, `internal/container/manager.go:111`

The model API key is embedded in `OPENCODE_CONFIG_CONTENT` (a JSON blob) and injected as an environment variable into every container. Any code running inside the container — including the AI agent, any tool it invokes, or any skill it executes — can read the full process environment and extract the key.

**Risk:** A prompt-injected or malicious skill could leak the API key via an outbound message or network call.

**Mitigation applied:** The API key has been removed from the `OPENCODE_CONFIG_CONTENT` JSON blob. It is now written as a dedicated provider-specific environment variable (`ANTHROPIC_API_KEY` or `OPENAI_API_KEY`) in the same env file, which the Vercel AI SDK reads natively. This means the config JSON — which may appear in logs or error output — no longer contains the key.

**Residual risk:** The API key is still present in the container's process environment (as a standard env var), so it remains accessible to code running inside the container. A complete fix would require a provider-side proxy that issues short-lived, scoped tokens per session — deferred as a future hardening task.

---

## Medium Severity

### M1 — Input file world-readable (0644)

**File:** `internal/container/inputwriter.go:27`

```go
os.WriteFile(path, data, 0644)
```

Inbound Telegram messages (user text and metadata) are written to host disk with `0644`, making them readable by any local process running as any user on the host.

**Remediation:** Change permission to `0600`.

---

### M2 — IPC subdirectories created with 0755

> ✅ **Fixed**

**File:** `internal/ipc/watcher.go:44`

```go
os.MkdirAll(dir, 0755)
```

IPC directories — containing all agent-produced files (tasks, messages, agent spawn requests) — are world-traversable. Any local process can enumerate and read their contents.

**Remediation applied:** `os.MkdirAll(dir, 0700)` sets the permission on *newly created* directories only — it is a no-op on existing ones. `os.Chmod(dir, 0700)` is called immediately after to tighten permissions regardless of whether the directory pre-existed. Two tests cover this: `TestWatcher_RegisterDir_CreatesPrivateSubdirs` (fresh dirs) and `TestWatcher_RegisterDir_RemediatesExistingLoosePerms` (dirs pre-created with `0755`, simulating an upgrade).

---

### M3 — No size limit on IPC file reads (potential OOM/DoS)

> ✅ **Fixed**

**File:** `internal/ipc/router.go:23`

```go
data, err := os.ReadFile(path)
```

A container can write an arbitrarily large JSON file. The harness reads it entirely into memory with no size cap, which can cause OOM if a container is compromised or misbehaves.

**Remediation applied:** `Route` now calls `os.Stat` before `os.ReadFile` and returns an error if the file exceeds 1 MB (`maxIPCFileSize = 1 << 20`). The guard is strictly greater-than, so a file of exactly 1 MB is accepted. Two tests cover the boundary: `TestRouter_RejectsOversizedFile` (one byte over the limit is rejected) and `TestRouter_AcceptsFileAtSizeLimit` (exactly at the limit passes the size check). A minor TOCTOU window exists between `Stat` and `ReadFile`, but exploiting it requires racing a file replacement within the same small window — acceptable given the threat model.

---

### M4 — TOCTOU race in container pool (container leak)

> ✅ **Fixed**

**File:** `internal/container/manager.go:77-82`

```go
if v, ok := m.pool.Load(chatID); ok {
    return v.(*Handle), nil
}
return m.startContainer(ctx, chatID)
```

Two concurrent messages for the same `chatID` can both find an empty pool entry and both call `startContainer`. The second container started is stored via `pool.Store`, silently discarding the first handle. The first container continues running but is no longer tracked — it will never be stopped.

**Remediation applied:** `Manager` now holds a `startMu sync.Mutex`. `ensureContainer` uses a double-checked locking pattern: a fast lock-free path for warm containers (no lock acquired), and a serialised slow path for new starts. A second `pool.Load` under the lock prevents duplicate starts when goroutines race to the slow path.

**Known caveat:** `startMu` is global across all chats, so two different chats needing new containers simultaneously will serialize. This is acceptable at the current single-operator scale; a per-chatID singleflight pattern would be more precise if concurrency requirements grow.

**Test gap:** `ensureContainer` is unexported and `startContainer` shells out to `podman`, making the concurrent path untestable in unit tests. The fix is verified by code inspection. Use `go test -race ./internal/container/...` against a live Podman environment for runtime validation.

---

### M5 — No per-chat rate limiting

**File:** `cmd/pitu/main.go:227`

The polling loop dispatches every incoming message to the queue with no rate limiting per chat or per user. A single user can flood the queue, consuming the global concurrency cap (`max_concurrent`) and starving all other chats.

**Remediation:** Track per-chat message timestamps and drop or defer messages that exceed a configurable rate threshold (e.g. 1 message per 5 seconds per chat).

---

## Low Severity

### L1 — Prompt injection via `role` / `prompt` fields in sub-agent spawn

**File:** `internal/container/manager.go:160-163`, `309-315`

The `role` and `prompt` fields from `AgentFile` (written by the container) are used verbatim:

- Written into `SKILL.md` as freeform Markdown without sanitization.
- Concatenated into the OpenCode prompt argument: `role + ": " + prompt`.

A container-side agent could exploit this to inject instructions into a sub-agent's system skill or initial prompt.

**Remediation:** Sanitize or length-limit `role` (e.g. alphanumeric + spaces only, max 64 chars). Treat it as an opaque identifier, not freeform text, when writing to skill files.

---

### L2 — Bot token stored in plaintext config

**File:** `internal/config/config.go:32`, `config.example.toml`

`bot_token` is stored as a plain string in `~/.pitu/config.toml`. The harness does not verify the file's permissions before reading it, so a misconfigured system (e.g. world-readable home directory, dotfile sync) could expose the token.

**Remediation:** Document that the config file must be `chmod 600`. Optionally, add a startup check that warns or aborts if the file's permissions are too permissive.

---

### L3 — Telegram API HTTP status not checked before decoding

**File:** `internal/telegram/poller.go:54-67`

`getUpdates` decodes the response body without checking `resp.StatusCode`. A `429 Too Many Requests` or `5xx` response from Telegram is silently decoded as an empty update list instead of triggering a back-off retry.

**Remediation:** Check `resp.StatusCode` before decoding. Implement exponential back-off on `429` and `5xx` responses, respecting the `Retry-After` header when present.

---

### L4 — `go 1.25.0` is a non-existent Go version

**File:** `go.mod:3`

Go 1.25 does not exist as of the review date. Newer `go` toolchains that honor the `go` directive for toolchain selection may attempt to download a non-existent version, causing build failures in CI or fresh environments.

**Remediation:** Change the directive to the actual minimum Go version required (e.g. `go 1.23.0`).

---

## Dependency CVE Scan

Versions declared in `go.mod` were checked against known advisories. No CVEs were identified for the versions in use. Run `govulncheck ./...` regularly for a live, authoritative scan, as new advisories are disclosed continuously.

| Package | Version | Status |
|---|---|---|
| `modernc.org/sqlite` | v1.47.0 | No known CVEs |
| `github.com/fsnotify/fsnotify` | v1.9.0 | No known CVEs |
| `github.com/robfig/cron/v3` | v3.0.1 | No known CVEs |
| `github.com/BurntSushi/toml` | v1.6.0 | No known CVEs |
| `github.com/mark3labs/mcp-go` | v0.46.0 | No known CVEs |
| `github.com/google/uuid` | v1.6.0 | No known CVEs |
| `golang.org/x/sys` | v0.42.0 | No known CVEs |
| `gopkg.in/yaml.v3` | v3.0.1 | No known CVEs |
