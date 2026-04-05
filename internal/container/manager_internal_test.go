package container

import (
	"os"
	"testing"

	"github.com/pitu-dev/pitu/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteEnvFile_OpenCode(t *testing.T) {
	cfg := &config.Config{}
	cfg.Container.Runtime = "opencode"
	m := &Manager{cfg: cfg}

	ef, err := os.CreateTemp("", "pitu-test-env-*")
	require.NoError(t, err)
	defer os.Remove(ef.Name())

	err = m.writeEnvFile(ef, "chat-123")
	require.NoError(t, err)
	ef.Close()

	data, err := os.ReadFile(ef.Name())
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "OPENCODE_CONFIG_CONTENT=")
	assert.NotContains(t, content, "PI_CONFIG_CONTENT=")
}

func TestWriteEnvFile_Pimono(t *testing.T) {
	cfg := &config.Config{}
	cfg.Container.Runtime = "pimono"
	m := &Manager{cfg: cfg}

	ef, err := os.CreateTemp("", "pitu-test-env-*")
	require.NoError(t, err)
	defer os.Remove(ef.Name())

	err = m.writeEnvFile(ef, "chat-123")
	require.NoError(t, err)
	ef.Close()

	data, err := os.ReadFile(ef.Name())
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "PI_CONFIG_CONTENT=")
	assert.NotContains(t, content, "OPENCODE_CONFIG_CONTENT=")
}
