package config

import (
	"net/url"
	"strings"

	coreproj "github.com/neokapi/neokapi/core/project"
)

// DefaultServerURL is the hosted Bowrain instance, used when a command that
// contacts a server has no URL configured anywhere (flag, env var, config
// file, stored login). Self-hosted deployments set BOWRAIN_SERVER_URL or pass
// --server; project commands read the recipe's bowrain: block and never need
// this default.
const DefaultServerURL = "https://app.bowrain.cloud"

// NormalizeServerURL canonicalizes a user-supplied Bowrain server URL. It
// lowercases the scheme and the host, drops the scheme's default port (443
// for https, 80 for http) and trims a trailing slash. It rewrites the hosted
// service's landing-page hosts (bowrain.cloud, www.bowrain.cloud) to the app
// host (app.bowrain.cloud): the top-level domain serves the marketing site,
// so an API call against it would not reach the platform. A path is kept as
// written. A string that is not a URL with a host is returned with only the
// trailing slash trimmed.
func NormalizeServerURL(raw string) string {
	if raw == "" {
		return ""
	}
	s := strings.TrimRight(raw, "/")
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return s
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	switch host {
	case "bowrain.cloud", "www.bowrain.cloud":
		u.Scheme = "https"
		u.Host = "app.bowrain.cloud"
		return strings.TrimRight(u.String(), "/")
	}
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host += ":" + port
	}
	u.Host = host
	return strings.TrimRight(u.String(), "/")
}

// The framework compares the server a sync state is bound to with the one a
// recipe connects to (core/project's relink), through this normalisation, so
// it and the claim check here agree on what one server is.
func init() {
	coreproj.ServerURLNormalizer = NormalizeServerURL
}
