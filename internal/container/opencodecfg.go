package container

import (
	"encoding/json"
	"fmt"
)

// GenerateOpenCodeConfig returns the JSON string for OPENCODE_CONFIG_CONTENT.
// It registers pitu-mcp as a stdio MCP server and injects PITU_CHAT_ID.
func GenerateOpenCodeConfig(chatID string) string {
	cfg := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"mcp": map[string]any{
			"pitu": map[string]any{
				"type":    "local",
				"command": []string{"/usr/local/bin/pitu-mcp"},
				"environment": map[string]string{
					"PITU_CHAT_ID": chatID,
				},
				"enabled": true,
			},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		panic(fmt.Sprintf("container: marshal opencode config: %v", err))
	}
	return string(data)
}
