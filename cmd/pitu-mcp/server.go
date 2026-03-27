package main

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func buildServer(h *toolHandlers) *server.MCPServer {
	s := server.NewMCPServer("pitu-mcp", "1.0.0")

	s.AddTool(mcp.NewTool("sendMessage",
		mcp.WithDescription("Send a message to the originating Telegram chat"),
		mcp.WithString("text", mcp.Required(), mcp.Description("Message text")),
		mcp.WithString("sender", mcp.Required(), mcp.Description("Sender display name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text := req.GetString("text", "")
		sender := req.GetString("sender", "")
		result, err := h.handleSendMessage(text, sender)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})

	s.AddTool(mcp.NewTool("scheduleTask",
		mcp.WithDescription("Create a recurring or one-shot scheduled task"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Task name")),
		mcp.WithString("schedule", mcp.Required(), mcp.Description("Cron expression or RFC3339 timestamp")),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("Prompt to execute when the task fires")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		schedule := req.GetString("schedule", "")
		prompt := req.GetString("prompt", "")
		result, err := h.handleScheduleTask(name, schedule, prompt)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})

	s.AddTool(mcp.NewTool("listTasks",
		mcp.WithDescription("Enqueues a list-tasks request for this chat (response is processed asynchronously by the host)"),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := h.handleListTasks()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})

	s.AddTool(mcp.NewTool("pauseTask",
		mcp.WithDescription("Pause a scheduled task by ID"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Task UUID returned by scheduleTask")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := req.GetString("id", "")
		result, err := h.handlePauseTask(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})

	s.AddTool(mcp.NewTool("registerGroup",
		mcp.WithDescription("Register a new addressable group (main agent only)"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Group name")),
		mcp.WithString("description", mcp.Required(), mcp.Description("Group description")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		desc := req.GetString("description", "")
		result, err := h.handleRegisterGroup(name, desc)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})

	// TODO: remove stub and implement when real swarm support lands.
	// The tool signature (role, prompt) is intentionally stable — it becomes the production API.
	s.AddTool(mcp.NewTool("spawnAgent",
		mcp.WithDescription("Spawn a sub-agent for a delegated task (not yet supported)"),
		mcp.WithString("role", mcp.Required(), mcp.Description("Sub-agent role or name")),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("Task to delegate to the sub-agent")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		role := req.GetString("role", "")
		prompt := req.GetString("prompt", "")
		result, err := h.handleSpawnAgent(role, prompt)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		// Success branch: unreachable while stub is in place; activate when real implementation lands.
		return mcp.NewToolResultText(result), nil
	})

	return s
}
