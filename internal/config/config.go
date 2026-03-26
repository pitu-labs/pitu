package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Telegram  TelegramConfig  `toml:"telegram"`
	Container ContainerConfig `toml:"container"`
	Skills    SkillsConfig    `toml:"skills"`
	DB        DBConfig        `toml:"db"`
}

type TelegramConfig struct {
	BotToken string `toml:"bot_token"`
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
	return cfg, nil
}
