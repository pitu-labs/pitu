package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteContext creates CONTEXT.md in dir for the given chatID and skills.
// Does nothing if CONTEXT.md already exists (preserves accumulated memory).
func WriteContext(dir, chatID string, discovered []Skill) error {
	path := filepath.Join(dir, "CONTEXT.md")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists — don't overwrite
	}
	catalog := BuildCatalog(discovered)
	content := fmt.Sprintf(`# Agent Context

**Chat ID:** %s
**Platform:** Telegram

## Skills

When a task matches a skill's description, read the SKILL.md at the listed path to load its full instructions.

%s

## Instructions

You are a helpful AI assistant running inside Pitu. Respond to messages via the mcp__pitu__sendMessage tool.
`, chatID, catalog)
	return os.WriteFile(path, []byte(content), 0644)
}
