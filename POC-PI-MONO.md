# PoC Report: Pi-Mono Integration

- **Branch:** `feature/pi-mono-poc` → archived as `archived/feature/pi-mono-poc`
- **Date:** 2026-04-05
- **Status:** Archived — not merged
- **Spec:** `docs/superpowers/specs/2026-04-04-pi-mono-integration-design.md`

## Summary

This branch explored replacing OpenCode with Pi-Mono (`@mariozechner/pi-coding-agent`) as Pitu's agent runtime. The integration was bootstrapped by a coding agent following the spec. After code review and architectural analysis, the branch was archived without merging.

## What Was Built

- `Containerfile.pimono` — clean two-stage image (Go builder + Node runtime)
- `pitu-pi-wrapper.sh` — bash bridge translating Pitu's JSON input format to `pi` CLI args
- `internal/container/manager.go` — runtime switch (`runtime = "opencode" | "pimono"`) with backward-compatible default
- `internal/ipc/watcher.go` — `RegisterAuditFile` to watch `log.jsonl`
- `internal/container/opencodecfg.go` — `GeneratePiMonoConfig` for MCP config generation
- `cmd/pitu/main.go` — `/trace` command to expose Pi-Mono's reasoning log to Telegram

## Key Findings

### Code Quality Issues (from review)

| Severity | Issue |
|----------|-------|
| Critical | Compiled `pitu` binary committed to git (supply-chain risk) |
| Critical | Double `WriteInputFile` call triggers spurious watcher events |
| Critical | `MODEL` env var passed to `pi --model` without allowlist validation |
| Critical | Reply path is a stub — the audit watcher logs but never delivers to Telegram |
| Important | `GetTrace` loads entire `log.jsonl` into memory (contradicts the spec's own mitigation) |
| Important | `GeneratePiMonoConfig` ignores its `model` parameter |
| Important | `opencodeDir` created and mounted for Pi-Mono containers that don't use it |
| Important | `config.example.toml` missing the new `runtime` field |

### Unverified Core Assumption

The entire architecture depends on `pi -p` (print mode) invoking MCP tool calls during execution. If print mode short-circuits the MCP loop — which is common in CLI tools with a "quick answer" mode — then `pitu-mcp` is never called and there is no reply path at all. This assumption was never tested before the integration was built.

### Design Assessment

The spec's architectural instincts are sound (keep `pitu-mcp` as the bridge, clean-slate image, managed audit logs as a separate concern from reply delivery). The implementation failed to distinguish between two different things:

- **Reply delivery** (Phase 1): agent response → Telegram. Needs `pitu-mcp` → `ipc/messages/` → watcher → Telegram.
- **Audit traces** (Phase 3): internal reasoning → `/trace` command. Needs `log.jsonl` watching.

The watcher stub was built in the Phase 3 position while Phase 1 was left unimplemented. The integration dispatches prompts to the container but users receive no responses.

## Why Pi-Mono Was Not Adopted

| Criterion | OpenCode | Pi-Mono |
|-----------|----------|---------|
| IPC model fit | Native — `sendMessage` writes files | Requires bridging — stdout or unverified MCP |
| Reply path | Proven, clean, tested | Unimplemented |
| Session continuity | Explicit `-c` flag, tested | Unverified across `podman exec` calls |
| Audit traces | Not structured | Genuinely better (`log.jsonl` tree sessions) |
| Stability/maturity | Broader community | Single-developer, unpinned npm package |
| Integration complexity | Minimal (already working) | Non-trivial with unverified assumptions |

**The only column Pi-Mono wins clearly is audit traces** — and that feature was not implemented in this PoC.

The "leaner dependency" and "better extensibility" motivations don't hold under scrutiny: OpenCode's weight is container-isolated, and Pitu's extensibility story is already `pitu-mcp` (which the spec kept regardless). Pi-Mono's TypeScript extension system only becomes an advantage if the Go bridge is abandoned entirely.

## If You Return to This

Before rebuilding, answer these two questions first:

1. **Does `pi -p --session log.jsonl` invoke MCP tools?** Run a one-shot test: invoke `pi -p` with a prompt that calls a known MCP tool, observe whether the tool fires. This single test validates or invalidates the entire architecture.

2. **Does `--session log.jsonl` correctly resume conversation state across separate `podman exec` calls?** OpenCode's `-c` flag is explicit about this. Pi-Mono's behavior needs verification.

If both answers are yes, the architecture is sound and only targeted fixes are needed (see issues above). If either is no, the approach needs rethinking before any code is written.

## Recommendation

Stay on OpenCode. If structured audit traces become a first-class user requirement, revisit Pi-Mono as a deliberate project — with the two questions above answered first, `@mariozechner/pi-coding-agent` pinned to a specific version, and the reply path implemented before anything else.
