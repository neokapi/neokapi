package mcp

import (
	"context"
	"strings"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
)

// bgCtx returns a background context. Used by resource handlers that don't
// receive a context from the MCP SDK's resource handler interface.
func bgCtx() context.Context {
	return context.Background()
}

// extractParam extracts the value after a prefix from a URI.
// For "brand://profiles/abc123", extractParam(uri, "brand://profiles/") returns "abc123".
func extractParam(uri, prefix string) string {
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(uri, prefix)
	// Stop at any trailing slash or query string.
	if idx := strings.IndexAny(rest, "/?"); idx >= 0 {
		rest = rest[:idx]
	}
	return rest
}

// extractParamBefore extracts the value between a prefix and a suffix in a URI.
// For "brand://profiles/abc123/vocabulary", extractParamBefore(uri, "brand://profiles/", "/vocabulary")
// returns "abc123".
func extractParamBefore(uri, prefix, suffix string) string {
	if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, suffix) {
		return ""
	}
	return uri[len(prefix) : len(uri)-len(suffix)]
}

// resolveProfile is a convenience wrapper around corebrand.ResolveProfile
// that handles empty locale/channel gracefully.
func resolveProfile(profile *corebrand.VoiceProfile, locale, channel string) *corebrand.VoiceProfile {
	return corebrand.ResolveProfile(profile, model.LocaleID(locale), channel)
}
