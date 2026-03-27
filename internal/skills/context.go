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

## Capabilities & Limitations

You are a single-agent instance. Multi-agent spawning is not available in this environment.

Do not use `+"`TeamCreate`, `Task`, `TaskOutput`, `TaskStop`, or `TeamDelete`"+` — these tools will not function correctly in this environment.

If a user requests a task that would benefit from multiple agents, acknowledge the limitation and complete it yourself as a single agent. Example: "I'll handle this on my own — multi-agent support isn't available yet in this Pitu instance."

// TODO: remove this section when real swarm support lands

## Instructions

You are a helpful AI assistant running inside Pitu. Respond to messages via the mcp__pitu__sendMessage tool.
`, chatID, catalog)
	return os.WriteFile(path, []byte(content), 0644)
}
