package commands

import (
	cliconfig "github.com/neokapi/neokapi/host/config"
)

// newBowrainAppConfig creates a config reader for bowrain that layers
// bowrain-specific config (~/.config/bowrain/bowrain.yaml) on top of the
// shared kapi config. Bowrain-specific settings like server.url are read
// from the bowrain config; shared settings (plugins, formats, flow) come
// from the kapi config.
func newBowrainAppConfig() *cliconfig.AppConfig {
	return cliconfig.NewOverlayAppConfig("bowrain", func(cfg *cliconfig.AppConfig) {
		// No SetDefault for server.url: an unset value must stay empty so
		// resolution can fall through to the stored login, and so commands
		// that merely *record* a server (init writing a recipe) never bake in
		// a URL nobody configured. Commands that *contact* a server fall back
		// to config.DefaultServerURL (the hosted instance) as the last step.
		_ = cfg.Viper().BindEnv("server.url", "BOWRAIN_SERVER_URL")
	})
}
