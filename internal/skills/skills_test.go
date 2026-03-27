package skills_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pitu-dev/pitu/internal/skills"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(skillDir, 0755))
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\nDo things.\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644))
}

func TestDiscover_FindsSkills(t *testing.T) {
	tmp := t.TempDir()
	makeSkill(t, tmp, "my-skill", "Does my thing")

	found := skills.Discover([]string{tmp})
	require.Len(t, found, 1)
	assert.Equal(t, "my-skill", found[0].Name)
	assert.Equal(t, "Does my thing", found[0].Description)
}

func TestDiscover_ProjectOverridesUser(t *testing.T) {
	user := t.TempDir()
	project := t.TempDir()
	makeSkill(t, user, "shared-skill", "user version")
	makeSkill(t, project, "shared-skill", "project version")

	// project paths listed first = higher precedence
	found := skills.Discover([]string{project, user})
	require.Len(t, found, 1)
	assert.Equal(t, "project version", found[0].Description)
}

func TestDiscover_MissingDescriptionSkipped(t *testing.T) {
	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "bad-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: bad-skill\n---\n"), 0644))
	found := skills.Discover([]string{tmp})
	assert.Empty(t, found)
}

func TestBuildCatalog_ContainsNameAndDescription(t *testing.T) {
	tmp := t.TempDir()
	makeSkill(t, tmp, "demo-skill", "Demonstrates things. Use when demonstrating.")
	found := skills.Discover([]string{tmp})
	catalog := skills.BuildCatalog(found)
	assert.Contains(t, catalog, "demo-skill")
	assert.Contains(t, catalog, "Demonstrates things")
}

func TestWriteContext_CreatesFile(t *testing.T) {
	tmp := t.TempDir()
	skillsDir := t.TempDir()
	makeSkill(t, skillsDir, "test-skill", "Test skill description")
	found := skills.Discover([]string{skillsDir})

	require.NoError(t, skills.WriteContext(tmp, "chat-42", found))
	data, err := os.ReadFile(filepath.Join(tmp, "CONTEXT.md"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "chat-42")
	assert.Contains(t, content, "test-skill")
}

func TestWriteContext_DoesNotOverwriteExisting(t *testing.T) {
	tmp := t.TempDir()
	existing := filepath.Join(tmp, "CONTEXT.md")
	require.NoError(t, os.WriteFile(existing, []byte("# existing content"), 0644))

	require.NoError(t, skills.WriteContext(tmp, "any-chat", nil))
	data, _ := os.ReadFile(existing)
	assert.True(t, strings.HasPrefix(string(data), "# existing content"))
}

func TestWriteContext_ContainsCapabilitiesBlock(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, skills.WriteContext(tmp, "chat-99", nil))
	data, err := os.ReadFile(filepath.Join(tmp, "CONTEXT.md"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "Capabilities & Limitations")
	assert.Contains(t, content, "TeamCreate")
	assert.Contains(t, content, "single-agent instance")
	assert.NotContains(t, content, "// TODO")
}
