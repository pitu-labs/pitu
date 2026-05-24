package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// knownCapabilities lists every capability pitu-mcp can register tools for.
// Later PRs append "gmail", "gcalendar". listCapabilities reports this as "available".
var knownCapabilities = []string{"noop"}

// defaultCapabilityTimeout bounds how long a capability tool waits for the harness.
const defaultCapabilityTimeout = 30 * time.Second

func parseCapabilities(env string) []string {
	var out []string
	for _, p := range strings.Split(env, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func isCapabilityEnabled(caps []string, name string) bool {
	for _, c := range caps {
		if c == name {
			return true
		}
	}
	return false
}

// registerCapabilityTools registers the always-on listCapabilities tool plus the
// tool families for each enabled capability. Called from buildServer.
func registerCapabilityTools(s *server.MCPServer, h *toolHandlers, enabled []string) {
	// listCapabilities — always available, read-only.
	s.AddTool(mcp.NewTool("listCapabilities",
		mcp.WithDescription("List the capabilities enabled for this chat and which are available to enable. Use this to honestly report what you can and cannot do."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]any{
			"enabled":   enabled,
			"available": knownCapabilities,
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})

	// noop capability — development/validation only.
	if isCapabilityEnabled(enabled, "noop") {
		s.AddTool(mcp.NewTool("noop.ping",
			mcp.WithDescription("Validation tool: echoes its message back through the harness request/response IPC path."),
			mcp.WithString("msg", mcp.Required(), mcp.Description("Message to echo")),
		), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			msg := req.GetString("msg", "")
			result, err := h.dispatchCapability("noop", "noop.ping", map[string]any{"msg": msg}, defaultCapabilityTimeout)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, err := json.Marshal(result)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(string(data)), nil
		})
	}
}
