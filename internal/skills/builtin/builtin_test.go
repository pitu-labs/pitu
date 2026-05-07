package builtin_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pitu-dev/pitu/internal/skills/builtin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnpack_WritesEmbeddedAssets(t *testing.T) {
	dest := t.TempDir()
	require.NoError(t, builtin.Unpack(dest))

	p := filepath.Join(dest, "README.md")
	data, err := os.ReadFile(p)
	require.NoError(t, err, "embedded README.md should exist after Unpack")
	assert.Contains(t, string(data), "Built-in Runtime Skills", "README.md content should match embedded asset")

	info, err := os.Stat(p)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "unpacked files should have mode 0600")
}

func TestUnpack_CreatesDestDirIfMissing(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "nonexistent")
	require.NoError(t, builtin.Unpack(dest))
	_, err := os.Stat(dest)
	assert.NoError(t, err, "Unpack should create dest dir when absent")
}

func TestUnpack_OverwritesExistingFiles(t *testing.T) {
	dest := t.TempDir()
	readme := filepath.Join(dest, "README.md")
	require.NoError(t, os.WriteFile(readme, []byte("stale content"), 0600))

	require.NoError(t, builtin.Unpack(dest))

	data, err := os.ReadFile(readme)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Built-in Runtime Skills", "Unpack should overwrite existing files with embedded content")
	assert.NotContains(t, string(data), "stale content", "stale content should be replaced")
}
