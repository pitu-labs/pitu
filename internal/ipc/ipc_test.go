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
		func(ipc.ReactionFile) {},
	)

	msg := ipc.OutboundMessage{ChatID: "123", Text: "reply", Type: "message"}
	data, _ := json.Marshal(msg)
	tmp := t.TempDir()
	fpath := filepath.Join(tmp, "ts-1.json")
	require.NoError(t, os.WriteFile(fpath, data, 0644))

	require.NoError(t, r.Route("messages", fpath, "123", "", ""))
	require.NotNil(t, gotMsg)
	assert.Equal(t, "123", gotMsg.ChatID)
	assert.Equal(t, "reply", gotMsg.Text)
}

func TestRouter_DispatchesMessageFile_WithRoleAndSubAgentID(t *testing.T) {
	var gotMsg *ipc.OutboundMessage
	r := ipc.NewRouter(
		func(m ipc.OutboundMessage) { gotMsg = &m },
		func(f ipc.TaskFile) {},
		func(g ipc.GroupFile) {},
		func(ipc.AgentFile) {},
		func(ipc.ReactionFile) {},
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

	require.NoError(t, r.Route("messages", fpath, "123", "researcher", "agent-1"))
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
		func(ipc.ReactionFile) {},
	)

	task := ipc.TaskFile{Action: "create", Name: "daily", Schedule: "0 9 * * *", Prompt: "p", ChatID: "123"}
	data, _ := json.Marshal(task)
	tmp := t.TempDir()
	fpath := filepath.Join(tmp, "ts-2.json")
	require.NoError(t, os.WriteFile(fpath, data, 0644))

	require.NoError(t, r.Route("tasks", fpath, "123", "", ""))
	require.NotNil(t, gotTask)
	assert.Equal(t, "123", gotTask.ChatID)
	assert.Equal(t, "daily", gotTask.Name)
}

func TestRouter_UnknownSubdir(t *testing.T) {
        r := ipc.NewRouter(func(ipc.OutboundMessage) {}, func(ipc.TaskFile) {}, func(ipc.GroupFile) {}, func(ipc.AgentFile) {}, func(ipc.ReactionFile) {})
	err := r.Route("unknown", "/tmp/file.json", "x", "", "")
        assert.Error(t, err)
}
func TestRouter_DispatchesAgentFile(t *testing.T) {
	var gotAgent *ipc.AgentFile
	r := ipc.NewRouter(
		func(ipc.OutboundMessage) {},
		func(ipc.TaskFile) {},
		func(ipc.GroupFile) {},
		func(a ipc.AgentFile) { gotAgent = &a },
		func(ipc.ReactionFile) {},
	)

	af := ipc.AgentFile{Action: "spawn", SubAgentID: "uuid-1", Role: "Researcher", Prompt: "find papers", ChatID: "chat-1"}
	data, _ := json.Marshal(af)
	tmp := t.TempDir()
	fpath := filepath.Join(tmp, "ts-3.json")
	require.NoError(t, os.WriteFile(fpath, data, 0644))

	require.NoError(t, r.Route("agents", fpath, "chat-1", "", ""))
	require.NotNil(t, gotAgent)
	assert.Equal(t, "chat-1", gotAgent.ChatID)
	assert.Equal(t, "Researcher", gotAgent.Role)
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
		func(ipc.ReactionFile) {},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w, err := ipc.NewWatcher(r)
	require.NoError(t, err)
	require.NoError(t, w.RegisterDir(tmp, "chat-1", "", ""))
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
		func(ipc.ReactionFile) {},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w, err := ipc.NewWatcher(r)
	require.NoError(t, err)
	require.NoError(t, w.RegisterDir(tmp, "chat-99", "", ""))
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
	assert.Equal(t, "chat-99", received[0].ChatID)
	assert.Equal(t, "watch test", received[0].Text)
}

func TestRouter_OverridesForgedChatID(t *testing.T) {
	var gotMsg *ipc.OutboundMessage
	r := ipc.NewRouter(
		func(m ipc.OutboundMessage) { gotMsg = &m },
		func(ipc.TaskFile) {},
		func(ipc.GroupFile) {},
		func(ipc.AgentFile) {},
		func(ipc.ReactionFile) {},
	)

	// Container forges a different chat_id in the JSON body
	msg := ipc.OutboundMessage{ChatID: "other-chat", Text: "sneaky", Type: "message"}
	data, _ := json.Marshal(msg)
	tmp := t.TempDir()
	fpath := filepath.Join(tmp, "ts-forge.json")
	require.NoError(t, os.WriteFile(fpath, data, 0644))

	require.NoError(t, r.Route("messages", fpath, "real-chat", "", ""))
	require.NotNil(t, gotMsg)
	assert.Equal(t, "real-chat", gotMsg.ChatID) // trusted, not forged
	assert.Equal(t, "sneaky", gotMsg.Text)
}

func TestRouter_OverridesForgedChatID_Tasks(t *testing.T) {
	var gotTask *ipc.TaskFile
	r := ipc.NewRouter(
		func(ipc.OutboundMessage) {},
		func(f ipc.TaskFile) { gotTask = &f },
		func(ipc.GroupFile) {},
		func(ipc.AgentFile) {},
		func(ipc.ReactionFile) {},
	)
	tf := ipc.TaskFile{Action: "create", ChatID: "forged-chat", Name: "t"}
	data, _ := json.Marshal(tf)
	fpath := filepath.Join(t.TempDir(), "ts.json")
	require.NoError(t, os.WriteFile(fpath, data, 0644))
	require.NoError(t, r.Route("tasks", fpath, "real-chat", "", ""))
	require.NotNil(t, gotTask)
	assert.Equal(t, "real-chat", gotTask.ChatID)
}

func TestRouter_OverridesForgedChatID_Groups(t *testing.T) {
	var gotGroup *ipc.GroupFile
	r := ipc.NewRouter(
		func(ipc.OutboundMessage) {},
		func(ipc.TaskFile) {},
		func(g ipc.GroupFile) { gotGroup = &g },
		func(ipc.AgentFile) {},
		func(ipc.ReactionFile) {},
	)
	gf := ipc.GroupFile{ChatID: "forged-chat", Name: "g"}
	data, _ := json.Marshal(gf)
	fpath := filepath.Join(t.TempDir(), "ts.json")
	require.NoError(t, os.WriteFile(fpath, data, 0644))
	require.NoError(t, r.Route("groups", fpath, "real-chat", "", ""))
	require.NotNil(t, gotGroup)
	assert.Equal(t, "real-chat", gotGroup.ChatID)
}

func TestRouter_OverridesForgedChatID_Agents(t *testing.T) {
	var gotAgent *ipc.AgentFile
	r := ipc.NewRouter(
		func(ipc.OutboundMessage) {},
		func(ipc.TaskFile) {},
		func(ipc.GroupFile) {},
		func(a ipc.AgentFile) { gotAgent = &a },
		func(ipc.ReactionFile) {},
	)
	af := ipc.AgentFile{Action: "spawn", ChatID: "forged-chat", Role: "Writer"}
	data, _ := json.Marshal(af)
	fpath := filepath.Join(t.TempDir(), "ts.json")
	require.NoError(t, os.WriteFile(fpath, data, 0644))
	require.NoError(t, r.Route("agents", fpath, "real-chat", "", ""))
	require.NotNil(t, gotAgent)
	assert.Equal(t, "real-chat", gotAgent.ChatID)
}

func TestRouter_OverridesForgedChatID_Reactions(t *testing.T) {
	var gotReaction *ipc.ReactionFile
	r := ipc.NewRouter(
		func(ipc.OutboundMessage) {},
		func(ipc.TaskFile) {},
		func(ipc.GroupFile) {},
		func(ipc.AgentFile) {},
		func(rf ipc.ReactionFile) { gotReaction = &rf },
	)
	rf := ipc.ReactionFile{ChatID: "forged-chat", MessageID: 42, Emoji: "👍"}
	data, _ := json.Marshal(rf)
	fpath := filepath.Join(t.TempDir(), "ts.json")
	require.NoError(t, os.WriteFile(fpath, data, 0644))
	require.NoError(t, r.Route("reactions", fpath, "real-chat", "", ""))
	require.NotNil(t, gotReaction)
	assert.Equal(t, "real-chat", gotReaction.ChatID)
}
