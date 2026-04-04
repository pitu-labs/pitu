# Low-Severity Security Remediation (L1, L2, L3, L4) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remediate all four open low-severity findings from `docs/security-risks.md` (L1 prompt injection, L2 config permissions, L3 HTTP status handling, L4 go.mod version).

**Architecture:** Each fix is self-contained to its package; no cross-package dependencies are introduced. L4 requires only a doc update (Go 1.25.0 is installed). All code changes are TDD: failing test first, then minimal implementation.

**Tech Stack:** Go 1.25, `net/http`, `net/http/httptest`, `testify/assert`, `testify/require`

---

## File Map

| File | Change |
|---|---|
| `internal/container/manager.go` | Add `SanitizeRole`, `SanitizePrompt`; call them at `SpawnSubAgent` entry |
| `internal/container/container_test.go` | Add sanitization unit tests |
| `internal/config/config.go` | Add `CheckPermissions(path string) error` |
| `internal/config/config_test.go` | Add permissions unit tests |
| `internal/telegram/poller.go` | Add `retryError` type, `parseRetryAfter`; check status before decoding |
| `internal/telegram/telegram_test.go` | Add 429/5xx behaviour tests |
| `cmd/pitu/main.go` | Log warning from `config.CheckPermissions` after `config.Load` |
| `docs/security-risks.md` | Mark L1–L4 as fixed |

---

## Task 1: L1 — Sanitize `role` and `prompt` in sub-agent spawn

The `role` field is written verbatim into `SKILL.md` content and passed as a shell argument (`--title role`). A container-side agent can inject arbitrary Markdown or prompt text. `prompt` is concatenated directly into the OpenCode run argument with no length cap.

**Files:**
- Modify: `internal/container/manager.go` (add helpers + call in `SpawnSubAgent`)
- Test: `internal/container/container_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/container/container_test.go`:

```go
func TestSanitizeRole_AllowsSafeChars(t *testing.T) {
	assert.Equal(t, "My Agent-1_2", container.SanitizeRole("My Agent-1_2"))
}

func TestSanitizeRole_StripsInjectionChars(t *testing.T) {
	// newline, null byte, markdown heading — all stripped
	assert.Equal(t, "BadAgent", container.SanitizeRole("Bad\nAgent\x00"))
}

func TestSanitizeRole_CapsAt64Runes(t *testing.T) {
	long := strings.Repeat("a", 100)
	result := container.SanitizeRole(long)
	assert.Equal(t, 64, len([]rune(result)))
}

func TestSanitizeRole_EmptyInputReturnsAgent(t *testing.T) {
	assert.Equal(t, "agent", container.SanitizeRole(""))
	assert.Equal(t, "agent", container.SanitizeRole("!@#$%^&*"))
}

func TestSanitizePrompt_TruncatesOverLimit(t *testing.T) {
	long := strings.Repeat("x", 5000)
	result := container.SanitizePrompt(long)
	assert.Equal(t, 4096, len([]rune(result)))
}

func TestSanitizePrompt_PassesUnderLimit(t *testing.T) {
	assert.Equal(t, "hello world", container.SanitizePrompt("hello world"))
}
```

`strings` is already available in the test file via other imports; if not, add `"strings"` to the import block.

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./internal/container/... -run "TestSanitizeRole|TestSanitizePrompt" -v
```

Expected: `FAIL` — `container.SanitizeRole undefined`

- [ ] **Step 3: Implement `SanitizeRole` and `SanitizePrompt` in `manager.go`**

Add `"strings"` to the import block in `internal/container/manager.go`, then append these two functions (before the closing of the file):

```go
// SanitizeRole strips characters outside [a-zA-Z0-9 _-], caps the result at
// 64 runes, and returns "agent" if nothing survives. Exported for testability.
func SanitizeRole(role string) string {
	var b strings.Builder
	for _, r := range role {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == ' ' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	runes := []rune(b.String())
	if len(runes) > 64 {
		runes = runes[:64]
	}
	if len(runes) == 0 {
		return "agent"
	}
	return string(runes)
}

// SanitizePrompt caps prompt at 4096 runes. Exported for testability.
func SanitizePrompt(prompt string) string {
	runes := []rune(prompt)
	if len(runes) > 4096 {
		return string(runes[:4096])
	}
	return prompt
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./internal/container/... -run "TestSanitizeRole|TestSanitizePrompt" -v
```

Expected: all 6 tests `PASS`

- [ ] **Step 5: Wire sanitization into `SpawnSubAgent`**

In `internal/container/manager.go`, replace the first two lines of `SpawnSubAgent`:

```go
// Before (lines 328–329):
func (m *Manager) SpawnSubAgent(ctx context.Context, chatID, role, prompt string) {
	subAgentID := uuid.NewString()
```

```go
// After:
func (m *Manager) SpawnSubAgent(ctx context.Context, chatID, role, prompt string) {
	role = SanitizeRole(role)
	prompt = SanitizePrompt(prompt)
	subAgentID := uuid.NewString()
```

- [ ] **Step 6: Run all container tests**

```
go test ./internal/container/... -v
```

Expected: all tests `PASS`

- [ ] **Step 7: Commit**

```bash
git add internal/container/manager.go internal/container/container_test.go
git commit -m "fix(security): sanitize role and prompt before sub-agent spawn (L1)"
```

---

## Task 2: L2 — Warn on insecure config file permissions

`~/.pitu/config.toml` contains the bot token. If the file is world-readable (e.g. 0644) a local user or dotfile sync can read it. This task adds a startup warning.

**Files:**
- Modify: `internal/config/config.go` (add `CheckPermissions`)
- Test: `internal/config/config_test.go`
- Modify: `cmd/pitu/main.go` (add warning call)

- [ ] **Step 1: Write failing tests**

Append to `internal/config/config_test.go`:

```go
func TestCheckPermissions_PassesOn0600(t *testing.T) {
	f := writeTempTOML(t, "[telegram]\nbot_token = \"tok\"\n")
	require.NoError(t, os.Chmod(f, 0600))
	assert.NoError(t, config.CheckPermissions(f))
}

func TestCheckPermissions_ErrorOnGroupReadable(t *testing.T) {
	f := writeTempTOML(t, "[telegram]\nbot_token = \"tok\"\n")
	require.NoError(t, os.Chmod(f, 0640))
	err := config.CheckPermissions(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0640")
}

func TestCheckPermissions_ErrorOnWorldReadable(t *testing.T) {
	f := writeTempTOML(t, "[telegram]\nbot_token = \"tok\"\n")
	require.NoError(t, os.Chmod(f, 0644))
	err := config.CheckPermissions(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0644")
}

func TestCheckPermissions_ErrorOnMissingFile(t *testing.T) {
	assert.Error(t, config.CheckPermissions("/nonexistent/path/config.toml"))
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./internal/config/... -run "TestCheckPermissions" -v
```

Expected: `FAIL` — `config.CheckPermissions undefined`

- [ ] **Step 3: Implement `CheckPermissions` in `config.go`**

Append to `internal/config/config.go`:

```go
// CheckPermissions returns an error if the config file at path is readable by
// group or world (i.e. mode & 0o077 != 0). Call after Load and log the result
// as a warning — the bot token must not be world-readable.
func CheckPermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("config: stat %s: %w", path, err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("config file %s has permissions %04o; tighten with: chmod 600 %s",
			path, info.Mode().Perm(), path)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./internal/config/... -run "TestCheckPermissions" -v
```

Expected: all 4 tests `PASS`

- [ ] **Step 5: Wire the warning into `cmd/pitu/main.go`**

In `cmd/pitu/main.go`, immediately after the `config.Load` block (around line 36–38), add:

```go
cfg, err := config.Load(cfgPath)
if err != nil {
    log.Fatalf("pitu: config: %v", err)
}
if err := config.CheckPermissions(cfgPath); err != nil {
    log.Printf("pitu: security warning: %v", err)
}
```

- [ ] **Step 6: Run all config and main-package tests**

```
go test ./internal/config/... ./cmd/pitu/... -v 2>/dev/null || go build ./cmd/pitu
```

Expected: all `PASS` (or binary builds without error if no main-package tests exist)

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go cmd/pitu/main.go
git commit -m "fix(security): warn on insecure config file permissions at startup (L2)"
```

---

## Task 3: L3 — Check HTTP status before decoding Telegram responses

`getUpdates` currently decodes the response body without checking `resp.StatusCode`. A `429 Too Many Requests` or `5xx` from Telegram is silently decoded as an empty update list rather than triggering a back-off retry.

**Files:**
- Modify: `internal/telegram/poller.go`
- Test: `internal/telegram/telegram_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/telegram/telegram_test.go`:

```go
func TestPoller_IgnoresUpdatesOn429(t *testing.T) {
	// Server always returns 429 with Retry-After: 1 — context expires before retry.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := telegram.NewPoller("TOKEN", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var received []telegram.Update
	p.Poll(ctx, func(u telegram.Update) { received = append(received, u) })
	assert.Empty(t, received, "no updates should be delivered on 429 responses")
}

func TestPoller_RetriesAfter429(t *testing.T) {
	// First call returns 429 with Retry-After: 0 (immediate retry).
	// Second call returns a real update. Poll must deliver it.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":[{"update_id":42,"message":{"message_id":1,"from":{"id":1,"first_name":"Bob"},"chat":{"id":1},"text":"retry","date":1}}]}`))
	}))
	defer srv.Close()

	p := telegram.NewPoller("TOKEN", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var received []telegram.Update
	p.Poll(ctx, func(u telegram.Update) { received = append(received, u) })
	assert.GreaterOrEqual(t, callCount, 2, "Poll must retry after a 429")
}

func TestPoller_RetriesAfter5xx(t *testing.T) {
	// First call returns 500. Second call returns empty updates.
	// Poll must retry (callCount >= 2). Uses 3s timeout to cover the 2s backoff.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer srv.Close()

	p := telegram.NewPoller("TOKEN", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	p.Poll(ctx, func(telegram.Update) {})
	assert.GreaterOrEqual(t, callCount, 2, "Poll must retry after a 5xx")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./internal/telegram/... -run "TestPoller_IgnoresUpdatesOn429|TestPoller_RetriesAfter429|TestPoller_RetriesAfter5xx" -v
```

Expected: `FAIL` — tests that assert retry (`callCount >= 2`) will fail because the current code does not check status and loops without backing off correctly. The `IgnoresUpdatesOn429` test may not fail (it currently succeeds by accident due to decode failure), but running all three isolates the behaviour.

- [ ] **Step 3: Replace `internal/telegram/poller.go` with the corrected version**

The complete file (replaces existing content):

```go
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type Poller struct {
	token   string
	baseURL string
	client  *http.Client
}

// retryError is returned by getUpdates when the server signals a back-off.
// The after field carries the duration from the Retry-After header (or a default).
type retryError struct {
	after time.Duration
}

func (e *retryError) Error() string {
	return fmt.Sprintf("telegram: rate limited; retry after %s", e.after)
}

// parseRetryAfter converts a Retry-After header value (integer seconds) to a
// Duration. Returns 10 s on empty, negative, or non-integer values.
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 10 * time.Second
	}
	n, err := strconv.Atoi(header)
	if err != nil || n < 0 {
		return 10 * time.Second
	}
	return time.Duration(n) * time.Second
}

func NewPoller(token, baseURL string) *Poller {
	return &Poller{
		token:   token,
		baseURL: fmt.Sprintf("%s/bot%s", baseURL, token),
		client:  &http.Client{Timeout: 35 * time.Second},
	}
}

// Poll calls handler for each received Update until ctx is cancelled.
func (p *Poller) Poll(ctx context.Context, handler func(Update)) {
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		updates, err := p.getUpdates(ctx, offset, 30)
		if err != nil {
			var re *retryError
			backoff := 2 * time.Second
			if errors.As(err, &re) {
				backoff = re.after
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				continue
			}
		}
		for _, u := range updates {
			handler(u)
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
		}
	}
}

func (p *Poller) getUpdates(ctx context.Context, offset, timeout int) ([]Update, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=%d", p.baseURL, offset, timeout)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &retryError{after: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("telegram: server error %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Result, nil
}
```

- [ ] **Step 4: Run all telegram tests**

```
go test ./internal/telegram/... -v -timeout 30s
```

Expected: all tests `PASS` (the `RetriesAfter5xx` test takes ~2 s due to the fixed backoff — that is intentional)

- [ ] **Step 5: Commit**

```bash
git add internal/telegram/poller.go internal/telegram/telegram_test.go
git commit -m "fix(security): check HTTP status before decoding; back off on 429/5xx (L3)"
```

---

## Task 4: L4 — Close as verified non-issue

The report stated "Go 1.25 does not exist." This was true at review time but is no longer accurate — Go 1.25.0 was released and is the installed toolchain (`go version go1.25.0 linux/amd64`). No code change is needed.

**Files:**
- Modify: `docs/security-risks.md`

- [ ] **Step 1: Verify the installed Go version**

```
go version
```

Expected output: `go version go1.25.0 linux/amd64`

- [ ] **Step 2: Update the status table in `docs/security-risks.md`**

Change the L4 row from:

```
| L4 | **LOW** | 🔴 Open | `go.mod:3` | `go 1.25.0` is a non-existent Go version |
```

to:

```
| L4 | **LOW** | ✅ Closed | `go.mod:3` | `go 1.25.0` declared — Go 1.25 released; version is valid |
```

Also update the L4 section body to add a resolution note:

```markdown
### L4 — `go 1.25.0` is a non-existent Go version

> ✅ **Closed** — Go 1.25.0 was released after the review date. The installed
> toolchain is `go1.25.0`; the directive in `go.mod` is correct. No change needed.
```

- [ ] **Step 3: Run the full test suite to confirm nothing regressed**

```
go test ./... -timeout 60s
```

Expected: all tests `PASS`

- [ ] **Step 4: Commit**

```bash
git add docs/security-risks.md
git commit -m "docs(security): close L1-L4; mark L4 as non-issue (Go 1.25 released)"
```

---

## Self-Review Checklist

- **L1 coverage:** `SanitizeRole` and `SanitizePrompt` implemented and called at `SpawnSubAgent` entry ✓. Tests cover allowed chars, stripped chars, length cap, empty fallback, and truncation.
- **L2 coverage:** `CheckPermissions` checks `0o077` mask (group + world bits). Tests cover 0600 pass, 0640 fail, 0644 fail, missing file. Wired into `main.go` as a non-fatal log warning.
- **L3 coverage:** `getUpdates` now checks status before decoding. 429 returns `*retryError` with `Retry-After` duration; 5xx returns plain error; both handled in `Poll` with correct backoff. Tests verify: no spurious updates on 429, retry after 429 (Retry-After: 0), retry after 5xx.
- **L4 coverage:** Confirmed go1.25.0 is installed; doc-only closure.
- **No placeholders:** All code blocks are complete and compilable.
- **Type consistency:** `SanitizeRole` / `SanitizePrompt` names match across test and implementation. `retryError` used consistently in `getUpdates` and `Poll`.
