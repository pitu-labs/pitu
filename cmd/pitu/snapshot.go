package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pitu-dev/pitu/internal/store"
)

type tasksSnapshot struct {
	UpdatedAt string       `json:"updated_at"`
	Tasks     []store.Task `json:"tasks"`
}

// writeTasksSnapshot reads the current tasks for chatID from SQLite and writes
// an atomic snapshot to <dataDir>/<chatID>/memory/tasks.json. The file is
// visible to the agent at /workspace/memory/tasks.json. Errors are non-fatal —
// the snapshot may be stale but SQLite remains authoritative.
func writeTasksSnapshot(st *store.Store, dataDir, chatID string) error {
	tasks, err := st.GetTasksByChatID(chatID)
	if err != nil {
		return fmt.Errorf("writeTasksSnapshot: query: %w", err)
	}
	if tasks == nil {
		tasks = []store.Task{}
	}
	snap := tasksSnapshot{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Tasks:     tasks,
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("writeTasksSnapshot: marshal: %w", err)
	}
	memDir := filepath.Join(dataDir, chatID, "memory")
	if err := os.MkdirAll(memDir, 0700); err != nil {
		return fmt.Errorf("writeTasksSnapshot: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(memDir, ".tasks-*")
	if err != nil {
		return fmt.Errorf("writeTasksSnapshot: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writeTasksSnapshot: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("writeTasksSnapshot: close: %w", err)
	}
	dest := filepath.Join(memDir, "tasks.json")
	if err := os.Rename(tmpPath, dest); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("writeTasksSnapshot: rename: %w", err)
	}
	return nil
}
