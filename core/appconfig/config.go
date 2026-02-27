// Package appconfig provides lightweight application configuration for kapi.
// It reads ~/.config/kapi/kapi.yaml using only stdlib + yaml.v3 (no Viper).
package appconfig

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// configData represents the YAML structure of kapi.yaml.
type configData struct {
	Plugins struct {
		Directory string `yaml:"directory"`
		Registry  string `yaml:"registry"`
	} `yaml:"plugins"`
	Formats struct {
		Priorities map[string]int `yaml:"priorities"`
	} `yaml:"formats"`
}

// AppConfig holds application-level configuration for kapi.
type AppConfig struct {
	data configData
}

// New creates a new AppConfig with defaults applied.
func New() *AppConfig {
	c := &AppConfig{}

	// Set defaults.
	c.data.Plugins.Directory = defaultPluginDir()
	c.data.Plugins.Registry = "https://gokapi.github.io/registry/plugins.json"

	return c
}

// Load reads the kapi.yaml configuration file from standard locations.
// If no config file is found, defaults are used (not an error).
func (c *AppConfig) Load() error {
	path := findConfigFile()
	if path == "" {
		return nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Unmarshal on top of defaults so unset keys keep their default values.
	if err := yaml.Unmarshal(raw, &c.data); err != nil {
		return err
	}

	// Restore defaults for empty fields.
	if c.data.Plugins.Directory == "" {
		c.data.Plugins.Directory = defaultPluginDir()
	}
	if c.data.Plugins.Registry == "" {
		c.data.Plugins.Registry = "https://gokapi.github.io/registry/plugins.json"
	}

	return nil
}

// PluginDirectory returns the configured plugin directory.
func (c *AppConfig) PluginDirectory() string {
	return c.data.Plugins.Directory
}

// RegistryURL returns the URL of the remote plugin registry.
func (c *AppConfig) RegistryURL() string {
	return c.data.Plugins.Registry
}

// FormatPriorities returns the configured format priority overrides.
func (c *AppConfig) FormatPriorities() map[string]int {
	if c.data.Formats.Priorities == nil {
		return map[string]int{}
	}
	return c.data.Formats.Priorities
}

// findConfigFile searches standard locations for kapi.yaml.
func findConfigFile() string {
	// Check current directory.
	if _, err := os.Stat("kapi.yaml"); err == nil {
		return "kapi.yaml"
	}

	// Check $HOME/.config/kapi/kapi.yaml.
	if configDir, err := os.UserConfigDir(); err == nil {
		p := filepath.Join(configDir, "kapi", "kapi.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Check /etc/kapi/kapi.yaml.
	if _, err := os.Stat("/etc/kapi/kapi.yaml"); err == nil {
		return "/etc/kapi/kapi.yaml"
	}

	return ""
}

// defaultPluginDir returns the default plugin directory.
func defaultPluginDir() string {
	if configDir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(configDir, "gokapi", "plugins")
	}
	return "./plugins"
}
