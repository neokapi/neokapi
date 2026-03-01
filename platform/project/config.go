package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// LoadConfig loads the project configuration from .bowrain/config.yaml.
func LoadConfig(configDir string) (*Config, error) {
	configPath := filepath.Join(configDir, ConfigFile)

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

// GetConfigValue reads a dot-notation key from .bowrain/config.yaml.
// For example, "project.name" or "server.url".
func GetConfigValue(configDir, key string) string {
	configPath := filepath.Join(configDir, ConfigFile)
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	_ = v.ReadInConfig()
	return v.GetString(key)
}

// SetConfigValue sets a dot-notation key in .bowrain/config.yaml.
// The file is loaded, updated, and written back.
func SetConfigValue(configDir, key, value string) error {
	configPath := filepath.Join(configDir, ConfigFile)
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	_ = v.ReadInConfig()
	v.Set(key, value)
	return v.WriteConfigAs(configPath)
}

// SaveConfig saves the project configuration to .bowrain/config.yaml.
func SaveConfig(configDir string, cfg *Config) error {
	configPath := filepath.Join(configDir, ConfigFile)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
