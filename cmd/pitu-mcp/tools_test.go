package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pitu-dev/pitu/internal/ipc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleSendMessage_WritesFile(t *testing.T) {
	tmp := t.TempDir()
	for _, sub := range []string{"messages", "tasks", "groups"} {
		os.MkdirAll(filepath.Join(tmp, sub), 0755)
	}

	h := &toolHandlers{
		ipcDir:     tmp,
		chatID:     "chat-55",
		role:       "researcher",
		subAgentID: "agent-123",
	}
	result, err := h.handleSendMessage("Hello", "alice")
	require.NoError(t, err)
	assert.Contains(t, result, "ok")

	entries, _ := os.ReadDir(filepath.Join(tmp, "messages"))
	require.Len(t, entries, 1)
	data, _ := os.ReadFile(filepath.Join(tmp, "messages", entries[0].Name()))
	var msg ipc.OutboundMessage
	require.NoError(t, json.Unmarshal(data, &msg))
	assert.Equal(t, "Hello", msg.Text)
	assert.Equal(t, "chat-55", msg.ChatID)
	assert.Equal(t, "researcher", msg.Role)
	assert.Equal(t, "agent-123", msg.SubAgentID)
}

func TestHandleScheduleTask_ReturnsUUID(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "tasks"), 0755)

	h := &toolHandlers{ipcDir: tmp, chatID: "c1"}
	result, err := h.handleScheduleTask("daily", "0 9 * * *", "summarise")
	require.NoError(t, err)
	assert.Contains(t, result, `"id"`)

	entries, _ := os.ReadDir(filepath.Join(tmp, "tasks"))
	require.Len(t, entries, 1)
}

func TestHandleListTasks_WritesFile(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "tasks"), 0755)

	h := &toolHandlers{ipcDir: tmp, chatID: "c2"}
	_, err := h.handleListTasks()
	require.NoError(t, err)

	entries, _ := os.ReadDir(filepath.Join(tmp, "tasks"))
	require.Len(t, entries, 1)
	data, _ := os.ReadFile(filepath.Join(tmp, "tasks", entries[0].Name()))
	var tf ipc.TaskFile
	require.NoError(t, json.Unmarshal(data, &tf))
	assert.Equal(t, "list", tf.Action)
}

func TestHandleSpawnAgent_WritesAgentFile(t *testing.T) {
	tmp := t.TempDir()
	for _, sub := range []string{"messages", "tasks", "groups", "agents"} {
		os.MkdirAll(filepath.Join(tmp, sub), 0755)
	}

	h := &toolHandlers{ipcDir: tmp, chatID: "chat-1"}
	result, err := h.handleSpawnAgent("Researcher", "find papers on Go concurrency")
	require.NoError(t, err)
	assert.Contains(t, result, `"subAgentId"`)

	entries, _ := os.ReadDir(filepath.Join(tmp, "agents"))
	require.Len(t, entries, 1)
	data, _ := os.ReadFile(filepath.Join(tmp, "agents", entries[0].Name()))
	var af ipc.AgentFile
	require.NoError(t, json.Unmarshal(data, &af))
	assert.Equal(t, "spawn", af.Action)
	assert.Equal(t, "Researcher", af.Role)
	assert.Equal(t, "find papers on Go concurrency", af.Prompt)
	assert.Equal(t, "chat-1", af.ChatID)
	assert.NotEmpty(t, af.SubAgentID)
}
