package skills

import (
	"os"
	"path/filepath"
	"strings"
)

// WriteContext creates CONTEXT.md in dir for the given chatID, skills, and agent config.
// Does nothing if CONTEXT.md already exists (preserves accumulated memory).
func WriteContext(dir, chatID string, discovered []Skill, agent AgentConfig) error {
	path := filepath.Join(dir, "CONTEXT.md")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists — don't overwrite
	}
	catalog := BuildCatalog(discovered)

	var b strings.Builder
	b.WriteString("# Agent Context\n\n")
	b.WriteString("**Chat ID:** " + chatID + "\n")
	b.WriteString("**Platform:** Telegram\n")

	if agent.Identity != "" {
		b.WriteString("\n## Identity\n\n")
		b.WriteString(strings.TrimRight(agent.Identity, "\n"))
		b.WriteString("\n")
	}
	if agent.Soul != "" {
		b.WriteString("\n## Soul\n\n")
		b.WriteString(strings.TrimRight(agent.Soul, "\n"))
		b.WriteString("\n")
	}
	if agent.User != "" {
		b.WriteString("\n## User\n\n")
		b.WriteString(strings.TrimRight(agent.User, "\n"))
		b.WriteString("\n")
	}

	b.WriteString("\n## Skills\n\n")
	b.WriteString("When a task matches a skill's description, read the SKILL.md at the listed path to load its full instructions.\n\n")
	b.WriteString(catalog)
	b.WriteString("\n\n## Instructions\n\n")

	if agent.Soul != "" {
		b.WriteString("Respond to messages via the mcp__pitu__sendMessage tool.\n")
	} else {
		b.WriteString("You are a helpful AI assistant running inside Pitu. Respond to messages via the mcp__pitu__sendMessage tool.\n")
	}

	return os.WriteFile(path, []byte(b.String()), 0644)
}
