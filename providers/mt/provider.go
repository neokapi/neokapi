// Package provider defines the MTProvider interface for machine translation services.
package mtprovider

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

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
