package skills_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pitu-dev/pitu/internal/skills"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAgentConfig_AllFilesPresent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("be kind"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "IDENTITY.md"), []byte("You are Aria."), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "USER.md"), []byte("User is Rob."), 0644))

	cfg := skills.LoadAgentConfig(dir)
	assert.Equal(t, "be kind", cfg.Soul)
	assert.Equal(t, "You are Aria.", cfg.Identity)
	assert.Equal(t, "User is Rob.", cfg.User)
}

func TestLoadAgentConfig_PartialFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("be kind"), 0644))

	cfg := skills.LoadAgentConfig(dir)
	assert.Equal(t, "be kind", cfg.Soul)
	assert.Empty(t, cfg.Identity)
	assert.Empty(t, cfg.User)
}

func TestLoadAgentConfig_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	cfg := skills.LoadAgentConfig(dir)
	assert.Empty(t, cfg.Soul)
	assert.Empty(t, cfg.Identity)
	assert.Empty(t, cfg.User)
}

func TestLoadAgentConfig_MissingDir(t *testing.T) {
	cfg := skills.LoadAgentConfig("/nonexistent/path/that/does/not/exist")
	assert.Empty(t, cfg.Soul)
	assert.Empty(t, cfg.Identity)
	assert.Empty(t, cfg.User)
}
