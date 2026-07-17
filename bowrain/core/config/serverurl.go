package config

import (
	"net/url"
	"strings"
)

// DefaultServerURL is the hosted Bowrain instance, used when a command that
// contacts a server has no URL configured anywhere (flag, env var, config
// file, stored login). Self-hosted deployments set BOWRAIN_SERVER_URL or pass
// --server; project commands read the recipe's server: block and never need
// this default.
const DefaultServerURL = "https://app.bowrain.cloud"

// NormalizeServerURL canonicalizes a user-supplied Bowrain server URL. It
// trims a trailing slash, and rewrites the hosted service's landing-page
// hosts (bowrain.cloud, www.bowrain.cloud) to the app host
// (app.bowrain.cloud): the top-level domain serves the marketing site, so an
// API call against it would not reach the platform. Any other URL is returned
// unchanged apart from the trailing-slash trim.
func NormalizeServerURL(raw string) string {
	if raw == "" {
		return ""
	}
	s := strings.TrimRight(raw, "/")
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return s
	}
	switch strings.ToLower(u.Hostname()) {
	case "bowrain.cloud", "www.bowrain.cloud":
		u.Scheme = "https"
		u.Host = "app.bowrain.cloud"
		return strings.TrimRight(u.String(), "/")
	}
	return s
}
