package container_test

import (
	"strings"
	"testing"

	"github.com/pitu-dev/pitu/internal/config"
	"github.com/pitu-dev/pitu/internal/container"
	"github.com/stretchr/testify/assert"
)

func TestBuildExecArgsPimono(t *testing.T) {
	cfg := &config.Config{}
	m := container.NewManager(cfg, nil, nil, nil)
	args := m.BuildExecArgsPimono("ctr-123", "/host/ipc/input/msg.json")
	joined := strings.Join(args, " ")
	assert.Equal(t, "exec ctr-123 pi run --file /workspace/ipc/input/msg.json", joined)
}

func TestBuildExecArgs_PimonoRuntime(t *testing.T) {
	cfg := &config.Config{}
	cfg.Container.Runtime = "pimono"
	m := container.NewManager(cfg, nil, nil, nil)
	args := m.BuildExecArgs("ctr-123", "/host/ipc/input/msg.json", false)
	joined := strings.Join(args, " ")
	assert.Equal(t, "exec ctr-123 pi run --file /workspace/ipc/input/msg.json", joined)
}
