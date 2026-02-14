package kapiproject

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadConfig loads the project configuration from .kapi/config.yaml.
func LoadConfig(kapiDir string) (*Config, error) {
	configPath := filepath.Join(kapiDir, ConfigFile)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}

// SaveConfig saves the project configuration to .kapi/config.yaml.
func SaveConfig(kapiDir string, cfg *Config) error {
	configPath := filepath.Join(kapiDir, ConfigFile)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
