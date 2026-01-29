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

// NewAppConfig creates a config reader that searches for gokapi.yaml
// in standard locations.
func NewAppConfig() *AppConfig {
	v := viper.New()
	v.SetConfigName("gokapi")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("$HOME/.config/gokapi")
	v.AddConfigPath("/etc/gokapi")
	v.SetEnvPrefix("GOKAPI")
	v.AutomaticEnv()

	// Set defaults
	v.SetDefault("flow.channelBuffer", 64)
	pluginDir := "./plugins"
	if home, err := os.UserHomeDir(); err == nil {
		pluginDir = filepath.Join(home, ".kapi", "plugins")
	}
	v.SetDefault("plugins.directory", pluginDir)

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
func (c *AppConfig) Set(key string, value interface{}) {
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
