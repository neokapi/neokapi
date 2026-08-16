// Package venue holds the command-side policy for talking to a venue: which
// server a command contacts, and (as the built-in venue verbs land here) how
// their results are reported.
//
// It sits in the cli module rather than under host because resolving "which
// server did the person mean" reads the CLI's config system — the layered
// files, the environment bindings, the flag precedence — none of which the
// venue protocol itself needs. Keeping it here is what lets host/venue/auth
// stay a bare device-grant implementation: the AGPL server imports that package
// for its own token endpoint, and it should not inherit a config stack to do
// so.
package venue

import (
	cliconfig "github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/venue/config"
	"github.com/neokapi/neokapi/host/venue/schema"
)

// AppConfig reads the venue's per-machine settings, layered over the shared
// kapi config: venue settings (server.url) come from the venue's own file,
// while plugins, formats and flow come from kapi's.
//
// server.url gets no default on purpose. An unset value has to stay empty so
// resolution can fall through to the stored login, and so a command that merely
// *records* a server — init writing a recipe — never bakes in a URL nobody
// configured. Commands that *contact* a server apply the hosted default as the
// last step, via ResolveServerURLOrDefault.
func AppConfig() *cliconfig.AppConfig {
	return cliconfig.NewOverlayAppConfig("bowrain", func(cfg *cliconfig.AppConfig) {
		_ = cfg.Viper().BindEnv("server.url", "BOWRAIN_SERVER_URL")
	})
}

// ResolveServerURLFrom resolves the venue URL from an explicit value first,
// then the per-machine config (env + file), then the stored login. It returns
// "" when nothing is configured: a caller that contacts a server falls back to
// the hosted default via ResolveServerURLOrDefault, while a caller that merely
// records a server treats "" as "leave the recipe serverless".
func ResolveServerURLFrom(explicit string) string {
	if explicit != "" {
		return config.NormalizeServerURL(explicit)
	}
	cfg := AppConfig()
	_ = cfg.Load()
	if u := cfg.GetString("server.url"); u != "" {
		// The configured value is a compound project URL —
		// <server>/<workspace>/<project-id> — so reduce it to the origin before
		// anything appends an API path. Normalizing alone keeps the path, and
		// POSTing to /<workspace>/<project>/api/v1/... lands outside the CDN's
		// /api/* behaviour, which rejects the method before it reaches the
		// server. That is how device login failed with a CloudFront 403.
		return config.NormalizeServerURL(schema.ParseProjectURL(u).ServerURL)
	}
	if stored, err := config.LoadAuth(); err == nil && stored.ServerURL != "" {
		return config.NormalizeServerURL(stored.ServerURL)
	}
	return ""
}

// ResolveServerURLOrDefault is ResolveServerURLFrom with the hosted instance as
// the final fallback. Use it for commands that contact a server; a self-hosted
// deployment overrides via --server or BOWRAIN_SERVER_URL.
func ResolveServerURLOrDefault(explicit string) string {
	if u := ResolveServerURLFrom(explicit); u != "" {
		return u
	}
	return config.DefaultServerURL
}
