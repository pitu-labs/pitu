---
name: view-active-sessions
description: List currently warm agent containers and their last activity time. Use when diagnosing memory usage or checking which chats are active.
---

## View Active Sessions

```bash
podman ps --format "table {{.ID}}\t{{.Names}}\t{{.Status}}"
```

Containers are named by their Podman-assigned name. To correlate with a chat ID, check the SQLite database:

```bash
sqlite3 ~/.pitu/pitu.db "SELECT chat_id, updated_at FROM sessions ORDER BY updated_at DESC LIMIT 10;"
```
