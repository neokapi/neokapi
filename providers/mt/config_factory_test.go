package mtprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewProviderWithConfig_Demo verifies the only built-in config-constructible
// provider — the offline demo — builds from a generic config map.
func TestNewProviderWithConfig_Demo(t *testing.T) {
	assert.True(t, HasConfigFactory(Demo))
	p, err := NewProviderWithConfig(Demo, MTConfig{APIKey: "ignored"})
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, Demo, p.Name())
	require.NoError(t, p.Close())
}

func TestNewProviderWithConfig_Unknown(t *testing.T) {
	assert.False(t, HasConfigFactory("nope"))
	_, err := NewProviderWithConfig("nope", MTConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown MT provider")
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
