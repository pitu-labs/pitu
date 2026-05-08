---
name: add-discord
description: Add Discord as a frontend communication channel. Implements the full feature set — message receive/send, typing indicator, emoji reactions, allowlist, rate limiting, and named sub-agent bubbling. Can run alongside any existing frontend or replace it.
---

## Add Discord Frontend

This skill adds a Discord frontend adapter (`internal/discord/`) and wires it into the harness following the same pattern as the Telegram adapter. Work through each phase in order.

The architecture document at `docs/ARCHITECTURE.md` describes the Poller/Sender contract. Read the "Implementing a New Frontend" section before starting.

---

### Phase 0 — Inspect the current frontend wiring, then clarify deployment mode (REQUIRED before any code changes)

**Step 0a — Read the codebase first.**

Before asking anything, read `cmd/pitu/main.go` and `internal/config/config.go` to discover:

- Which frontend(s) are currently wired (look for `Poller` and `Sender` construction, and the corresponding config structs).
- How the existing config validation works (what tokens or credentials are required today).
- Whether any allowlist or rate-limiting pattern already exists, and what it looks like.

Record your findings. They will directly shape how you adapt Phases 2, 6, and 7.

**Step 0b — Ask the operator one question and wait for a reply before writing any code:**

> "I can see that [summarise what you found — e.g. 'the harness currently uses a custom Slack adapter' or 'no frontend is currently wired']. Should Discord run **alongside the existing frontend(s)** or **replace them entirely**?"

Record the operator's answer as either `alongside` or `replace`. Every branching note in Phase 6 refers to this recorded choice.

**Step 0c — Collect the Discord bot token.**

Also ask for the **Discord bot token** at this point if the operator has it available, so you can write the config in Phase 7 without another interruption. If they don't have it yet, proceed without it — Phase 7 will prompt again.

---

### Phase 1 — Prerequisite: discordgo

The `discordgo` library provides the Discord Gateway client. Add it to the module — for example:

```bash
go get github.com/bwmarrin/discordgo
go mod tidy
```

Confirm `go.mod` lists `github.com/bwmarrin/discordgo` before continuing.

---

### Phase 2 — Config

**2a. Add the `[discord]` section to `internal/config/config.go`.**

Add a new struct and field to `Config`:

```go
type DiscordConfig struct {
    BotToken          string   `toml:"bot_token"`
    AllowedChannelIDs []int64  `toml:"allowed_channel_ids"`
    RateLimit         string   `toml:"rate_limit"`
}

type Config struct {
    Telegram  TelegramConfig  `toml:"telegram"`
    Discord   DiscordConfig   `toml:"discord"`
    Container ContainerConfig `toml:"container"`
    Skills    SkillsConfig    `toml:"skills"`
    DB        DBConfig        `toml:"db"`
    Model     ModelConfig     `toml:"model"`
}
```

**2b. Update the frontend validation in `config.Load`.**

Read the existing validation logic (discovered in Phase 0) and extend it to include `cfg.Discord.BotToken`. The goal is to ensure at least one frontend is configured; the exact condition depends on what frontends are currently wired.

For example, if the current check requires only one specific frontend's token, broaden it to an OR across all known frontends:

```go
// Adapt this condition to include every frontend config field present in the struct.
if cfg.Discord.BotToken == "" /* && cfg.<OtherFrontend>.Token == "" ... */ {
    return nil, fmt.Errorf("config: at least one frontend must be configured")
}
```

If the operator chose `replace` in Phase 0 and the old frontend's config field is being removed, drop it from both the struct and this condition.

**2c. Add the section to `config.example.toml`** after any existing frontend config blocks:

```toml
[discord]
bot_token           = ""               # Discord bot token from the Developer Portal
# Allowlist of Discord channel IDs permitted to send messages.
# If empty, all channels the bot can see are accepted.
allowed_channel_ids = []
# Minimum time between accepted messages per channel. Same syntax as Telegram.
rate_limit          = "5s"
```

---

### Phase 3 — Types (`internal/discord/types.go`)

Create `internal/discord/types.go`. Discord Gateway events arrive pre-parsed by discordgo. Define only the minimal internal types needed to bridge into the harness:

```go
package discord

// Event is an inbound message normalised from a discordgo MessageCreate event.
type Event struct {
    ChannelID string // maps to ChatID in the harness
    AuthorID  string
    Username  string
    Content   string
    MessageID string // Discord snowflake as string
}
```

---

### Phase 4 — Poller (`internal/discord/poller.go`)

Discord uses a WebSocket Gateway instead of HTTP long-polling. The `Poller.Poll` method:

1. Opens the gateway connection with `session.Open()`.
2. Registers a `MessageCreate` handler that normalises events and calls the user-supplied handler.
3. Blocks on `session.Wait()` (returns when the context is cancelled or the connection closes).
4. Calls `session.Close()` on exit.

Unlike the Telegram poller, there is no explicit backoff loop: discordgo handles reconnects internally. The Poll method should respect context cancellation by running `session.Close()` in a goroutine triggered by `ctx.Done()`.

```go
package discord

import (
    "context"
    "log"

    "github.com/bwmarrin/discordgo"
)

type Poller struct {
    session *discordgo.Session
}

func NewPoller(token string) (*Poller, error) {
    s, err := discordgo.New("Bot " + token)
    if err != nil {
        return nil, err
    }
    s.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages
    return &Poller{session: s}, nil
}

// Poll calls handler for each inbound text message until ctx is cancelled.
func (p *Poller) Poll(ctx context.Context, handler func(Event)) {
    p.session.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
        if m.Author == nil || m.Author.Bot {
            return // ignore bots (prevents loops)
        }
        if m.Content == "" {
            return
        }
        handler(Event{
            ChannelID: m.ChannelID,
            AuthorID:  m.Author.ID,
            Username:  displayName(m),
            Content:   m.Content,
            MessageID: m.ID,
        })
    })
    if err := p.session.Open(); err != nil {
        log.Printf("discord: open: %v", err)
        return
    }
    defer p.session.Close()
    <-ctx.Done()
}

// displayName prefers the server nickname, falls back to GlobalName, then Username.
func displayName(m *discordgo.MessageCreate) string {
    if m.Member != nil && m.Member.Nick != "" {
        return m.Member.Nick
    }
    if m.Author.GlobalName != "" {
        return m.Author.GlobalName
    }
    return m.Author.Username
}
```

**Bot intents note:** `IntentsGuildMessages` covers server channels; `IntentsDirectMessages` covers DMs. If the bot should only handle DMs, remove `IntentsGuildMessages`. Both intents are non-privileged and do not require approval in the Developer Portal.

---

### Phase 5 — Sender (`internal/discord/sender.go`)

The Sender exposes the same three operations as the Telegram sender:

| Operation | Discord API call |
|-----------|-----------------|
| `SendMessage(channelID, text)` | `session.ChannelMessageSend(channelID, text)` |
| `SendTyping(channelID)` | `session.ChannelTyping(channelID)` |
| `ReactToMessage(channelID, messageID, emoji)` | `session.MessageReactionAdd(channelID, messageID, emoji)` |

**Reaction note:** Standard Unicode emoji (e.g. `👍`) are passed as-is. Custom guild emoji use the format `name:id` (e.g. `thumbsup:123456789`). The harness ReactionFile carries `MessageID int`; Discord snowflakes fit in a 64-bit integer, so convert with `strconv.FormatInt(int64(rf.MessageID), 10)` when calling the sender.

```go
package discord

import (
    "fmt"
    "strconv"

    "github.com/bwmarrin/discordgo"
)

type Sender struct {
    session *discordgo.Session
}

func NewSender(token string) (*Sender, error) {
    s, err := discordgo.New("Bot " + token)
    if err != nil {
        return nil, err
    }
    if err := s.Open(); err != nil {
        return nil, fmt.Errorf("discord: sender open: %w", err)
    }
    return &Sender{session: s}, nil
}

func (s *Sender) SendMessage(channelID, text string) error {
    _, err := s.session.ChannelMessageSend(channelID, text)
    return err
}

func (s *Sender) SendTyping(channelID string) error {
    return s.session.ChannelTyping(channelID)
}

// ReactToMessage converts the harness int messageID to a Discord snowflake string.
func (s *Sender) ReactToMessage(channelID string, messageID int, emoji string) error {
    return s.session.MessageReactionAdd(channelID, strconv.FormatInt(int64(messageID), 10), emoji)
}

func (s *Sender) Close() {
    s.session.Close()
}
```

**Two sessions note:** The Poller and Sender each open a separate gateway session. This matches discordgo's common usage pattern and avoids sharing mutable session state across goroutines. Both must be closed on shutdown.

---

### Phase 6 — Wire into `cmd/pitu/main.go`

All frontends share the same `q`, `mgr`, `router`, and `limiter` — the harness layers are channel-agnostic.

Apply your Phase 0 findings here:
- **`alongside`**: add the Discord block after any existing frontend initialization blocks and keep those blocks unchanged.
- **`replace`**: remove (or comment out) the existing frontend initialization block(s) and their config dependencies, then add the Discord block in their place. Also remove the now-unused config struct fields and imports.

**6a. Import the new package:**

```go
"github.com/pitu-dev/pitu/internal/discord"
```

**6b. Construct and start the Discord poller and sender.**

The pattern below is a reference — adapt variable names and surrounding code to match what is already in `main.go`:

```go
if cfg.Discord.BotToken != "" {
    discordSender, err := discord.NewSender(cfg.Discord.BotToken)
    if err != nil {
        log.Fatalf("pitu: discord sender: %v", err)
    }
    defer discordSender.Close()

    discordPoller, err := discord.NewPoller(cfg.Discord.BotToken)
    if err != nil {
        log.Fatalf("pitu: discord poller: %v", err)
    }

    // Wire Discord reactions into the IPC router's reaction callback.
    // The existing router reaction callback calls sender.ReactToMessage;
    // extend it to also call discordSender.ReactToMessage when the message
    // originates from Discord. Simplest approach: replace the callback to
    // try both senders and log only unexpected errors.

    go discordPoller.Poll(ctx, func(e discord.Event) {
        if !isAllowedDiscord(e.ChannelID, cfg.Discord.AllowedChannelIDs) {
            log.Printf("pitu: discord: rejected channel %s (not in allowlist)", e.ChannelID)
            return
        }
        if !limiter.Allow(e.ChannelID) {
            log.Printf("pitu: discord: rate-limited channel %s", e.ChannelID)
            return
        }
        if err := discordSender.SendTyping(e.ChannelID); err != nil {
            log.Printf("pitu: discord: typing: %v", err)
        }
        msg := ipc.InboundMessage{
            ChatID:    e.ChannelID,
            From:      e.Username,
            Text:      e.Content,
            MessageID: e.MessageID,
        }
        st.SaveMessage(store.Message{
            ChatID:    e.ChannelID,
            FromUser:  e.Username,
            Text:      e.Content,
            MessageID: e.MessageID,
            CreatedAt: time.Now().UTC(),
        })
        memDir := filepath.Join(dataDir, e.ChannelID, "memory")
        os.MkdirAll(memDir, 0700)
        skills.WriteContext(memDir, e.ChannelID, discovered, agentCfg)

        q.Enqueue(e.ChannelID, func() {
            if err := mgr.Dispatch(ctx, e.ChannelID, msg); err != nil {
                log.Printf("pitu: discord: dispatch %s: %v", e.ChannelID, err)
            }
        })
    })
}
```

**6c. Add the allowlist helper** (or adapt the existing one if a similar pattern is already present):

```go
func isAllowedDiscord(channelID string, allowed []int64) bool {
    if len(allowed) == 0 {
        return true
    }
    for _, id := range allowed {
        if strconv.FormatInt(id, 10) == channelID {
            return true
        }
    }
    return false
}
```

**6d. Extend the reaction callback** in `ipc.NewRouter` to route reactions to the correct sender.

Read the existing reaction callback (found in Phase 0) to understand what senders it already calls. Then extend it for Discord.

If multiple frontends are active (`alongside` mode), the callback must route to the right sender. A reliable discriminator: Discord channel IDs are always large positive integers (snowflakes ≥ 2^22, i.e. > 4 million); adapt this or any other discriminator that matches what the existing frontends use for their chat IDs. If only Discord is active (`replace` mode), replace the callback body entirely.

---

### Phase 7 — Configure, build, restart, and verify

Do all of the following steps yourself. Do not ask the operator to run commands or edit files manually.

**7a. Write the config.**

Read `~/.pitu/config.toml`. Add the `[discord]` section (using the token collected in Phase 0, or ask for it now if it was deferred):

```toml
[discord]
bot_token           = "<token>"
allowed_channel_ids = []
rate_limit          = "5s"
```

If the operator chose `replace` in Phase 0 and the previous frontend's section is still present, remove or comment it out. Write the updated file back.

**7b. Build.**

```bash
go build ./cmd/pitu
```

If the build fails, fix the errors and rebuild before continuing. Do not proceed past a failing build.

**7c. Restart the harness.**

Determine whether Pitú is running as a managed service or as a standalone process:

```bash
# Check for a managed service (Linux systemd)
systemctl --user is-active pitu 2>/dev/null || systemctl is-active pitu 2>/dev/null
```

- If a systemd service is active: `systemctl --user restart pitu || sudo systemctl restart pitu`
- If Pitú was installed via `./pitu service install`: `./pitu service install` (reinstalls and restarts)
- If neither: start the harness directly in the background and note the PID

**7d. Verify the connection from logs.**

Wait a few seconds, then check the logs for a successful Discord gateway connection:

```bash
./pitu service logs -n 40
```

Look for a log line indicating the Discord gateway connected (e.g. no `discord: open:` error lines). If the harness is not managed by the service subcommand, tail the process output directly.

**If the gateway fails to connect:**
- Confirm `MESSAGE CONTENT` privileged intent is enabled in the Discord Developer Portal → Bot → Privileged Gateway Intents. Without it, `m.Content` is always empty and the bot cannot read messages.
- Check for authentication errors (invalid token) in the logs.

**7e. Report to the operator.**

Once the logs confirm a clean startup with no Discord errors, tell the operator:

- That Discord is live and which mode is active (`alongside` or `replace`).
- How to get a channel ID for the allowlist: right-click a channel in Discord → "Copy Channel ID" (Developer Mode must be enabled in Discord User Settings → Advanced).
- That they can add channel IDs to `discord.allowed_channel_ids` in `~/.pitu/config.toml` at any time and restart to apply.

---

### Feature parity checklist

| Feature | Telegram | Discord |
|---------|----------|---------|
| Receive text messages | `getUpdates` long-poll | Gateway `MessageCreate` event |
| Send text messages | `sendMessage` | `ChannelMessageSend` |
| Typing indicator | `sendChatAction "typing"` | `ChannelTyping` |
| Emoji reactions | `setMessageReaction` | `MessageReactionAdd` |
| Allowlist | `allowed_chat_ids` | `allowed_channel_ids` |
| Rate limiting | per chat ID | per channel ID (same `ratelimit.Limiter`) |
| Sub-agent bubbling | parent agent re-dispatch | same — channel ID is the routing key |
| Named sub-agents | `From: "Agent: <role>"` | same field in `InboundMessage` |
