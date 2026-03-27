---
name: configure-telegram
description: Configure Pitu's Telegram bot token and polling mode. Use when setting up Pitu for the first time or changing the bot token.
compatibility: Requires a valid Telegram bot token from @BotFather
---

## Configure Telegram

1. Open `~/.pitu/config.toml` (create it if it doesn't exist — copy from `config.example.toml`).
2. Set `[telegram] bot_token` to your token from @BotFather.
3. Rebuild Pitu: `go build ./cmd/pitu`.
4. Run: `pitu` — it will start polling for messages.

To verify: send `/start` to your bot on Telegram and confirm the agent responds.
