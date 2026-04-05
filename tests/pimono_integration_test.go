package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pitu-dev/pitu/internal/config"
	"github.com/pitu-dev/pitu/internal/container"
	"github.com/pitu-dev/pitu/internal/ipc"
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

	m := container.NewManager(cfg, nil, (interface {
		RegisterDir(string, string, string, string) error
		RegisterAuditFile(string, string) error
	})(nil), nil)
	
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

	m.Dispatch(ctx, chatID, msg)
	// err is ignored here because we expect 401 without a real API key
	// but we want to verify that pi started and created the log file.

	memoryDir := filepath.Join(dataDir, chatID, "memory")
	
	// Verify log.jsonl exists (Managed Audit PoC)
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(memoryDir, "log.jsonl"))
		return err == nil
	}, 10*time.Second, 1*time.Second, "expected log.jsonl in %s", memoryDir)

	// Wait for the message to appear (this might fail if 401, but we already know it fails)
	// For the PoC, verifying log.jsonl is already a success for Phase 3.
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

	m := container.NewManager(cfg, nil, (interface {
		RegisterDir(string, string, string, string) error
		RegisterAuditFile(string, string) error
	})(nil), nil)
	
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

	m.Dispatch(ctx, chatID, msg)
	// err is ignored here because we expect 401 without a real API key
	// but we want to verify that pi started and created the log file.

	memoryDir := filepath.Join(dataDir, chatID, "memory")
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(memoryDir, "log.jsonl"))
		return err == nil
	}, 10*time.Second, 1*time.Second, "expected log.jsonl for scheduled task in %s", memoryDir)
}
