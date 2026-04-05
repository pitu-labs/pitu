---
name: setup
description: Set up Pitu from a freshly-cloned repo to a running instance. Invoke when installing Pitu for the first time.
---

## Setup Pitu

This skill walks you through a complete first-time setup. Each phase checks current state before acting — it is safe to re-run if setup was interrupted.

Work through each phase in order. Do not skip phases.

---

### Phase 1 — Prerequisites

**Check:** Run `go version` and `podman --version`. Note which are missing.

If both are present, skip to Phase 2.

**Fix:**

1. Detect the operating system:
   - Run `uname`. If output is `Darwin`, this is macOS.
   - If macOS: run `command -v brew`. Exit code 0 means Homebrew is present; non-zero means it is absent.
   - On Linux, read `/etc/os-release` and check the `ID` field.

2. Select the package manager and install only what is missing:

   | OS | Package manager | Install command |
   |---|---|---|
   | Debian / Ubuntu | apt | `sudo apt-get update && sudo apt-get install -y golang-go podman` |
   | Fedora / RHEL / CentOS | dnf | `sudo dnf install -y golang podman` |
   | Arch Linux | pacman | `sudo pacman -S --noconfirm go podman` |
   | macOS (Homebrew present) | brew | `brew install go podman` |
   | macOS (Homebrew absent) | — | Ask the user: "Homebrew is not installed. Shall I install it on your behalf?" If yes: `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"` then `brew install go podman`. If no: tell the user to install Go (`https://go.dev/doc/install`) and Podman (`https://podman.io/docs/installation`) via their preferred method, then stop. |
   | Unrecognised | — | Report which tools are missing. Link to `https://go.dev/doc/install` and `https://podman.io/docs/installation`. Stop. |

**Verify:** Run `go version && podman --version`. Both must succeed before continuing.

---

### Phase 2 — Build

**Check:** Test whether both `./pitu` and `./pitu-mcp` exist in the project root.

If both exist, skip to Phase 3.

**Fix:**

```bash
go build ./cmd/pitu && go build ./cmd/pitu-mcp
```

**Verify:** Both commands exited 0 and both `./pitu` and `./pitu-mcp` are present in the project root.

---

### Phase 3 — Config

**Check:** Test whether `~/.pitu/config.toml` exists.

If it exists, skip to Phase 4.

**Fix:**

```bash
mkdir -p ~/.pitu && cp config.example.toml ~/.pitu/config.toml
```

**Verify:** `~/.pitu/config.toml` exists and contains `[telegram]`, `[container]`, and `[db]` sections.

---

### Phase 3.5 — Runtime Preference

Ask the user: "Which agent runtime would you like to use?
1. **OpenCode** (Traditional, Go-based bridge, Bun runtime)
2. **Pi-Mono** (Minimalist, TypeScript-native, JSONL auditing)"

Based on their choice, update `~/.pitu/config.toml`:

If **OpenCode**:
```bash
sed -i 's/runtime = .*/runtime = "opencode"/' ~/.pitu/config.toml
sed -i 's/image = .*/image = "pitu-agent:latest"/' ~/.pitu/config.toml
```

If **Pi-Mono**:
```bash
sed -i 's/runtime = .*/runtime = "pimono"/' ~/.pitu/config.toml
sed -i 's/image = .*/image = "pitu-pimono:latest"/' ~/.pitu/config.toml
```

**Verify:** `grep "runtime =" ~/.pitu/config.toml` shows the selected value.

---

### Phase 4 — Telegram

Invoke the `/configure-telegram` skill now. It will guide you through setting the bot token.

Return here once `/configure-telegram` completes successfully. Before continuing to Phase 5, confirm that `~/.pitu/config.toml` contains a non-empty `bot_token` value under `[telegram]`:

```bash
grep "bot_token" ~/.pitu/config.toml
```

The output must show a non-empty token value. If `bot_token` is still `"YOUR_BOT_TOKEN_HERE"` or empty, re-run `/configure-telegram` before proceeding.

---

### Phase 4.5 — Model Configuration

Invoke the `/update-model` skill now. It will guide you through selecting a provider, entering an API key (or Ollama base URL), and writing the `[model]` section to `~/.pitu/config.toml`.

Return here once `/update-model` completes. Before continuing to Phase 5, confirm the config contains a non-empty `provider` and `model`:

```bash
grep -A4 '\[model\]' ~/.pitu/config.toml
```

The output must show non-empty `provider` and `model` values. If they are missing or set to placeholder values, re-run `/update-model` before proceeding.

---

### Phase 5 — Container Image

**Check:** Determine which image to check based on `~/.pitu/config.toml`.

```bash
grep "runtime =" ~/.pitu/config.toml
```

If runtime is `"pimono"`, check `pitu-pimono:latest`. Otherwise check `pitu-agent:latest`.

```bash
podman image exists <image-name>
```

If the image is present, skip to Phase 6.

**Fix:**

If **OpenCode**:
```bash
podman build -t pitu-agent:latest -f container/Containerfile .
```

If **Pi-Mono**:
```bash
podman build -t pitu-pimono:latest -f container/Containerfile.pimono .
```

This may take several minutes on the first run as base layers are downloaded.

**Verify:** `podman images <image-name>` shows the image with a recent creation timestamp.

---

### Phase 5.5 — Agent Personalisation (optional)

This phase is optional. If the user wants to skip it, proceed directly to Phase 6.

Ask the user: "Would you like to personalise your agent now? You can skip this and do it later. (yes / skip)". If they say skip or no, go to Phase 6.

Ask the user each of the following questions in order. If they skip a question (say "skip", "no", or leave it blank), do not create that file.

**Question 1:** "What would you like your agent to be called, and what role should it play? For example: 'You are Aria, a focused assistant for software projects.'"

If answered: note the content for `~/.pitu/agent/IDENTITY.md`.

**Question 2:** "How should your agent behave? Describe its personality, tone, and any hard limits. For example: 'Direct and friendly. Never discuss competitors. Keep answers concise.'"

If answered: note the content for `~/.pitu/agent/SOUL.md`.

**Question 3:** "Tell me about yourself — your name, timezone, and how you prefer to work. For example: 'I'm Rob, UTC-5, I prefer short replies and bullet points.'"

If answered: note the content for `~/.pitu/agent/USER.md`.

After handling all three questions:

```bash
mkdir -p "$HOME/.pitu/agent"
```

Write each answered file using the Write tool with the absolute path (e.g. `/home/username/.pitu/agent/IDENTITY.md`). Resolve the home directory first:

```bash
echo "$HOME"
```

Then write each answered file. For example, if `$HOME` is `/home/rob` and the user answered Question 1 with "You are Aria, a focused assistant.":

```bash
cat > /home/rob/.pitu/agent/IDENTITY.md << 'EOF'
You are Aria, a focused assistant.
EOF
```

**IMPORTANT:** Always use the absolute path (never `~` or `$HOME` in heredocs), because `~` is not expanded by all tools and may create a literal `~/` directory instead of writing to the user's home directory.

**Verify:** List the files that were created:

```bash
ls "$HOME/.pitu/agent/"
```

Tell the user which files were created and that changes will take effect for new chats. Existing chats can be refreshed by deleting `$HOME/.pitu/data/{chatID}/memory/AGENTS.md`.

---

### Phase 6 — Smoke Test

1. Install and start the harness as a system service:

   ```bash
   ./pitu service install
   ```

   This registers pitu with the OS service manager (systemd on Linux, launchd on macOS),
   starts it immediately, and enables autostart on boot. The service will restart automatically
   if it crashes.

2. Tell the user: "Please send `/start` to your Telegram bot now."

3. Wait up to 30 seconds for a response to appear in Telegram.

4. **If the bot responds:** setup is complete. Summarise what was installed and configured during this session.

5. **If no response arrives within 30 seconds:**
   - Show the last 20 lines of the service log: `./pitu service logs -n 20`
   - Check service status: `./pitu service status`
   - Ask the user to verify their bot token in `~/.pitu/config.toml`.
   - Suggest re-running `/configure-telegram` if the token looks wrong.
