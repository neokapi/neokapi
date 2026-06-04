package client

import "encoding/json"

// SyncBlock carries the full block model through the sync boundary (Bowrain AD-009 Phase 7).
// Unlike BlockContent which only carries plain text, SyncBlock preserves structured
// segments with inline spans, annotations, skeleton, display hints, and metadata.
type SyncBlock struct {
	ID                 string                   `json:"id"`
	ItemName           string                   `json:"item_name"`
	Name               string                   `json:"name"`
	Type               string                   `json:"type,omitempty"`
	MimeType           string                   `json:"mime_type,omitempty"`
	Translatable       bool                     `json:"translatable"`
	Source             []SyncSegment            `json:"source"`
	SourceText         string                   `json:"source_text"`
	Targets            map[string][]SyncSegment `json:"targets,omitempty"`
	Properties         map[string]string        `json:"properties,omitempty"`
	Annotations        json.RawMessage          `json:"annotations,omitempty"`
	Skeleton           json.RawMessage          `json:"skeleton,omitempty"`
	PreserveWhitespace bool                     `json:"preserve_whitespace,omitempty"`
	DisplayHint        json.RawMessage          `json:"display_hint,omitempty"`
	ContentRef         json.RawMessage          `json:"content_ref,omitempty"`
	ConnectorData      map[string]string        `json:"connector_data,omitempty"`
	ContentHash        string                   `json:"content_hash,omitempty"`
}

// SyncSegment represents a segment within a SyncBlock.
type SyncSegment struct {
	ID         string            `json:"id"`
	Runs       []SyncRun         `json:"runs,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// SyncRunConstraints mirrors model.RunConstraints on the sync wire.
type SyncRunConstraints struct {
	Deletable   bool `json:"deletable,omitempty"`
	Cloneable   bool `json:"cloneable,omitempty"`
	Reorderable bool `json:"reorderable,omitempty"`
}

// SyncTextRun carries a plain-text span on the sync wire.
type SyncTextRun struct {
	Text string `json:"text"`
}

// SyncPlaceholderRun is a non-paired inline code (model.PlaceholderRun) on the sync wire.
type SyncPlaceholderRun struct {
	ID          string              `json:"id"`
	Type        string              `json:"type"`
	SubType     string              `json:"subType,omitempty"`
	Data        string              `json:"data"`
	Equiv       string              `json:"equiv"`
	Disp        string              `json:"disp,omitempty"`
	Constraints *SyncRunConstraints `json:"constraints,omitempty"`
}

// SyncPcOpenRun is the opening half of a paired inline code (model.PcOpenRun) on the sync wire.
type SyncPcOpenRun struct {
	ID          string              `json:"id"`
	Type        string              `json:"type"`
	SubType     string              `json:"subType,omitempty"`
	Data        string              `json:"data"`
	Equiv       string              `json:"equiv"`
	Disp        string              `json:"disp,omitempty"`
	Constraints *SyncRunConstraints `json:"constraints,omitempty"`
}

// SyncPcCloseRun is the closing half of a paired inline code (model.PcCloseRun) on the sync wire.
type SyncPcCloseRun struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	SubType string `json:"subType,omitempty"`
	Data    string `json:"data"`
	Equiv   string `json:"equiv,omitempty"`
}

// SyncSubRun is a sub-flow run (model.SubRun) on the sync wire, referencing a nested content unit.
type SyncSubRun struct {
	ID    string `json:"id"`
	Ref   string `json:"ref"`
	Equiv string `json:"equiv"`
}

// SyncPluralRun carries a plural-form run (model.PluralRun) on the sync wire.
// Forms maps ICU plural-category strings (e.g. "one", "other") to run sequences.
type SyncPluralRun struct {
	Pivot string               `json:"pivot"`
	Forms map[string][]SyncRun `json:"forms"`
}

// SyncSelectRun carries a select/switch run (model.SelectRun) on the sync wire.
// Cases maps case-key strings to run sequences.
type SyncSelectRun struct {
	Pivot string               `json:"pivot"`
	Cases map[string][]SyncRun `json:"cases"`
}

// SyncRun is the discriminated-union inline-content primitive on
// the sync wire. Exactly one of the pointer fields is non-nil per
// record.
type SyncRun struct {
	Text    *SyncTextRun        `json:"text,omitempty"`
	Ph      *SyncPlaceholderRun `json:"ph,omitempty"`
	PcOpen  *SyncPcOpenRun      `json:"pcOpen,omitempty"`
	PcClose *SyncPcCloseRun     `json:"pcClose,omitempty"`
	Sub     *SyncSubRun         `json:"sub,omitempty"`
	Plural  *SyncPluralRun      `json:"plural,omitempty"`
	Select  *SyncSelectRun      `json:"select,omitempty"`
}

// SyncTerm carries terminology entries through the sync boundary.
type SyncTerm struct {
	ConceptID    string                `json:"concept_id"`
	SourceTerm   string                `json:"source_term"`
	SourceLocale string                `json:"source_locale"`
	Translations []SyncTermTranslation `json:"translations,omitempty"`
	Definition   string                `json:"definition,omitempty"`
	Domain       string                `json:"domain,omitempty"`
	Properties   map[string]string     `json:"properties,omitempty"`
	Status       string                `json:"status,omitempty"`
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

// RichPullResponse is the response from a rich pull (Bowrain AD-009 Phase 7).
// It carries full content (blocks, terms, media) instead of change log entries.
type RichPullResponse struct {
	Cursor  int64       `json:"cursor"`
	HasMore bool        `json:"has_more"`
	Blocks  []SyncBlock `json:"blocks"`
	Terms   []SyncTerm  `json:"terms,omitempty"`
	Media   []SyncMedia `json:"media,omitempty"`
}
