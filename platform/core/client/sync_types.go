package client

import "encoding/json"

// SyncBlock carries the full block model through the sync boundary (AD-038 Phase 7).
// Unlike BlockContent which only carries plain text, SyncBlock preserves structured
// segments with inline spans, annotations, skeleton, display hints, and metadata.
type SyncBlock struct {
	ID                 string                       `json:"id"`
	ItemName           string                       `json:"item_name"`
	Name               string                       `json:"name"`
	Type               string                       `json:"type,omitempty"`
	MimeType           string                       `json:"mime_type,omitempty"`
	Translatable       bool                         `json:"translatable"`
	Source             []SyncSegment                `json:"source"`
	SourceText         string                       `json:"source_text"`
	Targets            map[string][]SyncSegment     `json:"targets,omitempty"`
	Properties         map[string]string            `json:"properties,omitempty"`
	Annotations        json.RawMessage              `json:"annotations,omitempty"`
	Skeleton           json.RawMessage              `json:"skeleton,omitempty"`
	PreserveWhitespace bool                         `json:"preserve_whitespace,omitempty"`
	DisplayHint        json.RawMessage              `json:"display_hint,omitempty"`
	ContentRef         json.RawMessage              `json:"content_ref,omitempty"`
	ConnectorData      map[string]string            `json:"connector_data,omitempty"`
	ContentHash        string                       `json:"content_hash,omitempty"`
}

// SyncSegment represents a segment within a SyncBlock.
type SyncSegment struct {
	ID         string            `json:"id"`
	Text       string            `json:"text"`
	CodedText  string            `json:"coded_text,omitempty"`
	Spans      []SyncSpan        `json:"spans,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// SyncSpan represents an inline span within a segment.
type SyncSpan struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	SubType  string `json:"sub_type,omitempty"`
	SpanType string `json:"span_type"`
	Data     string `json:"data,omitempty"`
}

// SyncTerm carries terminology entries through the sync boundary.
type SyncTerm struct {
	ConceptID    string              `json:"concept_id"`
	SourceTerm   string              `json:"source_term"`
	SourceLocale string              `json:"source_locale"`
	Translations []SyncTermTranslation `json:"translations,omitempty"`
	Definition   string              `json:"definition,omitempty"`
	Domain       string              `json:"domain,omitempty"`
	Properties   map[string]string   `json:"properties,omitempty"`
	Status       string              `json:"status,omitempty"`
}

// SyncTermTranslation is a single term translation for a locale.
type SyncTermTranslation struct {
	Text   string `json:"text"`
	Locale string `json:"locale"`
	Status string `json:"status,omitempty"`
}

// SyncMedia carries media asset metadata through the sync boundary.
type SyncMedia struct {
	ID            string            `json:"id"`
	ItemName      string            `json:"item_name"`
	MimeType      string            `json:"mime_type"`
	Filename      string            `json:"filename"`
	AltText       string            `json:"alt_text,omitempty"`
	SizeBytes     int64             `json:"size_bytes"`
	BlobKey       string            `json:"blob_key,omitempty"`
	Locale        string            `json:"locale,omitempty"`
	SourceMediaID string            `json:"source_media_id,omitempty"`
	Properties    map[string]string `json:"properties,omitempty"`
}

// RichPullResponse is the response from a rich pull (AD-038 Phase 7).
// It carries full content (blocks, terms, media) instead of change log entries.
type RichPullResponse struct {
	Cursor  int64       `json:"cursor"`
	HasMore bool        `json:"has_more"`
	Blocks  []SyncBlock `json:"blocks"`
	Terms   []SyncTerm  `json:"terms,omitempty"`
	Media   []SyncMedia `json:"media,omitempty"`
}
