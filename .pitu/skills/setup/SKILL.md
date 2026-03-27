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

**Verify:** Both commands exited 0 and `./pitu` is present in the project root.

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

### Phase 4 — Telegram

Invoke the `/configure-telegram` skill now. It will guide you through setting the bot token and verifying the bot is reachable.

Return here once `/configure-telegram` completes successfully. Before continuing to Phase 5, confirm that `~/.pitu/config.toml` contains a non-empty `bot_token` value under `[telegram]`:

```bash
grep "bot_token" ~/.pitu/config.toml
```

The output must show a non-empty token value. If `bot_token` is still `"YOUR_BOT_TOKEN_HERE"` or empty, re-run `/configure-telegram` before proceeding.

---

### Phase 5 — Container Image

**Check:** Run `podman image exists pitu-agent:latest`. Exit code 0 means the image is present.

If the image is present, skip to Phase 6.

**Fix:**

```bash
podman build -t pitu-agent:latest -f container/Containerfile .
```

This may take several minutes on the first run as base layers are downloaded.

**Verify:** `podman images pitu-agent:latest` shows the image with a recent creation timestamp.

---

### Phase 6 — Smoke Test

1. Start the harness:

   ```bash
   ./pitu
   ```

2. Tell the user: "Please send `/start` to your Telegram bot now."

3. Wait up to 30 seconds for a response to appear in Telegram.

4. **If the bot responds:** setup is complete. Summarise what was installed and configured during this session.

5. **If no response arrives within 30 seconds:**
   - Show the last 20 lines of `pitu` output.
   - Ask the user to verify their bot token in `~/.pitu/config.toml`.
   - Suggest re-running `/configure-telegram` if the token looks wrong.
