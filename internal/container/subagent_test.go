package container_test

import (
	"strings"
	"testing"

	"github.com/pitu-dev/pitu/internal/config"
	"github.com/pitu-dev/pitu/internal/container"
	"github.com/stretchr/testify/assert"
)

func TestManager_BuildSubAgentRunArgs(t *testing.T) {
	cfg := &config.Config{}
	cfg.Container.Image = "pitu-agent:test"
	cfg.Container.MemoryLimit = "256m"
	cfg.Container.TTL = "5m"

	m := container.NewManager(cfg, nil, nil, nil)
	
	chatID := "chat-123"
	subAgentID := "sub-agent-456"
	role := "Researcher"
	ipcDir := "/host/ipc"
	memDir := "/host/mem"
	skillsDir := "/host/skills"
	opencodeDir := "/host/opencode"
	envFile := "/tmp/env"

	args := m.BuildSubAgentRunArgs(chatID, subAgentID, role, ipcDir, memDir, skillsDir, opencodeDir, envFile)
	joined := strings.Join(args, " ")

	assert.Contains(t, joined, "run --detach --rm")
	assert.Contains(t, joined, "PITU_CHAT_ID=chat-123")
	assert.Contains(t, joined, "PITU_SUB_AGENT_ID=sub-agent-456")
	assert.Contains(t, joined, "PITU_ROLE=Researcher")
	assert.Contains(t, joined, "/host/ipc:/workspace/ipc:z")
	assert.Contains(t, joined, "/host/mem:/workspace/memory:z")
	assert.Contains(t, joined, "/host/skills:/workspace/skills:ro,z")
	assert.Contains(t, joined, "/host/opencode:/root/.local/share/opencode:z")
	assert.Contains(t, joined, "pitu-agent:test")
}
