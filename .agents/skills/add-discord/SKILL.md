---
name: add-discord
description: Add Discord as a frontend communication channel alongside or instead of Telegram. Implements the full feature set — message receive/send, typing indicator, emoji reactions, allowlist, rate limiting, and named sub-agent bubbling.
---

## Add Discord Frontend

This skill adds a Discord frontend adapter (`internal/discord/`) and wires it into the harness following the same pattern as the Telegram adapter. Work through each phase in order.

The architecture document at `docs/ARCHITECTURE.md` describes the Poller/Sender contract. Read the "Implementing a New Frontend" section before starting.

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

**2b. Relax the Telegram-only requirement in `config.Load`.**

Replace the hard `bot_token` check with a check that at least one frontend is configured:

```go
if cfg.Telegram.BotToken == "" && cfg.Discord.BotToken == "" {
    return nil, fmt.Errorf("config: at least one frontend must be configured (telegram.bot_token or discord.bot_token)")
}
```

**2c. Add the section to `config.example.toml`** after the `[telegram]` block:

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

Add the Discord frontend alongside Telegram. Both frontends share the same `q`, `mgr`, `router`, and `limiter` — the harness layers are channel-agnostic.

**6a. Import the new package:**

```go
"github.com/pitu-dev/pitu/internal/discord"
```

**6b. Construct and start after the existing Telegram block** (or replace it if running Discord-only):

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

**6c. Add the allowlist helper** (parallel to `isAllowed` for Telegram):

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

**6d. Extend the reaction callback** in `ipc.NewRouter` to route reactions to the correct sender based on which frontend owns the channel. The simplest approach: attempt Telegram first (it will return an error on a non-numeric chat ID), then attempt Discord. Or check the channel ID format: Telegram IDs are signed integers (may be negative for groups), Discord snowflakes are always large positive integers. Choose whichever approach is clearest for the fork.

---

### Phase 7 — Verify and Smoke Test

After implementing all phases, confirm the project compiles cleanly (e.g. `go build ./cmd/pitu`) and resolve any type or import errors before restarting the harness.

For a smoke test:

1. Set `discord.bot_token` in `~/.pitu/config.toml` to the token from the [Discord Developer Portal](https://discord.com/developers/applications). Invite the bot to a server with the `bot` scope and `Send Messages`, `Add Reactions`, `Read Message History` permissions.
2. Get the target channel's ID: right-click the channel → "Copy Channel ID" (Developer Mode must be on in Discord settings).
3. Add the channel ID to `discord.allowed_channel_ids` in `config.toml`.
4. Restart the harness (e.g. via `./pitu service install` or by restarting the existing service).
5. Send a message in the target Discord channel. The expected behavior: a typing indicator appears shortly after the message, followed by the agent's reply.

**If no response:**
- Review harness logs (e.g. `./pitu service logs -n 30`) for connection or dispatch errors.
- Confirm the bot has been granted the `MESSAGE CONTENT` privileged intent in the Developer Portal → Bot → Privileged Gateway Intents. Without it, `m.Content` is always empty.

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
