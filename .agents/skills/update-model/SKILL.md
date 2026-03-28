---
name: update-model
description: Configure the AI provider and model Pitu uses inside its agent containers. Invoke to set or change the provider, model name, and API key in ~/.pitu/config.toml.
---

## Update Pitu Model Configuration

This skill updates the `[model]` section of `~/.pitu/config.toml`. It is safe to re-run — it overwrites the existing model section each time.

---

### Phase 1 — Read current config

Run:

```bash
cat ~/.pitu/config.toml
```

Note the current `[model]` values if present. Show them to the user before asking for changes.

---

### Phase 2 — Choose provider

Ask the user which provider to use. Present these options:

1. **Anthropic** — Claude models (claude-sonnet-4-5, claude-opus-4-5, claude-haiku-4-5, etc.)
2. **OpenAI** — GPT models (gpt-4o, gpt-4o-mini, o1, etc.)
3. **Ollama** — Local models, no API key required (llama3, mistral, gemma3, etc.)
4. **Other (OpenAI-compatible)** — Groq, Together AI, or any provider with an OpenAI-compatible API

Wait for the user's answer before continuing.

---

### Phase 3 — Collect settings

Based on the provider chosen in Phase 2, ask for the following. Ask one question at a time.

**If Anthropic:**
- API key (from https://console.anthropic.com/settings/keys)
- Model name — suggest `claude-sonnet-4-5` as default; accept any valid model name

Set `base_url = ""`.

**If OpenAI:**
- API key (from https://platform.openai.com/api-keys)
- Model name — suggest `gpt-4o` as default; accept any valid model name

Set `base_url = ""`.

**If Ollama:**
- Base URL — suggest `http://localhost:11434/v1` as default
- Model name (must already be pulled on the host, e.g. `llama3`)

Set `api_key = ""`.

**If Other:**
- Provider ID — a short identifier used internally (e.g. `groq`, `together`). This becomes the key in the OpenCode `provider` object and the prefix in the model string.
- Base URL for the API endpoint (e.g. `https://api.groq.com/openai/v1`)
- API key
- Model name (e.g. `llama-3.3-70b-versatile`)

---

### Phase 4 — Write config

Read the current `~/.pitu/config.toml` content. Remove any existing `[model]` block (from the `[model]` line through the last key=value line in that section). Append the new `[model]` block at the end of the file.

The new block must look exactly like this (substitute the collected values):

```toml
[model]
provider = "<provider>"
model    = "<model>"
api_key  = "<api_key>"
base_url = "<base_url>"
```

Leave `api_key = ""` for Ollama. Leave `base_url = ""` for Anthropic and OpenAI.

**Verify:** Run `cat ~/.pitu/config.toml` and confirm the `[model]` section shows the new values.

---

### Phase 5 — Confirm

Tell the user:

> "Model configuration updated. Pitu will use `<provider>/<model>` for all agent containers. Restart the harness (`pkill pitu && ./pitu`) for the change to take effect on new containers."

If the harness is not running, omit the restart instruction.
