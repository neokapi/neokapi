package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gokapi/gokapi/core/config"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const (
	// ProjectConfigVersion is the current schema version for config.yaml.
	ProjectConfigVersion = "v1"
)

// LoadConfig loads the project configuration from .bowrain/config.yaml.
//
// Supports three formats (detected automatically):
//  1. Flat with version (current): version + fields at top level
//  2. Envelope (legacy): apiVersion + kind + metadata + spec
//  3. Bare YAML (legacy): no version or envelope
//
// Legacy formats are read transparently — the in-memory Config is the same.
func LoadConfig(configDir string) (*Config, error) {
	configPath := filepath.Join(configDir, ConfigFile)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Probe for envelope format (legacy)
	var probe struct {
		APIVersion string `yaml:"apiVersion"`
	}
	_ = yaml.Unmarshal(data, &probe)

	if probe.APIVersion != "" {
		return loadEnvelopedConfig(data)
	}

	// Flat YAML (current format or bare legacy)
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// loadEnvelopedConfig parses a legacy enveloped project config.
func loadEnvelopedConfig(data []byte) (*Config, error) {
	env, err := config.Parse(data, ".yaml")
	if err != nil {
		return nil, fmt.Errorf("parse enveloped config: %w", err)
	}

	if env.Kind != config.KindProjectConfig {
		return nil, fmt.Errorf("expected kind %q, got %q", config.KindProjectConfig, env.Kind)
	}

	// Apply migrations if needed
	if err := config.DefaultMigrations.Upgrade(env); err != nil {
		return nil, fmt.Errorf("migrate config: %w", err)
	}

	// Re-marshal the spec to YAML and unmarshal into Config
	specData, err := yaml.Marshal(env.Spec)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(specData, &cfg); err != nil {
		return nil, fmt.Errorf("parse config spec: %w", err)
	}

	// Set version from the envelope's apiVersion.
	cfg.Version = env.APIVersion
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

	// Try direct key first, then try under "spec." for legacy enveloped configs
	val := v.GetString(key)
	if val == "" {
		val = v.GetString("spec." + key)
	}
	return val
}

// SetConfigValue sets a dot-notation key in .bowrain/config.yaml.
// The file is loaded, updated, and written back.
func SetConfigValue(configDir, key, value string) error {
	configPath := filepath.Join(configDir, ConfigFile)
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	_ = v.ReadInConfig()

	// If this is a legacy enveloped config, set under "spec."
	if v.GetString("apiVersion") != "" {
		v.Set("spec."+key, value)
	} else {
		v.Set(key, value)
	}
	return v.WriteConfigAs(configPath)
}

// SaveConfig saves the project configuration to .bowrain/config.yaml
// as flat YAML with a top-level version field.
func SaveConfig(configDir string, cfg *Config) error {
	configPath := filepath.Join(configDir, ConfigFile)

	// Ensure the version is set.
	cfg.Version = ProjectConfigVersion

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
