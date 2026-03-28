---
name: uninstall-pitu
description: Completely remove a running Pitu installation — service, runtime state, binaries, and containers — while leaving the source repository intact. Use when decommissioning Pitu or resetting to a clean slate.
---

## Uninstall Pitu

Works through each layer from outermost (service) to innermost (images). Each step is safe to run even if that layer is already absent.

---

### Step 1 — Stop and remove the OS service

```bash
./pitu service uninstall
```

On Linux this disables and removes the systemd user unit. On macOS it unloads and removes the launchd plist. If pitu was never installed as a service, this prints "not installed — nothing to do" and exits cleanly.

---

### Step 2 — Remove runtime state

```bash
rm -rf ~/.pitu
```

This removes all of: `config.toml`, the SQLite database, memory directories, logs, and any user-installed skills. **This step is irreversible.** If you want to keep your config or memory, back up `~/.pitu` first.

---

### Step 3 — Remove the built binaries

From the repository root:

```bash
rm -f pitu pitu-mcp
```

---

### Step 4 — Stop and remove Podman containers

List any running or stopped pitu containers:

```bash
podman ps -a --filter name=pitu
```

Stop and remove each one shown (replace `<id>` with the actual container ID or name):

```bash
podman rm -f <id>
```

Or remove all pitu containers in one step:

```bash
podman ps -a --filter name=pitu --format '{{.ID}}' | xargs -r podman rm -f
```

---

### Step 5 — Remove the container image (optional)

If you no longer need the agent image (a fresh `go build` + `podman build` will recreate it):

```bash
podman rmi pitu-agent:latest
```

---

### Verification

Confirm nothing is left running:

```bash
./pitu service status 2>/dev/null || echo "binaries removed"
podman ps -a --filter name=pitu
```

The source repository and your git history are untouched. To set up again from scratch, run `/setup`.
