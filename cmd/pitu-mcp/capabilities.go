package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/pitu-dev/pitu/internal/ipc"
)

// defaultCapabilityTimeout bounds how long a capability tool waits for the harness.
const defaultCapabilityTimeout = 30 * time.Second

// capabilityTool describes one MCP tool belonging to a capability. Bundling the
// param schema and handler with the name/description keeps a tool's full definition
// in one place rather than scattered across registration code — important once a
// capability (e.g. gmail) exposes many tools. The handler typically calls
// h.dispatchCapability to round-trip through the harness.
type capabilityTool struct {
	name        string
	description string
	params      []mcp.ToolOption
	handler     func(h *toolHandlers, req mcp.CallToolRequest) (any, error)
}

// capability groups the tools registered when a named capability is enabled for a chat.
type capability struct {
	name  string
	tools []capabilityTool
}

// capabilityRegistry is the single source of truth for which capabilities exist and
// what tools each registers. New capabilities (gmail, gcalendar, …) append an entry.
var capabilityRegistry = []capability{
	{
		name: ipc.CapabilityNoop, // development/validation only
		tools: []capabilityTool{
			{
				name:        "noop.ping",
				description: "Validation tool: echoes its message back through the harness request/response IPC path.",
				params:      []mcp.ToolOption{mcp.WithString("msg", mcp.Required(), mcp.Description("Message to echo"))},
				handler: func(h *toolHandlers, req mcp.CallToolRequest) (any, error) {
					msg := req.GetString("msg", "")
					return h.dispatchCapability(ipc.CapabilityNoop, "noop.ping", map[string]any{"msg": msg}, defaultCapabilityTimeout)
				},
			},
		},
	},
}

// knownCapabilityNames returns every capability name the registry knows about.
// listCapabilities reports this as "available".
func knownCapabilityNames() []string {
	out := make([]string, len(capabilityRegistry))
	for i, c := range capabilityRegistry {
		out[i] = c.name
	}
	return out
}

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

// registerCapabilityTools registers the always-on listCapabilities tool plus the tool
// families for each enabled capability, driven by capabilityRegistry. Called from buildServer.
func registerCapabilityTools(s *server.MCPServer, h *toolHandlers, enabled []string) {
	// listCapabilities — always available, read-only.
	s.AddTool(mcp.NewTool("listCapabilities",
		mcp.WithDescription("List the capabilities enabled for this chat and which are available to enable. Use this to honestly report what you can and cannot do."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonToolResult(map[string]any{
			"enabled":   enabled,
			"available": knownCapabilityNames(),
		})
	})

	for _, cap := range capabilityRegistry {
		if !isCapabilityEnabled(enabled, cap.name) {
			continue
		}
		for _, tool := range cap.tools {
			tool := tool // capture per iteration
			opts := append([]mcp.ToolOption{mcp.WithDescription(tool.description)}, tool.params...)
			s.AddTool(mcp.NewTool(tool.name, opts...),
				func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					result, err := tool.handler(h, req)
					if err != nil {
						return mcp.NewToolResultError(err.Error()), nil
					}
					return jsonToolResult(result)
				})
		}
	}
}

// jsonToolResult marshals v to JSON and wraps it as an MCP text result, matching the
// result convention used by the other pitu-mcp tools in server.go.
func jsonToolResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}
