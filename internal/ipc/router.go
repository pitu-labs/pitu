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
func (r *Router) Route(subdir, path, role, subAgentID string) error {
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
                m.Role = role
                m.SubAgentID = subAgentID
                r.onMessage(m)
        case "tasks":		var t TaskFile
		if err := json.Unmarshal(data, &t); err != nil {
			return fmt.Errorf("ipc: unmarshal task: %w", err)
		}
		r.onTask(t)
	case "groups":
		var g GroupFile
		if err := json.Unmarshal(data, &g); err != nil {
			return fmt.Errorf("ipc: unmarshal group: %w", err)
		}
		r.onGroup(g)
	case "agents":
		var a AgentFile
		if err := json.Unmarshal(data, &a); err != nil {
			return fmt.Errorf("ipc: unmarshal agent: %w", err)
		}
		r.onAgent(a)
	case "reactions":
		var rf ReactionFile
		if err := json.Unmarshal(data, &rf); err != nil {
			return fmt.Errorf("ipc: unmarshal reaction: %w", err)
		}
		r.onReaction(rf)
	default:
		return fmt.Errorf("ipc: unknown subdir %q", subdir)
	}
	return nil
}
