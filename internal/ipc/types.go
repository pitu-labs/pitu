package ipc

// InboundMessage is written by the harness to ipc/input/ and passed to OpenCode.
type InboundMessage struct {
	ChatID    string `json:"chat_id"`
	From      string `json:"from"`
	Text      string `json:"text"`
	MessageID string `json:"message_id"`
}

// OutboundMessage is written by pitu-mcp to ipc/messages/.
type OutboundMessage struct {
	ChatID     string `json:"chat_id"`
	Text       string `json:"text"`
	Type       string `json:"type"`
	Role       string `json:"role,omitempty"`          // New: source role
	SubAgentID string `json:"sub_agent_id,omitempty"`  // New: source agent ID
}

// TaskFile is written by pitu-mcp to ipc/tasks/.
type TaskFile struct {
	Action   string `json:"action"`       // "create" | "pause"
	ID       string `json:"id,omitempty"` // required for "pause"
	Name     string `json:"name,omitempty"`
	Schedule string `json:"schedule,omitempty"` // cron or RFC3339
	Prompt   string `json:"prompt,omitempty"`
	ChatID   string `json:"chat_id"`
}

// GroupFile is written by pitu-mcp to ipc/groups/.
type GroupFile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ChatID      string `json:"chat_id"`
}

// AgentFile is written by pitu-mcp to ipc/agents/ to request a sub-agent spawn.
type AgentFile struct {
	Action     string `json:"action"`       // "spawn"
	SubAgentID string `json:"sub_agent_id"` // reserved for future result correlation; not used by harness today
	Role       string `json:"role"`
	Prompt     string `json:"prompt"`
	ChatID     string `json:"chat_id"`
}

// ReactionFile is written by pitu-mcp to ipc/reactions/ to set an emoji reaction on a message.
type ReactionFile struct {
	ChatID    string `json:"chat_id"`
	MessageID int    `json:"message_id"`
	Emoji     string `json:"emoji"`
}
