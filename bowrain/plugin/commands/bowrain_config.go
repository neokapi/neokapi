package commands

import (
	cliconfig "github.com/neokapi/neokapi/cli/config"
)

// newBowrainAppConfig creates a config reader for bowrain that layers
// bowrain-specific config (~/.config/bowrain/bowrain.yaml) on top of the
// shared kapi config. Bowrain-specific settings like server.url are read
// from the bowrain config; shared settings (plugins, formats, flow) come
// from the kapi config.
func newBowrainAppConfig() *cliconfig.AppConfig {
	return cliconfig.NewOverlayAppConfig("bowrain", func(cfg *cliconfig.AppConfig) {
		cfg.Viper().SetDefault("server.url", "http://localhost:8080")
		_ = cfg.Viper().BindEnv("server.url", "BOWRAIN_SERVER_URL")
	})
}
