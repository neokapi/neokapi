// Package provider defines the MTProvider interface for machine translation services.
package mtprovider

import (
	"context"
	"fmt"
	"sync"

	"github.com/neokapi/neokapi/core/model"
)

// ProviderID is a type-safe identifier for an MT provider.
type ProviderID string

// String returns the string representation.
func (id ProviderID) String() string { return string(id) }

// MTProvider defines the interface for machine translation service providers.
//
// The framework ships no classic MT engines: the translation core is LLM-first
// (see providers/ai), and the only built-in MTProvider is the offline demo
// provider (demo.go). Plugins host real MT engines by registering a
// ConfigFactory (RegisterConfigFactory) and, for credential-free engines, a
// ProviderFactory (RegisterProvider).
type MTProvider interface {
	// Name returns the provider identifier (e.g., Demo).
	Name() ProviderID

	// Translate translates text using the MT service.
	Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error)

	// Close releases provider resources.
	Close() error
}

// TranslateRequest contains parameters for a translation request.
type TranslateRequest struct {
	Source       string
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
}

// TranslateResponse contains the translation result.
type TranslateResponse struct {
	Translation string
}

// ---------------------------------------------------------------------------
// Config-driven provider registry (mirrors providers/ai's NewProvider)
// ---------------------------------------------------------------------------

// MTConfig is the generic, credential-bearing configuration used to construct
// any registered MT provider from a flow/CLI config map. Each provider factory
// consumes the fields it needs and ignores the rest. The CLI credential
// resolver populates APIKey from the OS keychain (see
// cli/credentials/resolve.go); other fields come from the recipe step config.
type MTConfig struct {
	// APIKey is the primary credential for keyed MT providers.
	APIKey string `json:"apiKey,omitempty"`
	// BaseURL overrides the provider API endpoint (primarily for tests).
	BaseURL string `json:"baseURL,omitempty"`
}

// ConfigFactory builds an MTProvider from a generic MTConfig. Plugin-hosted
// providers register one so that the unified `translate` tool can construct
// them from a flow/CLI config map.
type ConfigFactory func(cfg MTConfig) MTProvider

var (
	configFactoryMu sync.RWMutex
	configFactories = map[ProviderID]ConfigFactory{}
)

// RegisterConfigFactory registers a config-driven MT provider factory by id.
// Plugins may call this to add providers that the translate tool can construct
// from recipe config.
func RegisterConfigFactory(id ProviderID, factory ConfigFactory) {
	configFactoryMu.Lock()
	defer configFactoryMu.Unlock()
	configFactories[id] = factory
}

// NewProviderWithConfig constructs a credential-bearing MT provider by id from
// a generic MTConfig. Returns an error if the id has no registered config
// factory.
func NewProviderWithConfig(id ProviderID, cfg MTConfig) (MTProvider, error) {
	configFactoryMu.RLock()
	factory, ok := configFactories[id]
	configFactoryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown MT provider: %s", id)
	}
	return factory(cfg), nil
}

// HasConfigFactory reports whether a config-driven factory is registered for id.
func HasConfigFactory(id ProviderID) bool {
	configFactoryMu.RLock()
	defer configFactoryMu.RUnlock()
	_, ok := configFactories[id]
	return ok
}

func init() {
	// The demo provider is config-constructible (ignores all credentials) so
	// the config path can produce illustrative output without keys.
	RegisterConfigFactory(Demo, func(_ MTConfig) MTProvider { return NewDemoProvider() })
}
