package ipc_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pitu-dev/pitu/internal/ipc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouter_DispatchesMessageFile(t *testing.T) {
	var gotMsg *ipc.OutboundMessage
	r := ipc.NewRouter(
		func(m ipc.OutboundMessage) { gotMsg = &m },
		func(f ipc.TaskFile) {},
		func(g ipc.GroupFile) {},
		func(ipc.AgentFile) {},
	)

	msg := ipc.OutboundMessage{ChatID: "123", Text: "reply", Type: "message"}
	data, _ := json.Marshal(msg)
	tmp := t.TempDir()
	fpath := filepath.Join(tmp, "ts-1.json")
	require.NoError(t, os.WriteFile(fpath, data, 0644))

	require.NoError(t, r.Route("messages", fpath, "", ""))
	require.NotNil(t, gotMsg)
	assert.Equal(t, "reply", gotMsg.Text)
}

func TestRouter_DispatchesMessageFile_WithRoleAndSubAgentID(t *testing.T) {
	var gotMsg *ipc.OutboundMessage
	r := ipc.NewRouter(
		func(m ipc.OutboundMessage) { gotMsg = &m },
		func(f ipc.TaskFile) {},
		func(g ipc.GroupFile) {},
		func(ipc.AgentFile) {},
	)

	msg := ipc.OutboundMessage{
		ChatID:     "123",
		Text:       "reply",
		Type:       "message",
		Role:       "old-role",
		SubAgentID: "old-id",
	}
	data, _ := json.Marshal(msg)
	tmp := t.TempDir()
	fpath := filepath.Join(tmp, "ts-1.json")
	require.NoError(t, os.WriteFile(fpath, data, 0644))

	require.NoError(t, r.Route("messages", fpath, "researcher", "agent-1"))
	require.NotNil(t, gotMsg)
	assert.Equal(t, "researcher", gotMsg.Role)
	assert.Equal(t, "agent-1", gotMsg.SubAgentID)
}

func TestRouter_DispatchesTaskFile(t *testing.T) {
	var gotTask *ipc.TaskFile
	r := ipc.NewRouter(
		func(ipc.OutboundMessage) {},
		func(f ipc.TaskFile) { gotTask = &f },
		func(ipc.GroupFile) {},
		func(ipc.AgentFile) {},
	)

	task := ipc.TaskFile{Action: "create", Name: "daily", Schedule: "0 9 * * *", Prompt: "p", ChatID: "123"}
	data, _ := json.Marshal(task)
	tmp := t.TempDir()
	fpath := filepath.Join(tmp, "ts-2.json")
	require.NoError(t, os.WriteFile(fpath, data, 0644))

	require.NoError(t, r.Route("tasks", fpath, "", ""))
	require.NotNil(t, gotTask)	assert.Equal(t, "daily", gotTask.Name)
}

func TestRouter_UnknownSubdir(t *testing.T) {
        r := ipc.NewRouter(func(ipc.OutboundMessage) {}, func(ipc.TaskFile) {}, func(ipc.GroupFile) {}, func(ipc.AgentFile) {})
        err := r.Route("unknown", "/tmp/file.json", "", "")
        assert.Error(t, err)
}
func TestRouter_DispatchesAgentFile(t *testing.T) {
	var gotAgent *ipc.AgentFile
	r := ipc.NewRouter(
		func(ipc.OutboundMessage) {},
		func(ipc.TaskFile) {},
		func(ipc.GroupFile) {},
		func(a ipc.AgentFile) { gotAgent = &a },
	)

	af := ipc.AgentFile{Action: "spawn", SubAgentID: "uuid-1", Role: "Researcher", Prompt: "find papers", ChatID: "chat-1"}
	data, _ := json.Marshal(af)
	tmp := t.TempDir()
	fpath := filepath.Join(tmp, "ts-3.json")
	require.NoError(t, os.WriteFile(fpath, data, 0644))

	require.NoError(t, r.Route("agents", fpath, "", ""))
	require.NotNil(t, gotAgent)	assert.Equal(t, "Researcher", gotAgent.Role)
	assert.Equal(t, "find papers", gotAgent.Prompt)
}

func TestWatcher_PicksUpAgentFiles(t *testing.T) {
	tmp := t.TempDir()

	var received []ipc.AgentFile
	var mu sync.Mutex

	r := ipc.NewRouter(
		func(ipc.OutboundMessage) {},
		func(ipc.TaskFile) {},
		func(ipc.GroupFile) {},
		func(a ipc.AgentFile) {
			mu.Lock()
			received = append(received, a)
			mu.Unlock()
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w, err := ipc.NewWatcher(r)
	require.NoError(t, err)
	require.NoError(t, w.RegisterDir(tmp, "", ""))
	go w.Watch(ctx)
	time.Sleep(50 * time.Millisecond)

	af := ipc.AgentFile{Action: "spawn", SubAgentID: "u1", Role: "Writer", Prompt: "write a poem", ChatID: "c1"}
	data, _ := json.Marshal(af)
	agentsDir := filepath.Join(tmp, "agents")
	require.NoError(t, os.WriteFile(filepath.Join(agentsDir, "ts-100.json"), data, 0644))

	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, 1)
	assert.Equal(t, "Writer", received[0].Role)
}

func TestWatcher_PicksUpNewFiles(t *testing.T) {
	tmp := t.TempDir()

	var received []ipc.OutboundMessage
	var mu sync.Mutex

	r := ipc.NewRouter(
		func(m ipc.OutboundMessage) {
			mu.Lock()
			received = append(received, m)
			mu.Unlock()
		},
		func(ipc.TaskFile) {},
		func(ipc.GroupFile) {},
		func(ipc.AgentFile) {},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w, err := ipc.NewWatcher(r)
	require.NoError(t, err)
	require.NoError(t, w.RegisterDir(tmp, "", ""))
	go w.Watch(ctx)
	time.Sleep(50 * time.Millisecond) // let watcher start

	msg := ipc.OutboundMessage{ChatID: "99", Text: "watch test", Type: "message"}
	data, _ := json.Marshal(msg)
	messagesDir := filepath.Join(tmp, "messages") // created by RegisterDir
	require.NoError(t, os.WriteFile(filepath.Join(messagesDir, "ts-99.json"), data, 0644))

	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, 1)
	assert.Equal(t, "watch test", received[0].Text)
}
