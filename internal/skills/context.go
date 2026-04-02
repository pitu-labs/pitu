package skills

import (
	"os"
	"path/filepath"
	"strings"
)

// WriteContext always refreshes AGENTS.md with the current agent config and skills,
// and creates CONTEXT.md (the agent's mutable memory scratch-pad) only if absent.
// Splitting these files means identity/persona changes are picked up on the next message
// without wiping any notes the agent has accumulated in CONTEXT.md.
func WriteContext(dir, chatID string, discovered []Skill, agent AgentConfig) error {
	if err := writeSystem(dir, chatID, discovered, agent); err != nil {
		return err
	}
	return writeMemory(dir)
}

func writeSystem(dir, chatID string, discovered []Skill, agent AgentConfig) error {
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
		b.WriteString("Respond to each message with a single mcp__pitu__sendMessage call. Do not call it more than once per message.\n")
	} else {
		b.WriteString("You are a helpful AI assistant running inside Pitu. Respond to each message with a single mcp__pitu__sendMessage call. Do not call it more than once per message.\n")
	}
	b.WriteString("Optionally, before or after sending your response, you may call mcp__pitu__reactToMessage with the inbound message_id and an appropriate emoji reaction.\n")

	return os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(b.String()), 0644)
}

func writeMemory(dir string) error {
	path := filepath.Join(dir, "CONTEXT.md")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists — preserve accumulated memory
	}
	return os.WriteFile(path, []byte("# Memory\n\nUse this file to record notes, reminders, and context across conversations.\n"), 0644)
}
