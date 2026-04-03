package ipc

import (
	"encoding/json"
	"fmt"
	"os"
)

type Router struct {
	onMessage  func(OutboundMessage)
	onTask     func(TaskFile)
	onGroup    func(GroupFile)
	onAgent    func(AgentFile)
	onReaction func(ReactionFile)
}

func NewRouter(onMessage func(OutboundMessage), onTask func(TaskFile), onGroup func(GroupFile), onAgent func(AgentFile), onReaction func(ReactionFile)) *Router {
	return &Router{onMessage: onMessage, onTask: onTask, onGroup: onGroup, onAgent: onAgent, onReaction: onReaction}
}

// Route reads the file at path and dispatches to the appropriate handler based on subdir.
// chatID is the authoritative chat ID derived from the filesystem path (harness-controlled);
// it overrides any chat_id value present in the container-supplied JSON.
func (r *Router) Route(subdir, path, chatID, role, subAgentID string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("ipc: read %s: %w", path, err)
	}
	switch subdir {
	case "messages":
		var m OutboundMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("ipc: unmarshal message: %w", err)
		}
		m.ChatID = chatID // override: never trust container-supplied chatID
		m.Role = role
		m.SubAgentID = subAgentID
		r.onMessage(m)
	case "tasks":
		var t TaskFile
		if err := json.Unmarshal(data, &t); err != nil {
			return fmt.Errorf("ipc: unmarshal task: %w", err)
		}
		t.ChatID = chatID
		r.onTask(t)
	case "groups":
		var g GroupFile
		if err := json.Unmarshal(data, &g); err != nil {
			return fmt.Errorf("ipc: unmarshal group: %w", err)
		}
		g.ChatID = chatID
		r.onGroup(g)
	case "agents":
		var a AgentFile
		if err := json.Unmarshal(data, &a); err != nil {
			return fmt.Errorf("ipc: unmarshal agent: %w", err)
		}
		a.ChatID = chatID
		r.onAgent(a)
	case "reactions":
		var rf ReactionFile
		if err := json.Unmarshal(data, &rf); err != nil {
			return fmt.Errorf("ipc: unmarshal reaction: %w", err)
		}
		rf.ChatID = chatID
		r.onReaction(rf)
	default:
		return fmt.Errorf("ipc: unknown subdir %q", subdir)
	}
	return nil
}
