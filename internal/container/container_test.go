package container_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pitu-dev/pitu/internal/config"
	"github.com/pitu-dev/pitu/internal/container"
	"github.com/pitu-dev/pitu/internal/ipc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateOpenCodeConfig_ContainsChatID(t *testing.T) {
	cfg := container.GenerateOpenCodeConfig("chat-123")
	assert.Contains(t, cfg, "chat-123")
	assert.Contains(t, cfg, "pitu-mcp")
	// Must be valid JSON
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(cfg), &parsed))
}

func TestWriteInputFile_CreatesCorrectFile(t *testing.T) {
	tmp := t.TempDir()
	inputDir := filepath.Join(tmp, "input")
	require.NoError(t, os.MkdirAll(inputDir, 0755))

	msg := ipc.InboundMessage{
		ChatID:    "999",
		From:      "bob",
		Text:      "test message",
		MessageID: "m42",
	}

	path, err := container.WriteInputFile(tmp, msg)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path, ".json"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var got ipc.InboundMessage
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "test message", got.Text)
	assert.Equal(t, "m42", got.MessageID)
}

func TestManager_GeneratesCorrectPodmanRunArgs(t *testing.T) {
	cfg := &config.Config{}
	cfg.Container.Image = "pitu-agent:test"
	cfg.Container.MemoryLimit = "256m"
	cfg.Container.TTL = "5m"

	m := container.NewManager(cfg, nil, nil, nil)
	args := m.BuildRunArgs("chat-99", "/host/ipc", "/host/memory", "/host/skills", "/host/opencode")

	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "pitu-agent:test")
	assert.Contains(t, joined, "/host/ipc")
	assert.Contains(t, joined, "PITU_CHAT_ID=chat-99")
	assert.Contains(t, joined, "256m")
}

func TestBuildExecArgs_NoCFlag_WhenNoSession(t *testing.T) {
	cfg := &config.Config{}
	m := container.NewManager(cfg, nil, nil, nil)
	args := m.BuildExecArgs("ctr-abc", "/host/ipc/input/msg.json", false)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "opencode run")
	assert.Contains(t, joined, "-f /workspace/ipc/input/msg.json")
	assert.NotContains(t, joined, " -c ")
}

func TestBuildExecArgs_CFlagPresent_WhenHasSession(t *testing.T) {
	cfg := &config.Config{}
	m := container.NewManager(cfg, nil, nil, nil)
	args := m.BuildExecArgs("ctr-abc", "/host/ipc/input/msg.json", true)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, " -c ")
	assert.Contains(t, joined, "-f /workspace/ipc/input/msg.json")
}

func TestManager_BuildRunArgs_ContainsOpenCodeMount(t *testing.T) {
	cfg := &config.Config{}
	cfg.Container.Image = "pitu-agent:test"
	cfg.Container.MemoryLimit = "256m"
	cfg.Container.TTL = "5m"

	m := container.NewManager(cfg, nil, nil, nil)
	args := m.BuildRunArgs("chat-99", "/host/ipc", "/host/memory", "/host/skills", "/host/opencode")

	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "/host/opencode:/root/.local/share/opencode")
}

func TestManager_BuildSpawnArgs(t *testing.T) {
	cfg := &config.Config{}
	m := container.NewManager(cfg, nil, nil, nil)
	args := m.BuildSpawnArgs("ctr-xyz", "Researcher", "find papers on Go concurrency")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "exec ctr-xyz opencode run")
	assert.Contains(t, joined, "--title Researcher")
	assert.Contains(t, joined, "Researcher: find papers on Go concurrency")
	assert.NotContains(t, joined, " -c ")
}

func TestWriteInputFile_FileNameIncludesTimestamp(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "input"), 0755))

	msg := ipc.InboundMessage{ChatID: "1", From: "a", Text: "b", MessageID: "c"}
	before := time.Now().UnixNano()
	path, err := container.WriteInputFile(tmp, msg)
	after := time.Now().UnixNano()
	require.NoError(t, err)

	name := filepath.Base(path)
	// name is "{ts}-{msgid}.json" — ts should be between before and after
	parts := strings.SplitN(strings.TrimSuffix(name, ".json"), "-", 2)
	require.Len(t, parts, 2)
	var ts int64
	_, err = fmt.Sscanf(parts[0], "%d", &ts)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, ts, before)
	assert.LessOrEqual(t, ts, after)
}
