// Package provider defines the MTProvider interface for machine translation services.
package mtprovider

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// MTProvider defines the interface for machine translation service providers.
type MTProvider interface {
	// Name returns the provider identifier (e.g., "deepl", "google").
	Name() string

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
