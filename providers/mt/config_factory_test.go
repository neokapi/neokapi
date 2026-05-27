package mtprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProviderWithConfig_AllRealProviders(t *testing.T) {
	cases := []struct {
		id   ProviderID
		want ProviderID
	}{
		{DeepL, DeepL},
		{Google, Google},
		{MSFT, MSFT},
		{ModernMT, ModernMT},
		{MyMemory, MyMemory},
		{Demo, Demo},
	}
	for _, tc := range cases {
		t.Run(string(tc.id), func(t *testing.T) {
			assert.True(t, HasConfigFactory(tc.id))
			p, err := NewProviderWithConfig(tc.id, MTConfig{APIKey: "k", SubscriptionKey: "s", Region: "westeurope", Email: "x@y.z", ProjectID: "proj"})
			require.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, tc.want, p.Name())
			require.NoError(t, p.Close())
		})
	}
}

func TestNewProviderWithConfig_Unknown(t *testing.T) {
	assert.False(t, HasConfigFactory("nope"))
	_, err := NewProviderWithConfig("nope", MTConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown MT provider")
}

// TestMicrosoftFallsBackToAPIKey verifies the microsoft factory accepts a key
// injected as "apiKey" (the uniform credential field) when SubscriptionKey is
// empty — important because the CLI credential resolver only writes "apiKey".
func TestMicrosoftFallsBackToAPIKey(t *testing.T) {
	p, err := NewProviderWithConfig(MSFT, MTConfig{APIKey: "azure-key"})
	require.NoError(t, err)
	ms, ok := p.(*MicrosoftProvider)
	require.True(t, ok)
	assert.Equal(t, "azure-key", ms.cfg.SubscriptionKey)
}

// TestRegisterConfigFactory_Custom verifies plugins can register a provider.
func TestRegisterConfigFactory_Custom(t *testing.T) {
	const custom ProviderID = "custom-test-mt"
	RegisterConfigFactory(custom, func(_ MTConfig) MTProvider { return NewDemoProvider() })
	defer func() {
		configFactoryMu.Lock()
		delete(configFactories, custom)
		configFactoryMu.Unlock()
	}()

	p, err := NewProviderWithConfig(custom, MTConfig{})
	require.NoError(t, err)
	assert.Equal(t, Demo, p.Name())
}
