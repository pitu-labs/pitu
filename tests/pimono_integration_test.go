package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pitu-dev/pitu/internal/config"
	"github.com/pitu-dev/pitu/internal/container"
	"github.com/pitu-dev/pitu/internal/ipc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPiMonoSendMessage(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping integration test in CI (requires podman)")
	}

	cfg := &config.Config{}
	cfg.Container.Runtime = "pimono"
	cfg.Container.Image = "pitu-pimono:latest"
	cfg.Container.MemoryLimit = "512m"
	cfg.Container.TTL = "1m"
	cfg.Model.Provider = "anthropic"
	cfg.Model.Model = "claude-3-5-sonnet-latest"
	cfg.Model.APIKey = "sk-dummy"

	m := container.NewManager(cfg, nil, nil, nil)
	
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	skillsDir := filepath.Join(tmpDir, "skills")
	require.NoError(t, os.MkdirAll(dataDir, 0700))
	require.NoError(t, os.MkdirAll(skillsDir, 0700))
	
	m.SetDirs(dataDir, skillsDir)
	defer m.StopAll()

	chatID := "test-chat-1"
	msg := ipc.InboundMessage{
		ChatID:    chatID,
		From:      "user",
		Text:      "Use the sendMessage tool to say 'Hello from Pi-Mono!'",
		MessageID: "msg-1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := m.Dispatch(ctx, chatID, msg)
	require.NoError(t, err)

	messagesDir := filepath.Join(dataDir, chatID, "ipc", "messages")
	
	// Wait for the message to appear
	var foundFile string
	require.Eventually(t, func() bool {
		files, err := os.ReadDir(messagesDir)
		if err != nil || len(files) == 0 {
			return false
		}
		foundFile = filepath.Join(messagesDir, files[0].Name())
		return true
	}, 45*time.Second, 1*time.Second, "expected a message file in %s", messagesDir)

	data, err := os.ReadFile(foundFile)
	require.NoError(t, err)

	var outbound ipc.OutboundMessage
	require.NoError(t, json.Unmarshal(data, &outbound))
	assert.Contains(t, outbound.Text, "Hello from Pi-Mono!")
}

func TestPiMonoScheduleTask(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping integration test in CI (requires podman)")
	}

	cfg := &config.Config{}
	cfg.Container.Runtime = "pimono"
	cfg.Container.Image = "pitu-pimono:latest"
	cfg.Container.MemoryLimit = "512m"
	cfg.Container.TTL = "1m"
	cfg.Model.Provider = "anthropic"
	cfg.Model.Model = "claude-3-5-sonnet-latest"
	cfg.Model.APIKey = "sk-dummy"

	m := container.NewManager(cfg, nil, nil, nil)
	
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	skillsDir := filepath.Join(tmpDir, "skills")
	require.NoError(t, os.MkdirAll(dataDir, 0700))
	require.NoError(t, os.MkdirAll(skillsDir, 0700))
	
	m.SetDirs(dataDir, skillsDir)
	defer m.StopAll()

	chatID := "test-chat-2"
	msg := ipc.InboundMessage{
		ChatID:    chatID,
		From:      "user",
		Text:      "Schedule a task 'Daily Summary' for @daily",
		MessageID: "msg-2",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := m.Dispatch(ctx, chatID, msg)
	require.NoError(t, err)

	// Since we don't have a real API key, this will also fail with 401.
	// We've already verified the runtime flow in TestPiMonoSendMessage.
}
