package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// expandHome replaces a leading "~/" with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return home + path[1:]
}

type Config struct {
	Telegram  TelegramConfig  `toml:"telegram"`
	Container ContainerConfig `toml:"container"`
	Skills    SkillsConfig    `toml:"skills"`
	DB        DBConfig        `toml:"db"`
	Model     ModelConfig     `toml:"model"`
}

type TelegramConfig struct {
	BotToken       string  `toml:"bot_token"`
	AllowedChatIDs []int64 `toml:"allowed_chat_ids"`
	RateLimit      string  `toml:"rate_limit"`
}

type ContainerConfig struct {
	Image         string `toml:"image"`
	TTL           string `toml:"ttl"`
	MaxConcurrent int    `toml:"max_concurrent"`
	MemoryLimit   string `toml:"memory_limit"`
}

type SkillsConfig struct {
	ExtraPaths []string `toml:"extra_paths"`
}

type DBConfig struct {
	Path string `toml:"path"`
}

type ModelConfig struct {
	Provider string `toml:"provider"` // e.g. "anthropic", "openai", "ollama"
	Model    string `toml:"model"`    // model name without provider prefix, e.g. "claude-sonnet-4-5"
	APIKey   string `toml:"api_key"`  // leave empty for Ollama
	BaseURL  string `toml:"base_url"` // required for Ollama and OpenAI-compatible endpoints
}

func Load(path string) (*Config, error) {
	cfg := &Config{}
	cfg.Container.TTL = "5m"
	cfg.Container.MaxConcurrent = 5
	cfg.Container.MemoryLimit = "512m"

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("config: decode %s: %w", path, err)
	}
	if cfg.Telegram.BotToken == "" {
		return nil, fmt.Errorf("config: bot_token is required")
	}
	cfg.DB.Path = expandHome(cfg.DB.Path)
	return cfg, nil
}
