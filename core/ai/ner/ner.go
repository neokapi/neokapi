// Package ner defines the interface for named entity recognition providers.
//
// NER providers detect named entities (people, organizations, dates, currencies, etc.)
// in text using ML models or cloud services. They complement LLM-based extraction
// by providing fast, cheap, deterministic entity recognition for obvious entity types.
//
// See Bowrain AD-015 for the hybrid LLM + NER extraction architecture.
package ner

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// Provider detects named entities in text using ML models or cloud services.
type Provider interface {
	// Name returns the provider identifier (e.g., "azure", "spacy", "comprehend").
	Name() string

	// DetectEntities detects named entities in a single text.
	DetectEntities(ctx context.Context, req Request) (*Response, error)

	// DetectEntitiesBatch detects entities in multiple texts.
	// Implementations should batch API calls according to provider limits.
	DetectEntitiesBatch(ctx context.Context, reqs []Request) ([]Response, error)

	// SupportedLocales returns the locales this provider can process.
	// An empty slice means all locales are supported.
	SupportedLocales() []model.LocaleID

	// Close releases provider resources.
	Close() error
}

// Request is the input for entity detection.
type Request struct {
	Text   string         // the text to analyze
	Locale model.LocaleID // language hint for the NER model
}

// Response contains the detected entities for a single text.
type Response struct {
	Entities []DetectedEntity
}

// DetectedEntity is a single entity detected by the NER provider.
type DetectedEntity struct {
	Text       string           // the entity text as found in the input
	Type       model.EntityType // classified entity type
	Confidence float64          // detection confidence [0,1]
	Offset     int              // character offset from start of text
	Length     int              // character length of the entity
}

// Config holds common NER provider configuration.
type Config struct {
	Endpoint string            // API endpoint URL
	APIKey   string            // API key or credential
	Options  map[string]string // Provider-specific options
}

// localProvider is the process-wide ON-DEVICE NER provider, when one is
// available: a local ML model that detects entities without any network call.
// The browser build registers a JS-bridged GLiNER model here; a native plugin
// can register an in-process ONNX model the same way. nil means no local
// model is available and `ai-entity-extract` with `engine: ner` fails with an
// actionable error instead of silently extracting nothing.
var localProvider Provider

// SetLocalProvider registers the process-wide local NER provider. Call once
// during initialization, before tools run.
func SetLocalProvider(p Provider) { localProvider = p }

// LocalProvider returns the registered local NER provider, or nil.
func LocalProvider() Provider { return localProvider }
