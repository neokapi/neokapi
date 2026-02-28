package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// AppConfig holds application-level configuration loaded via Viper.
type AppConfig struct {
	v *viper.Viper
}

// NewAppConfig creates a config reader that searches for kapi.yaml
// in standard locations.
func NewAppConfig() *AppConfig {
	v := viper.New()
	v.SetConfigName("kapi")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("$HOME/.config/kapi")
	v.AddConfigPath("/etc/kapi")
	v.SetEnvPrefix("KAPI")
	v.AutomaticEnv()

	// Set defaults
	v.SetDefault("server.url", "http://localhost:8080")
	v.SetDefault("flow.channelBuffer", 64)
	pluginDir := "./plugins"
	if configDir, err := os.UserConfigDir(); err == nil {
		pluginDir = filepath.Join(configDir, "gokapi", "plugins")
	}
	v.SetDefault("plugins.directory", pluginDir)
	v.SetDefault("plugins.registry", "https://gokapi.github.io/registry/plugins.json")

	// Explicit env bindings for keys that don't follow simple prefix rules.
	_ = v.BindEnv("server.url", "KAPI_SERVER_URL")

	return &AppConfig{v: v}
}

// Load reads the configuration file.
func (c *AppConfig) Load() error {
	if err := c.v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil // Config file not found is ok
		}
		return err
	}
	return nil
}

// Viper returns the underlying Viper instance for advanced access.
func (c *AppConfig) Viper() *viper.Viper {
	return c.v
}

// GetString returns a string config value.
func (c *AppConfig) GetString(key string) string {
	return c.v.GetString(key)
}

// GetInt returns an integer config value.
func (c *AppConfig) GetInt(key string) int {
	return c.v.GetInt(key)
}

// GetBool returns a boolean config value.
func (c *AppConfig) GetBool(key string) bool {
	return c.v.GetBool(key)
}

// Set sets a config value.
func (c *AppConfig) Set(key string, value any) {
	c.v.Set(key, value)
}

// ChannelBuffer returns the configured channel buffer size.
func (c *AppConfig) ChannelBuffer() int {
	return c.v.GetInt("flow.channelBuffer")
}

// PluginDirectory returns the configured plugin directory.
func (c *AppConfig) PluginDirectory() string {
	return c.v.GetString("plugins.directory")
}

// RegistryURL returns the URL of the remote plugin registry.
func (c *AppConfig) RegistryURL() string {
	return c.v.GetString("plugins.registry")
}

// RegistryEntry represents a named plugin registry.
type RegistryEntry struct {
	Name     string   `yaml:"name"               json:"name"`
	URL      string   `yaml:"url"                json:"url"`
	Channels []string `yaml:"channels,omitempty" json:"channels,omitempty"`
}

// DefaultRegistryURL is the official gokapi plugin registry.
const DefaultRegistryURL = "https://gokapi.github.io/registry/plugins.json"

// Registries returns the configured list of plugin registries.
// If the "registries" key is set, those entries are returned.
// Otherwise, falls back to "plugins.registry" wrapped as a single entry named "default".
func (c *AppConfig) Registries() []RegistryEntry {
	raw := c.v.Get("registries")
	if raw != nil {
		if entries := parseRegistryEntries(raw); len(entries) > 0 {
			return entries
		}
	}
	// Fallback to single registry URL.
	url := c.v.GetString("plugins.registry")
	if url == "" {
		url = DefaultRegistryURL
	}
	return []RegistryEntry{{Name: "default", URL: url}}
}

// parseRegistryEntries converts Viper's raw interface{} into []RegistryEntry.
// Handles both []any (from YAML) and []map[string]any (from Set).
func parseRegistryEntries(raw any) []RegistryEntry {
	switch v := raw.(type) {
	case []any:
		var entries []RegistryEntry
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if e, ok := registryEntryFromMap(m); ok {
					entries = append(entries, e)
				}
			}
		}
		return entries
	case []map[string]any:
		var entries []RegistryEntry
		for _, m := range v {
			if e, ok := registryEntryFromMap(m); ok {
				entries = append(entries, e)
			}
		}
		return entries
	default:
		return nil
	}
}

// registryEntryFromMap extracts a RegistryEntry from a map.
func registryEntryFromMap(m map[string]any) (RegistryEntry, bool) {
	name, _ := m["name"].(string)
	url, _ := m["url"].(string)
	if name == "" || url == "" {
		return RegistryEntry{}, false
	}
	e := RegistryEntry{Name: name, URL: url}
	if raw, ok := m["channels"]; ok {
		e.Channels = parseStringSlice(raw)
	}
	return e, true
}

// parseStringSlice converts an interface{} to []string.
func parseStringSlice(raw any) []string {
	switch v := raw.(type) {
	case []any:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

// ServerURL returns the configured Bowrain Server URL.
// Resolved from config file (server.url) or KAPI_SERVER_URL env var.
func (c *AppConfig) ServerURL() string {
	return c.v.GetString("server.url")
}

// GlobalConfigFilePath returns the path to the global config file (~/.config/kapi/kapi.yaml).
func GlobalConfigFilePath() string {
	if dir := os.Getenv("KAPI_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "kapi.yaml")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "kapi", "kapi.yaml")
}

// SetGlobalConfig sets a key-value pair in the global config file.
// The file is loaded, updated, and written back as YAML.
func SetGlobalConfig(key, value string) error {
	path := GlobalConfigFilePath()

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Try to read existing config (ignore not-found).
	_ = v.ReadInConfig()

	v.Set(key, value)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return v.WriteConfigAs(path)
}

// FormatPriorities returns the configured format priority overrides.
// The map keys are format names and values are priority integers.
// Higher values are preferred when multiple formats match the same
// MIME type or file extension.
func (c *AppConfig) FormatPriorities() map[string]int {
	result := make(map[string]int)
	sub := c.v.GetStringMap("formats.priorities")
	for name, val := range sub {
		switch v := val.(type) {
		case int:
			result[name] = v
		case int64:
			result[name] = int(v)
		case float64:
			result[name] = int(v)
		}
	}
	return result
}
