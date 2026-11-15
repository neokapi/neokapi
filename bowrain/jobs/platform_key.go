package jobs

import (
	"os"
	"strings"
)

// platformKeyPlaceholder is the value infrastructure writes into an unset
// secret. It is never a real key for any provider.
const platformKeyPlaceholder = "CHANGEME"

// PlatformAPIKey resolves the API key the platform provider calls with. The
// server's synchronous path and the worker both call it, so they always send
// the same key.
//
// The provider's own variable comes first (OPENAI_API_KEY for openai,
// GEMINI_API_KEY or GOOGLE_API_KEY for gemini, and so on), then the generic
// BOWRAIN_PLATFORM_API_KEY. An empty value and the "CHANGEME" placeholder count
// as unset, so switching ai.provider in ctrl never sends a placeholder upstream.
// Keyless providers (bedrock, ollama, demo) have no variable of their own and
// usually resolve to "".
func PlatformAPIKey(provider string) string {
	return platformAPIKey(provider, os.Getenv)
}

func platformAPIKey(provider string, getenv func(string) string) string {
	names := append(providerKeyEnv(provider), "BOWRAIN_PLATFORM_API_KEY")
	for _, n := range names {
		v := strings.TrimSpace(getenv(n))
		if v != "" && v != platformKeyPlaceholder {
			return v
		}
	}
	return ""
}

// providerKeyEnv names the provider-specific environment variables that may
// hold a provider's API key, in the order they are consulted.
func providerKeyEnv(provider string) []string {
	switch strings.ToLower(provider) {
	case "gemini":
		return []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"}
	case "openai":
		return []string{"OPENAI_API_KEY"}
	case "anthropic":
		return []string{"ANTHROPIC_API_KEY"}
	case "azureopenai":
		return []string{"AZURE_OPENAI_API_KEY"}
	}
	return nil
}
