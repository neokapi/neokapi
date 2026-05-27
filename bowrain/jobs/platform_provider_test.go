package jobs

import (
	"testing"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlatformProviderConfig_build(t *testing.T) {
	t.Run("generic provider (demo) needs no key", func(t *testing.T) {
		cfg := PlatformProviderConfig{Provider: "demo"}
		prov, ptype, err := cfg.build("")
		require.NoError(t, err)
		require.NotNil(t, prov)
		assert.Equal(t, "demo", ptype)
		assert.Equal(t, aiprovider.Demo, prov.Name())
		// The rate limiter lookup must recognise the returned provider type.
		assert.Contains(t, providerRateLimits, ptype)
	})

	t.Run("generic provider (gemini) with key", func(t *testing.T) {
		cfg := PlatformProviderConfig{Provider: "gemini", APIKey: "test-key", Model: "gemini-2.5-flash"}
		prov, ptype, err := cfg.build("")
		require.NoError(t, err)
		require.NotNil(t, prov)
		assert.Equal(t, "gemini", ptype)
		assert.Equal(t, aiprovider.Gemini, prov.Name())
	})

	t.Run("generic provider wins over azure endpoint", func(t *testing.T) {
		cfg := PlatformProviderConfig{Provider: "demo", Endpoint: "https://example.openai.azure.com"}
		_, ptype, err := cfg.build("")
		require.NoError(t, err)
		assert.Equal(t, "demo", ptype, "Provider must take precedence over the Azure endpoint path")
	})

	t.Run("unknown generic provider errors", func(t *testing.T) {
		cfg := PlatformProviderConfig{Provider: "not-a-provider"}
		_, _, err := cfg.build("")
		assert.Error(t, err)
	})

	t.Run("unconfigured errors", func(t *testing.T) {
		var cfg PlatformProviderConfig
		_, _, err := cfg.build("gpt-4o")
		assert.Error(t, err)
	})

	t.Run("azure endpoint path reports azureopenai type", func(t *testing.T) {
		// NewPlatformProvider builds an Azure managed-identity credential lazily;
		// construction succeeds without contacting Azure (token is fetched per request).
		cfg := PlatformProviderConfig{Endpoint: "https://example.openai.azure.com"}
		prov, ptype, err := cfg.build("gpt-4o")
		require.NoError(t, err)
		require.NotNil(t, prov)
		assert.Equal(t, "azureopenai", ptype)
	})
}
