package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// AppConfig holds application-level configuration loaded via Viper.
type AppConfig struct {
	v *viper.Viper
}

// NewAppConfig creates a config reader that searches for kapi.yaml
// in standard locations (plugins, formats, flow config).
func NewAppConfig() *AppConfig {
	v := viper.New()
	v.SetConfigName("kapi")
	v.SetConfigType("yaml")
	// Honor KAPI_CONFIG_DIR first, mirroring GlobalConfigFilePath. This keeps
	// in-repo (dogfood) kapi invocations isolated from the developer's real
	// ~/.config/kapi when KAPI_CONFIG_DIR points at a throwaway dir.
	if dir := os.Getenv("KAPI_CONFIG_DIR"); dir != "" {
		v.AddConfigPath(dir)
	}
	v.AddConfigPath(".")
	v.AddConfigPath("$HOME/.config/kapi")
	v.AddConfigPath("/etc/kapi")
	v.SetEnvPrefix("KAPI")
	// Translate dots to underscores so dotted keys like "plugins.directory"
	// resolve from env vars like KAPI_PLUGINS_DIRECTORY (not KAPI_PLUGINS.DIRECTORY).
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set defaults
	v.SetDefault(KeyFlowChannelBuffer, 64)
	pluginDir := "./plugins"
	if configDir, err := os.UserConfigDir(); err == nil {
		pluginDir = filepath.Join(configDir, "kapi", "plugins")
	}
	v.SetDefault(KeyPluginsDirectory, pluginDir)
	v.SetDefault(KeyPluginsRegistry, DefaultRegistryURL)

	return &AppConfig{v: v}
}

// NewOverlayAppConfig creates a config reader that layers app-specific config
// on top of the shared kapi config. App-specific settings are read from
// ~/.config/<appName>/<appName>.yaml; shared settings (plugins, formats, flow)
// come from the kapi config.
func NewOverlayAppConfig(appName string, configure func(cfg *AppConfig)) *AppConfig {
	cfg := NewAppConfig()

	overlayPath := GlobalConfigFilePath(appName)
	if _, err := os.Stat(overlayPath); err == nil {
		overlay := viper.New()
		overlay.SetConfigFile(overlayPath)
		overlay.SetConfigType("yaml")
		if err := overlay.ReadInConfig(); err != nil {
			// A present-but-unreadable overlay config is a real error; only
			// a missing file (filtered by os.Stat above) is acceptable.
			fmt.Fprintf(os.Stderr, "Warning: overlay config %s: %v\n", overlayPath, err)
		} else {
			// Merge into the file-layer (not Set) so env vars still beat the
			// overlay file and flag > env > file precedence is preserved.
			if err := cfg.v.MergeConfigMap(overlay.AllSettings()); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: merge overlay config %s: %v\n", overlayPath, err)
			}
		}
	}

	if configure != nil {
		configure(cfg)
	}

	return cfg
}

// Load reads the configuration file.
func (c *AppConfig) Load() error {
	if err := c.v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
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
	return c.v.GetInt(KeyFlowChannelBuffer)
}

// PluginDirectory returns the configured plugin directory.
func (c *AppConfig) PluginDirectory() string {
	return c.v.GetString(KeyPluginsDirectory)
}

// Language returns the configured target locale for CLI/UI output
// (BCP-47, e.g. "fr-FR"). Empty when unset — the i18n.Resolve chain
// then falls back to KAPI_LANG / LC_ALL / LANG.
func (c *AppConfig) Language() string {
	return c.v.GetString(KeyLanguage)
}

// RegistryURL returns the URL of the remote plugin registry.
func (c *AppConfig) RegistryURL() string {
	return c.v.GetString(KeyPluginsRegistry)
}

// UpdateChannel returns the release channel `kapi update` and the background
// update notifier track. Defaults to "stable"; set update.channel (or
// KAPI_UPDATE_CHANNEL) to "beta" to follow the fast track.
func (c *AppConfig) UpdateChannel() string {
	if ch := c.v.GetString(KeyUpdateChannel); ch != "" {
		return ch
	}
	return DefaultUpdateChannel
}

// RegistryEntry represents a named plugin registry.
type RegistryEntry struct {
	Name     string   `yaml:"name"               json:"name"`
	URL      string   `yaml:"url"                json:"url"`
	Channels []string `yaml:"channels,omitempty" json:"channels,omitempty"`
}

// DefaultRegistryURL is the official neokapi plugin registry.
const DefaultRegistryURL = "https://neokapi.github.io/registry/manifest-plugins.json"

// DefaultUpdateChannel is the release channel kapi follows unless overridden.
const DefaultUpdateChannel = "stable"

// Config key constants for use with Get/Set/BindEnv to avoid scattered magic strings.
const (
	KeyFlowChannelBuffer = "flow.channelBuffer"
	KeyPluginsDirectory  = "plugins.directory"
	KeyPluginsRegistry   = "plugins.registry"
	KeyFormatsPriorities = "formats.priorities"
	KeyLanguage          = "language"
	KeyUpdateChannel     = "update.channel"
)

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
	url := c.v.GetString(KeyPluginsRegistry)
	if url == "" {
		url = DefaultRegistryURL
	}
	return []RegistryEntry{{Name: "default", URL: url}}
}

// parseRegistryEntries converts Viper's raw any into []RegistryEntry.
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

// parseStringSlice converts an any to []string.
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

// GlobalConfigFilePath returns the path to the global config file
// for the given app name (e.g. ~/.config/kapi/kapi.yaml).
// If no app name is provided, defaults to "kapi".
func GlobalConfigFilePath(appName ...string) string {
	name := "kapi"
	if len(appName) > 0 && appName[0] != "" {
		name = appName[0]
	}
	envKey := strings.ToUpper(name) + "_CONFIG_DIR"
	if dir := os.Getenv(envKey); dir != "" {
		return filepath.Join(dir, name+".yaml")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, name, name+".yaml")
}

// SetGlobalConfig sets a key-value pair in the global config file.
// The file is loaded, updated, and written back as YAML.
// If an app name is provided, it determines the config path;
// otherwise defaults to "kapi".
func SetGlobalConfig(key, value string, appName ...string) error {
	path := GlobalConfigFilePath(appName...)

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
