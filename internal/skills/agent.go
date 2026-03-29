package skills

import (
	"os"
	"path/filepath"
)

// AgentConfig holds the operator-supplied personality and context files for the agent.
// All fields are optional — empty string means the corresponding file was absent.
type AgentConfig struct {
	Soul     string // contents of SOUL.md
	Identity string // contents of IDENTITY.md
	User     string // contents of USER.md
}

// LoadAgentConfig reads SOUL.md, IDENTITY.md, and USER.md from dir.
// Missing or unreadable files are silently skipped.
func LoadAgentConfig(dir string) AgentConfig {
	return AgentConfig{
		Soul:     readFileOrEmpty(filepath.Join(dir, "SOUL.md")),
		Identity: readFileOrEmpty(filepath.Join(dir, "IDENTITY.md")),
		User:     readFileOrEmpty(filepath.Join(dir, "USER.md")),
	}
}

func readFileOrEmpty(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
