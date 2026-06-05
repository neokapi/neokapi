// Package provider defines the MTProvider interface for machine translation services.
package mtprovider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/neokapi/neokapi/core/model"
)

// sendAndRead executes an HTTP request and returns the response body, mapping a
// transport error, a read error, or a non-200 status into the same wrapped
// errors each MT provider previously repeated inline.
func sendAndRead(client *http.Client, req *http.Request) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api returned status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// ProviderID is a type-safe identifier for an MT provider.
type ProviderID string

// String returns the string representation.
func (id ProviderID) String() string { return string(id) }

// Known MT provider identifiers.
const (
	DeepL    ProviderID = "deepl"
	Google   ProviderID = "google"
	MSFT     ProviderID = "microsoft"
	ModernMT ProviderID = "modernmt"
	MyMemory ProviderID = "mymemory"
)

// MTProvider defines the interface for machine translation service providers.
type MTProvider interface {
	// Name returns the provider identifier (e.g., DeepL, Google).
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
// any registered MT provider from a flow/CLI config map. It is the union of the
// per-provider credential fields; each provider factory consumes the fields it
// needs and ignores the rest. The CLI credential resolver populates APIKey from
// the OS keychain (see cli/credentials/resolve.go); other fields come from the
// recipe step config.
type MTConfig struct {
	// APIKey is the primary credential for deepl, google, and modernmt.
	APIKey string `json:"apiKey,omitempty"`
	// SubscriptionKey is the Azure credential for the microsoft provider. When
	// empty, APIKey is used as a fallback so credential resolution (which injects
	// "apiKey") works uniformly across providers.
	SubscriptionKey string `json:"subscriptionKey,omitempty"`
	// Region is the Azure region for the microsoft provider.
	Region string `json:"region,omitempty"`
	// Email is the optional MyMemory account email for higher rate limits.
	Email string `json:"email,omitempty"`
	// ProjectID is the optional Google Cloud project id.
	ProjectID string `json:"projectId,omitempty"`
	// BaseURL overrides the provider API endpoint (primarily for tests).
	BaseURL string `json:"baseURL,omitempty"`
}

// ConfigFactory builds an MTProvider from a generic MTConfig. Real providers
// (deepl, google, microsoft, modernmt, mymemory) register one so that the
// <provider>-translate tools can be constructed from a flow/CLI config map.
type ConfigFactory func(cfg MTConfig) MTProvider

var (
	configFactoryMu sync.RWMutex
	configFactories = map[ProviderID]ConfigFactory{}
)

// RegisterConfigFactory registers a config-driven MT provider factory by id.
// Plugins may call this to add providers that the <provider>-translate tools
// can construct from recipe config.
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
	RegisterConfigFactory(DeepL, func(cfg MTConfig) MTProvider {
		return NewDeepLProvider(DeepLConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL})
	})
	RegisterConfigFactory(Google, func(cfg MTConfig) MTProvider {
		return NewGoogleProvider(GoogleConfig{APIKey: cfg.APIKey, ProjectID: cfg.ProjectID, BaseURL: cfg.BaseURL})
	})
	RegisterConfigFactory(MSFT, func(cfg MTConfig) MTProvider {
		key := cfg.SubscriptionKey
		if key == "" {
			key = cfg.APIKey
		}
		return NewMicrosoftProvider(MicrosoftConfig{SubscriptionKey: key, Region: cfg.Region, BaseURL: cfg.BaseURL})
	})
	RegisterConfigFactory(ModernMT, func(cfg MTConfig) MTProvider {
		return NewModernMTProvider(ModernMTConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL})
	})
	RegisterConfigFactory(MyMemory, func(cfg MTConfig) MTProvider {
		return NewMyMemoryProvider(MyMemoryConfig{Email: cfg.Email, BaseURL: cfg.BaseURL})
	})
	// The demo provider is also config-constructible (ignores all credentials)
	// so the config path can fall back to illustrative output without keys.
	RegisterConfigFactory(Demo, func(_ MTConfig) MTProvider { return NewDemoProvider() })
}
