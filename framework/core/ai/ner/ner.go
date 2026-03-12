// Package ner defines the interface for named entity recognition providers.
//
// NER providers detect named entities (people, organizations, dates, currencies, etc.)
// in text using ML models or cloud services. They complement LLM-based extraction
// by providing fast, cheap, deterministic entity recognition for obvious entity types.
//
// See AD-022 for the hybrid LLM + NER extraction architecture.
package ner

import (
	"context"

	"github.com/gokapi/gokapi/core/model"
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
