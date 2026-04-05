package container

import (
	"encoding/json"
	"fmt"

	"github.com/pitu-dev/pitu/internal/config"
)

// GenerateOpenCodeConfig returns the JSON string for OPENCODE_CONFIG_CONTENT.
// It registers pitu-mcp as a stdio MCP server and, when model config is provided,
// injects the provider and model fields so OpenCode can authenticate without
// interactive setup.
func GenerateOpenCodeConfig(chatID string, model config.ModelConfig) string {
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

	if model.Provider != "" && model.Model != "" {
		cfg["model"] = model.Provider + "/" + model.Model
		cfg["provider"] = buildProviderBlock(model)
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		panic(fmt.Sprintf("container: marshal opencode config: %v", err))
	}
	return string(data)
}

// GeneratePiMonoConfig returns the JSON string for PI_CONFIG_CONTENT.
func GeneratePiMonoConfig(chatID string, model config.ModelConfig) string {
	return GenerateOpenCodeConfig(chatID, model)
}

// buildProviderBlock constructs the OpenCode provider config object for the given model.
// Built-in providers (anthropic, openai) use native options only.
// Providers with a BaseURL use @ai-sdk/openai-compatible (covers Ollama and custom endpoints).
func buildProviderBlock(model config.ModelConfig) map[string]any {
	opts := map[string]any{}
	// API key is passed via provider-specific env var, not embedded in config JSON
	if model.BaseURL != "" {
		opts["baseURL"] = model.BaseURL
	}

	providerCfg := map[string]any{"options": opts}
	if model.BaseURL != "" {
		// Ollama and custom OpenAI-compatible endpoints require the community adapter
		providerCfg["npm"] = "@ai-sdk/openai-compatible"
	}

	return map[string]any{model.Provider: providerCfg}
}

// APIKeyEnvVar returns the standard environment variable name for the given
// provider's API key. The key is written to the container's env file separately
// from the OpenCode config JSON so it does not appear in config blobs.
func APIKeyEnvVar(provider string) string {
	switch provider {
	case "openai":
		return "OPENAI_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	default: // "anthropic" and openai-compatible providers
		return "ANTHROPIC_API_KEY"
	}
}
