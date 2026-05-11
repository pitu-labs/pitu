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

> "I can see that [summarise what you found — e.g. 'the harness currently uses a Telegram adapter' or 'no frontend is currently wired']. Should Discord run **alongside the existing frontend(s)** or **replace them entirely**?"

Record the operator's answer as either `alongside` or `replace`. Every branching note in Phase 6 refers to this recorded choice.

**Step 0c — Collect the Discord bot token.**

Ask: "Do you already have a Discord bot token for this?"

- **If yes:** collect the token now, then also ask for at least one Discord channel ID to add to the allowlist (right-click any channel in Discord → "Copy Channel ID" — requires Developer Mode to be on in Discord User Settings → Advanced). Record both and skip Step 0d entirely.
- **If no:** work through Step 0d below before continuing.

**Step 0d — Guide the operator through Discord bot setup (skip if token already in hand).**

Walk the operator through each of the following steps, waiting for confirmation at each one before moving on. Do not assume any step is already done.

1. **Create an application.**
   Go to [discord.com/developers/applications](https://discord.com/developers/applications) → click **New Application** → give it a name (e.g. "Pitú") → click **Create**.

2. **Add a bot user.**
   In the left sidebar click **Bot** → click **Add Bot** → confirm. This creates the bot identity that will connect to the Gateway.

3. **Enable the MESSAGE CONTENT privileged intent.**
   Still on the **Bot** page, scroll to **Privileged Gateway Intents** → enable **MESSAGE CONTENT INTENT** → click **Save Changes**.
   > This is the single most common failure mode. Without it, message content is always empty and the bot will silently ignore every message.

4. **Copy the bot token.**
   Click **Reset Token** (or **Copy** if visible) → copy the token and keep it somewhere safe — Discord will not show it again. This is the value for `discord.bot_token` in the Pitú config.

5. **Invite the bot to a server.**
   In the left sidebar go to **OAuth2** → **URL Generator** → under **Scopes** check `bot` → under **Bot Permissions** check **Send Messages**, **Add Reactions**, and **Read Message History** → copy the generated URL → open it in a browser → select the server to invite the bot to → click **Authorise**.
   > The bot must be in at least one server before the Gateway connection can be tested.

6. **Collect a channel ID for the allowlist.**
   In Discord, enable Developer Mode if not already active: **User Settings → Advanced → Developer Mode**. Then right-click the channel where the bot should listen → **Copy Channel ID**. Share that ID — the agent will write it directly into the config.
   > At least one channel ID is needed so messages are not silently rejected by the allowlist check.

Once the operator has confirmed the bot is in a server and has provided the token and at least one channel ID, record both and proceed to Phase 1.

---

### Phase 1 — Prerequisite: discordgo

The `discordgo` library provides the Discord Gateway client. Add it to the module:

```bash
go get github.com/bwmarrin/discordgo
```
Fetches discordgo and adds it to `go.mod` and `go.sum`.

```bash
go mod tidy
```
Removes any unused dependencies introduced by the addition.

Confirm `go.mod` lists `github.com/bwmarrin/discordgo` before continuing.

---

### Phase 2 — Config

**2a. Add the `[discord]` section to `internal/config/config.go`.**

Follow the same pattern as the existing frontend config struct (e.g. `TelegramConfig`). Add a new `DiscordConfig` struct with three fields:

- `BotToken` (string, TOML key `bot_token`): the Discord bot token from the Developer Portal.
- `AllowedChannelIDs` (slice of int64, TOML key `allowed_channel_ids`): the channel IDs permitted to send messages. An empty slice means all visible channels are accepted.
- `RateLimit` (string, TOML key `rate_limit`): minimum time between accepted messages per channel, in the same duration syntax used by the existing frontend config.

Add a `Discord DiscordConfig` field to the top-level `Config` struct alongside the existing frontend config fields.

**2b. Update the frontend validation in `config.Load`.**

Read the existing frontend token guard discovered in Phase 0. Extend it so that the check passes when at least one frontend token is non-empty. The Discord bot token should satisfy this condition in the same way the existing frontend token does. If the operator chose `replace` and the old frontend's config field is being removed, drop it from both the struct and the validation condition.

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

Create `internal/discord/types.go` with a single exported struct, `Event`, that normalises an inbound Discord `MessageCreate` event into the fields the harness needs:

- `ChannelID` (string): the Discord channel ID, used as the chat ID throughout the harness.
- `AuthorID` (string): the Discord user snowflake ID of the message author.
- `Username` (string): the display name of the message author — prefer the server nickname, fall back to the global display name, then the username.
- `Content` (string): the message text.
- `MessageID` (string): the Discord message snowflake as a string, used when routing emoji reactions.

No other types are needed in this file.

---

### Phase 4 — Poller (`internal/discord/poller.go`)

Create `internal/discord/poller.go`. The Poller wraps a `discordgo.Session` and exposes two public members: a `NewPoller(token string)` constructor and a `Poll(ctx, handler)` method. Follow the structural shape of `internal/telegram/poller.go` but note the key differences below.

**Constructor:** Create the session by calling `discordgo.New` with the token prefixed by `"Bot "` (the required Discord bot authentication prefix). Set the session's gateway intents to cover guild messages and direct messages — both are non-privileged and require no Developer Portal approval. Return an error if the session cannot be created.

**Poll method:** The method should:

1. Register a `MessageCreate` handler on the session. The handler must silently discard any event where the author is nil, is a bot (prevents response loops), or where the message content is empty. For valid events, build a `discord.Event` from the message fields — resolving the display name by preferring server nickname, then global name, then username — and call the user-supplied handler.
2. Open the gateway connection. If opening fails, log the error and return immediately.
3. Block until `ctx` is cancelled, then close the session and return.

Unlike the Telegram poller, there is no explicit reconnection loop: discordgo manages reconnection internally.

---

### Phase 5 — Sender (`internal/discord/sender.go`)

Create `internal/discord/sender.go`. The Sender wraps its own `discordgo.Session`, separate from the Poller session — discordgo's common usage pattern avoids sharing mutable session state across goroutines. The `NewSender(token string)` constructor creates and opens the session; return an error if either step fails. Expose a `Close()` method that closes the session on shutdown.

The Sender exposes the same three operations as the Telegram sender:

| Operation | Discord API call |
|-----------|-----------------|
| `SendMessage(channelID, text string)` | `session.ChannelMessageSend(channelID, text)` |
| `SendTyping(channelID string)` | `session.ChannelTyping(channelID)` |
| `ReactToMessage(channelID string, messageID int, emoji string)` | `session.MessageReactionAdd(channelID, snowflake, emoji)` |

For `ReactToMessage`: the harness `ReactionFile` carries `MessageID` as an integer; convert it to its decimal string representation (a Discord snowflake) before passing it to the API call.

**Emoji format note:** Standard Unicode emoji (e.g. `👍`) pass through as-is. Custom guild emoji use the format `name:id` (e.g. `thumbsup:123456789`).

---

### Phase 6 — Wire into `cmd/pitu/main.go`

Add the `internal/discord` package to the imports in `cmd/pitu/main.go`.

All frontends share the same queue, container manager, IPC router, and rate limiter — the harness layers are channel-agnostic.

Apply your Phase 0 findings:
- **`alongside`**: add the Discord block after any existing frontend initialization blocks and keep those blocks unchanged.
- **`replace`**: remove the existing frontend initialization block(s) and their config dependencies, then add the Discord block in their place.

**6b. Construct and start the Discord poller and sender.**

Guard the entire Discord block with a check that `cfg.Discord.BotToken` is non-empty. Inside the guard, perform these steps in order:

1. Construct the Sender; if it fails, log a fatal error.
2. Defer the Sender's `Close()` call for graceful shutdown.
3. Construct the Poller; if it fails, log a fatal error.
4. Start the Poller in a goroutine, passing a handler function that:
   - Runs the channel allowlist check (see 6c). If the channel is not allowed, log the rejection and return.
   - Runs the rate-limiter check using the shared `limiter`. If the channel is rate-limited, log it and return.
   - Calls `SendTyping` on the Sender; log but do not fatal on error.
   - Builds an `ipc.InboundMessage` with the channel ID, username, message text, and message ID from the event.
   - Saves the message to the store with the same fields as the Telegram path.
   - Ensures the memory directory exists for this chat ID, then calls `skills.WriteContext`.
   - Enqueues a `mgr.Dispatch` call on `q` using the channel ID as the queue key.

Use the analogous Telegram handler already in `main.go` as the structural template.

**6c. Add an allowlist helper.**

Add a boolean helper that accepts a channel ID string and the configured `[]int64` allowlist. It returns true immediately if the allowlist is empty (open mode). Otherwise it iterates the list, converts each int64 to its decimal string representation, and returns true on the first match. If the list is exhausted with no match, it returns false.

The Telegram adapter in `main.go` has an analogous allowlist helper — follow the same pattern.

**6d. Extend the reaction callback** in `ipc.NewRouter` to route reactions to the correct sender.

Read the existing reaction callback (found in Phase 0) to understand what senders it already calls. Extend it for Discord. In `alongside` mode, discriminate by chat ID: Discord channel IDs are always large positive integers (snowflakes ≥ 2²²; i.e. values above approximately four million), which reliably distinguishes them from Telegram chat IDs. In `replace` mode, replace the callback body entirely.

---

### Phase 7 — Configure, build, restart, and verify

Do all of the following steps yourself. Do not ask the operator to run commands or edit files manually.

**7a. Write the config.**

Read `~/.pitu/config.toml`. Add the `[discord]` section using the token and channel ID(s) collected in Phase 0 (ask now if either was deferred):

```toml
[discord]
bot_token           = "<token>"
allowed_channel_ids = [<channel_id>]   # one or more IDs collected in Phase 0
rate_limit          = "5s"
```

If the operator chose `replace` and the previous frontend's section is still present, remove or comment it out. Write the updated file back, then lock its permissions:

```bash
chmod 600 ~/.pitu/config.toml
```
Restricts the config file to owner read/write only, protecting the bot token from other users on the system.

**7b. Build.**

```bash
go build ./cmd/pitu
```
Compiles the harness binary. Fix any errors before continuing — do not proceed past a failing build.

**7c. Restart the harness.**

Check whether Pitú is running as a managed service:

```bash
systemctl --user is-active pitu
```
Prints `active` if the user-level systemd service is running, otherwise an inactive status.

- If active: restart with `systemctl --user restart pitu`.
- If installed via `./pitu service install`: run `./pitu service install` to reinstall and restart.
- If neither: start the harness directly and note the PID.

**7d. Verify the connection from logs.**

```bash
./pitu service logs -n 40
```
Tails the last 40 lines of the harness log. A clean startup will show no `discord: open:` error lines.

If the gateway fails to connect:
- Confirm the MESSAGE CONTENT privileged intent is enabled in the Discord Developer Portal → Bot → Privileged Gateway Intents. Without it, message content is always empty and the bot cannot read messages.
- Check for authentication errors (invalid token) in the logs.

**7e. Report to the operator.**

Once the logs confirm a clean startup with no Discord errors, tell the operator:

- That Discord is live, which mode is active (`alongside` or `replace`), and which channel(s) are on the allowlist.
- That to add more channels later, they can share the channel IDs and you will update `~/.pitu/config.toml` and restart the harness.

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
