---
name: update-pitu
description: Update Pitu to the latest version from the repository. Use when a new release is available or after pulling security fixes.
---

## Update Pitu

1. Pull the latest changes:
   ```bash
   git pull origin main
   ```
2. Review the diff for any breaking changes in `config.example.toml` or `internal/store/store.go` (schema migrations).
3. Rebuild both binaries:
   ```bash
   go build ./cmd/pitu && go build ./cmd/pitu-mcp
   ```
4. Rebuild the container image:
   Check the current runtime in `~/.pitu/config.toml`:
   ```bash
   grep "runtime =" ~/.pitu/config.toml
   ```
   
   If **OpenCode**:
   ```bash
   podman build -t pitu-agent:latest -f container/Containerfile .
   ```
   
   If **Pi-Mono**:
   ```bash
   podman build -t pitu-pimono:latest -f container/Containerfile.pimono .
   ```
5. Restart the harness.
