package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pitu-dev/pitu/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := `
[telegram]
bot_token = "test-token"

[container]
image = "pitu-agent:test"
ttl = "10m"
max_concurrent = 3
memory_limit = "256m"

[skills]
extra_paths = ["/tmp/skills"]

[db]
path = "/tmp/pitu.db"
`
	f := writeTempTOML(t, content)
	cfg, err := config.Load(f)
	require.NoError(t, err)
	assert.Equal(t, "test-token", cfg.Telegram.BotToken)
	assert.Equal(t, "pitu-agent:test", cfg.Container.Image)
	assert.Equal(t, "10m", cfg.Container.TTL)
	assert.Equal(t, 3, cfg.Container.MaxConcurrent)
	assert.Equal(t, "256m", cfg.Container.MemoryLimit)
	assert.Equal(t, []string{"/tmp/skills"}, cfg.Skills.ExtraPaths)
	assert.Equal(t, "/tmp/pitu.db", cfg.DB.Path)
}

func TestLoad_Defaults(t *testing.T) {
	content := `
[telegram]
bot_token = "tok"
`
	f := writeTempTOML(t, content)
	cfg, err := config.Load(f)
	require.NoError(t, err)
	assert.Equal(t, "5m", cfg.Container.TTL)
	assert.Equal(t, 5, cfg.Container.MaxConcurrent)
	assert.Equal(t, "512m", cfg.Container.MemoryLimit)
}

func TestLoad_MissingBotToken(t *testing.T) {
	content := `[telegram]`
	f := writeTempTOML(t, content)
	_, err := config.Load(f)
	assert.ErrorContains(t, err, "bot_token")
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load("/nonexistent/path.toml")
	assert.Error(t, err)
}

func writeTempTOML(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(f, []byte(content), 0600))
	return f
}
