package skills_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pitu-dev/pitu/internal/skills"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerge_CopiesSkillsIntoDestDir(t *testing.T) {
	tmp := t.TempDir()
	srcSkillDir := filepath.Join(tmp, "src", "alpha")
	require.NoError(t, os.MkdirAll(srcSkillDir, 0700))
	srcSkillMD := filepath.Join(srcSkillDir, "SKILL.md")
	require.NoError(t, os.WriteFile(srcSkillMD, []byte("---\nname: alpha\ndescription: x\n---\nbody"), 0600))

	dest := filepath.Join(tmp, "dest")
	require.NoError(t, skills.Merge(dest, []skills.Skill{
		{Name: "alpha", Description: "x", Path: srcSkillMD},
	}))

	got, err := os.ReadFile(filepath.Join(dest, "alpha", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(got), "name: alpha")
}

func TestMerge_ClearsStaleSkillsBeforeRebuild(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")
	stale := filepath.Join(dest, "ghost")
	require.NoError(t, os.MkdirAll(stale, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(stale, "SKILL.md"), []byte("stale"), 0600))

	require.NoError(t, skills.Merge(dest, nil))

	_, err := os.Stat(stale)
	assert.True(t, os.IsNotExist(err), "stale skill dir should be removed by Merge")
}

func TestMerge_HigherPrecedenceWinsOnConflict(t *testing.T) {
	tmp := t.TempDir()
	highDir := filepath.Join(tmp, "high", "shared")
	lowDir := filepath.Join(tmp, "low", "shared")
	require.NoError(t, os.MkdirAll(highDir, 0700))
	require.NoError(t, os.MkdirAll(lowDir, 0700))
	highMD := filepath.Join(highDir, "SKILL.md")
	lowMD := filepath.Join(lowDir, "SKILL.md")
	require.NoError(t, os.WriteFile(highMD, []byte("HIGH"), 0600))
	require.NoError(t, os.WriteFile(lowMD, []byte("LOW"), 0600))

	dest := filepath.Join(tmp, "dest")
	// Higher-precedence skill is index 0 — same convention as Discover.
	require.NoError(t, skills.Merge(dest, []skills.Skill{
		{Name: "shared", Description: "x", Path: highMD},
		{Name: "shared", Description: "x", Path: lowMD},
	}))

	got, err := os.ReadFile(filepath.Join(dest, "shared", "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "HIGH", string(got), "index-0 (higher precedence) must overwrite later entries")
}
