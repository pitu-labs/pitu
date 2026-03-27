---
name: add-memory-backend
description: Swap the default SQLite store for a different persistence backend. Use when you need multi-node access, remote backups, or a different database engine.
---

## Add a Memory Backend

The store interface is defined in `internal/store/store.go`. To swap the backend:

1. Create a new package (e.g. `internal/store/postgres/`) that implements the same methods as `*store.Store`.
2. In `cmd/pitu/main.go`, replace `store.New(dbPath)` with your new constructor.
3. Rebuild: `go build ./cmd/pitu`.

The store methods to implement are: `SaveMessage`, `GetMessagesByChatID`, `SaveTask`, `GetTasksByChatID`, `GetAllActiveTasks`, `PauseTask`, `RegisterGroup`, `GetGroup`, `SaveSession`, `GetSession`, `Close`.
