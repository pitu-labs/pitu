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

func TestHandleListTasks_ReturnsPath(t *testing.T) {
	h := &toolHandlers{ipcDir: "", chatID: "c2"}
	result, err := h.handleListTasks()
	require.NoError(t, err)
	assert.Equal(t, `{"path":"/workspace/memory/tasks.json"}`, result)
}

func TestHandleSendMessage_SecondCallRejected(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "messages"), 0755))

	h := &toolHandlers{ipcDir: tmp, chatID: "chat-1"}
	_, err := h.handleSendMessage("first", "")
	require.NoError(t, err)

	_, err = h.handleSendMessage("second", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already called")

	entries, _ := os.ReadDir(filepath.Join(tmp, "messages"))
	require.Len(t, entries, 1, "second call must not write a file")
}

func TestHandleSendMessage_IndependentBetweenHandlers(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(a, "messages"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(b, "messages"), 0755))

	ha := &toolHandlers{ipcDir: a, chatID: "chat-a"}
	hb := &toolHandlers{ipcDir: b, chatID: "chat-b"}

	_, err := ha.handleSendMessage("hi from a", "")
	require.NoError(t, err)
	_, err = hb.handleSendMessage("hi from b", "")
	require.NoError(t, err, "separate handler must have its own guard")
}

func TestHandleSendMessage_GuardDoesNotAffectOtherTools(t *testing.T) {
	tmp := t.TempDir()
	for _, sub := range []string{"messages", "tasks", "reactions"} {
		require.NoError(t, os.MkdirAll(filepath.Join(tmp, sub), 0755))
	}

	h := &toolHandlers{ipcDir: tmp, chatID: "chat-1"}
	_, err := h.handleSendMessage("only reply", "")
	require.NoError(t, err)

	_, err = h.handleScheduleTask("daily", "0 9 * * *", "summarise")
	require.NoError(t, err, "scheduleTask must not be blocked by sendMessage guard")

	_, err = h.handleReactToMessage("42", "👍")
	require.NoError(t, err, "reactToMessage must not be blocked by sendMessage guard")
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
