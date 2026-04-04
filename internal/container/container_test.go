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
	cfg := container.GenerateOpenCodeConfig("chat-123", config.ModelConfig{})
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
	args := m.BuildRunArgs("chat-99", "/host/ipc", "/host/memory", "/host/skills", "/host/opencode", "/tmp/test-env-file")

	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "pitu-agent:test")
	assert.Contains(t, joined, "/host/ipc")
	assert.Contains(t, joined, "PITU_CHAT_ID=chat-99")
	assert.Contains(t, joined, "256m")
	assert.Contains(t, joined, "--env-file")
	assert.NotContains(t, joined, "OPENCODE_CONFIG_CONTENT")
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
	args := m.BuildRunArgs("chat-99", "/host/ipc", "/host/memory", "/host/skills", "/host/opencode", "/tmp/test-env-file")

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

func TestGenerateOpenCodeConfig_InjectsAnthropicProvider(t *testing.T) {
	m := config.ModelConfig{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-5",
		APIKey:   "sk-ant-test",
	}
	cfg := container.GenerateOpenCodeConfig("chat-1", m)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(cfg), &parsed))

	assert.Equal(t, "anthropic/claude-sonnet-4-5", parsed["model"])

	providers, ok := parsed["provider"].(map[string]any)
	require.True(t, ok, "provider key must be an object")
	anthropic, ok := providers["anthropic"].(map[string]any)
	require.True(t, ok, "provider.anthropic must be an object")
	opts, ok := anthropic["options"].(map[string]any)
	require.True(t, ok, "provider.anthropic.options must be an object")
	// API key must NOT be in the config JSON — it is passed via env var
	_, hasAPIKey := opts["apiKey"]
	assert.False(t, hasAPIKey, "apiKey must not appear in config JSON")
	// No npm field for native anthropic provider
	_, hasNPM := anthropic["npm"]
	assert.False(t, hasNPM)
}

func TestGenerateOpenCodeConfig_InjectsOllamaProvider(t *testing.T) {
	m := config.ModelConfig{
		Provider: "ollama",
		Model:    "llama3",
		BaseURL:  "http://localhost:11434/v1",
	}
	cfg := container.GenerateOpenCodeConfig("chat-2", m)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(cfg), &parsed))

	assert.Equal(t, "ollama/llama3", parsed["model"])

	providers := parsed["provider"].(map[string]any)
	ollama := providers["ollama"].(map[string]any)
	assert.Equal(t, "@ai-sdk/openai-compatible", ollama["npm"])
	opts := ollama["options"].(map[string]any)
	assert.Equal(t, "http://localhost:11434/v1", opts["baseURL"])
	_, hasAPIKey := opts["apiKey"]
	assert.False(t, hasAPIKey)
}

func TestGenerateOpenCodeConfig_EmptyModelOmitsProviderAndModel(t *testing.T) {
	cfg := container.GenerateOpenCodeConfig("chat-3", config.ModelConfig{})

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(cfg), &parsed))

	_, hasModel := parsed["model"]
	assert.False(t, hasModel, "model key must be absent when ModelConfig is empty")
	_, hasProvider := parsed["provider"]
	assert.False(t, hasProvider, "provider key must be absent when ModelConfig is empty")
}

func TestManager_BuildRunArgs_InjectsModelWhenConfigured(t *testing.T) {
	cfg := &config.Config{}
	cfg.Container.Image = "pitu-agent:test"
	cfg.Container.MemoryLimit = "256m"
	cfg.Container.TTL = "5m"
	cfg.Model.Provider = "anthropic"
	cfg.Model.Model = "claude-sonnet-4-5"
	cfg.Model.APIKey = "sk-ant-test"

	m := container.NewManager(cfg, nil, nil, nil)
	args := m.BuildRunArgs("chat-99", "/host/ipc", "/host/memory", "/host/skills", "/host/opencode", "/tmp/test-env-file")

	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "--env-file")
}

func TestManager_BuildRunArgs_UsesEnvFile(t *testing.T) {
	cfg := &config.Config{}
	cfg.Container.Image = "pitu-agent:test"
	cfg.Container.MemoryLimit = "256m"
	cfg.Container.TTL = "5m"
	cfg.Model.Provider = "anthropic"
	cfg.Model.APIKey = "sk-ant-secret"

	m := container.NewManager(cfg, nil, nil, nil)
	args := m.BuildRunArgs("chat-1", "/ipc", "/mem", "/skills", "/opencode", "/tmp/env-file")
	joined := strings.Join(args, " ")

	assert.Contains(t, joined, "--env-file")
	assert.Contains(t, joined, "/tmp/env-file")
	assert.NotContains(t, joined, "sk-ant-secret")
	assert.NotContains(t, joined, "OPENCODE_CONFIG_CONTENT")
}

// TestEnsureContainer_NoDuplicateStart would ideally assert that concurrent
// Dispatch calls for the same chatID result in exactly one startContainer
// invocation. This cannot be unit-tested here because startContainer shells out
// to `podman run`, which is unavailable in CI. The fix (startMu double-checked
// lock in manager.go) is verified by code inspection and can be validated with
// `go test -race` against a live Podman environment.

func TestGenerateOpenCodeConfig_DoesNotEmbedAPIKey(t *testing.T) {
	m := config.ModelConfig{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-5",
		APIKey:   "sk-ant-secret-key",
	}
	cfg := container.GenerateOpenCodeConfig("chat-1", m)
	assert.NotContains(t, cfg, "sk-ant-secret-key",
		"API key must not appear in config JSON")
}

func TestAPIKeyEnvVar(t *testing.T) {
	assert.Equal(t, "ANTHROPIC_API_KEY", container.APIKeyEnvVar("anthropic"))
	assert.Equal(t, "OPENAI_API_KEY", container.APIKeyEnvVar("openai"))
	assert.Equal(t, "ANTHROPIC_API_KEY", container.APIKeyEnvVar("ollama"))
	assert.Equal(t, "ANTHROPIC_API_KEY", container.APIKeyEnvVar(""))
}
