package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pitu-dev/pitu/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestWriteTasksSnapshot_CreatesFile(t *testing.T) {
	st := openTestStore(t)
	chatID := "chat-99"
	task := store.Task{ID: "t1", ChatID: chatID, Name: "daily", Schedule: "0 9 * * *", Prompt: "run"}
	require.NoError(t, st.SaveTask(task))

	dataDir := t.TempDir()
	require.NoError(t, writeTasksSnapshot(st, dataDir, chatID))

	snapPath := filepath.Join(dataDir, chatID, "memory", "tasks.json")
	data, err := os.ReadFile(snapPath)
	require.NoError(t, err)

	var snap tasksSnapshot
	require.NoError(t, json.Unmarshal(data, &snap))
	require.Len(t, snap.Tasks, 1)
	assert.Equal(t, "daily", snap.Tasks[0].Name)
	assert.NotEmpty(t, snap.UpdatedAt)
}

func TestWriteTasksSnapshot_EmptyTasksIsArray(t *testing.T) {
	st := openTestStore(t)
	dataDir := t.TempDir()
	require.NoError(t, writeTasksSnapshot(st, dataDir, "chat-empty"))

	snapPath := filepath.Join(dataDir, "chat-empty", "memory", "tasks.json")
	data, err := os.ReadFile(snapPath)
	require.NoError(t, err)

	var snap tasksSnapshot
	require.NoError(t, json.Unmarshal(data, &snap))
	// tasks must be [] not null — agent must never see a missing key
	assert.NotNil(t, snap.Tasks)
	assert.Empty(t, snap.Tasks)
}

func TestWriteTasksSnapshot_CreatesMemoryDir(t *testing.T) {
	st := openTestStore(t)
	dataDir := t.TempDir()
	chatID := "new-chat"

	// memory dir does not exist yet — writeTasksSnapshot must create it
	memDir := filepath.Join(dataDir, chatID, "memory")
	_, err := os.Stat(memDir)
	require.True(t, os.IsNotExist(err))

	require.NoError(t, writeTasksSnapshot(st, dataDir, chatID))

	_, err = os.Stat(memDir)
	assert.NoError(t, err, "memory dir should have been created")
}

func TestWriteTasksSnapshot_IsAtomic(t *testing.T) {
	st := openTestStore(t)
	dataDir := t.TempDir()
	chatID := "chat-atomic"

	// Write twice — second write must overwrite cleanly with no temp file left behind
	require.NoError(t, writeTasksSnapshot(st, dataDir, chatID))
	require.NoError(t, writeTasksSnapshot(st, dataDir, chatID))

	memDir := filepath.Join(dataDir, chatID, "memory")
	entries, err := os.ReadDir(memDir)
	require.NoError(t, err)
	// Only tasks.json should remain — no leftover .tasks-* temp files
	assert.Len(t, entries, 1)
	assert.Equal(t, "tasks.json", entries[0].Name())
}
