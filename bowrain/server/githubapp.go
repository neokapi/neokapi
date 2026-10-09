package server

import (
	"log/slog"

	"github.com/neokapi/neokapi/bowrain/forge"
)

// newGitHubApp builds the GitHub App from the configuration, or returns nil
// when none is configured.
//
// A half-configured app is loud rather than ignored: silently dropping it
// would strand every auth:app connector. The same holds for an API URL the
// app could not call, so a malformed GITHUB_API_URL disables the app at boot
// instead of failing every request after it.
func newGitHubApp(cfg *Config) *forge.GitHubApp {
	if cfg.GitHubAppID == "" && cfg.GitHubAppPrivateKey == "" && cfg.GitHubAppWebhookSecret == "" {
		return nil
	}
	app, err := forge.NewGitHubApp(cfg.GitHubAppID, cfg.GitHubAppPrivateKey, cfg.GitHubAppWebhookSecret)
	if err != nil {
		slog.Error("github app disabled (incomplete or invalid GITHUB_APP_* config)", "error", err)
		return nil
	}
	if cfg.GitHubAPIURL != "" {
		if err := app.SetAPIBase(cfg.GitHubAPIURL); err != nil {
			slog.Error("github app disabled (GITHUB_API_URL is not usable)", "error", err)
			return nil
		}
	}
	slog.Info("github app enabled for forge delivery", "app_id", cfg.GitHubAppID, "api", app.APIBase())
	return app
}
