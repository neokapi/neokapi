package venue

import (
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// The types in this file are the only part of the store's vocabulary that the
// venue client and the sync converter reach for. Everything else the package
// declares — the dashboards, the translation statistics, the ContentStore
// interface and its role sub-interfaces — is the server's persistence contract
// and is read by the server alone.
//
// The distinction is worth a file: these four are the shape of content and
// decisions in transit, and the surfaces that carry content between a project
// and a venue need them without needing the store that persists them.

// StoredBlock wraps a model.Block with store metadata.
type StoredBlock struct {
	*model.Block
	ProjectID   string
	ItemName    string
	SourceID    string // format-reader-assigned ID (e.g., "tu1"); empty for blocks stored without an item
	ContentHash string
	ContextHash string
	StoredAt    time.Time
	UpdatedAt   time.Time
}

// Asset represents a binary asset (image, audio, video) stored in BlobStore
// with metadata tracked in the ContentStore.
type Asset struct {
	ID               string            `json:"id"`
	ProjectID        string            `json:"project_id"`
	ItemName         string            `json:"item_name"` // source file this asset belongs to
	SourceID         string            `json:"source_id"` // format-reader-assigned ID within the item
	BlobKey          string            `json:"blob_key"`  // content-addressed key in BlobStore
	MimeType         string            `json:"mime_type"`
	Filename         string            `json:"filename"` // original filename
	SizeBytes        int64             `json:"size_bytes"`
	AltText          string            `json:"alt_text"`          // extractable localized text
	Properties       map[string]string `json:"properties"`        // dimensions, duration, codec, etc.
	ProcessingStatus string            `json:"processing_status"` // none, pending, processing, processed, failed
	ProcessingHint   string            `json:"processing_hint"`   // ocr, chart-text, subtitle-extract, asr
	Stream           string            `json:"stream,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// AssetVariant represents a locale-specific variant of an asset.
type AssetVariant struct {
	AssetID    string            `json:"asset_id"`
	Locale     string            `json:"locale"`   // BCP-47 tag
	BlobKey    string            `json:"blob_key"` // locale-specific binary in BlobStore
	Status     string            `json:"status"`   // pending, draft, approved
	MimeType   string            `json:"mime_type"`
	SizeBytes  int64             `json:"size_bytes"`
	Properties map[string]string `json:"properties"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// UnitDecision is the server's record of one workflow decision for one unit in
// one locale variant — the wire form of core/state.UnitState plus the item that
// scopes the unit's durable identity (the same structural name recurs across
// items, so an unscoped unit key cannot be joined safely). It records the
// decision as a FACT — who, when, which rung, and the hashes of the pairing it
// blesses: the translation, and the source that translation was approved for.
// Freshness is derived by whoever reads it, never stored.
type UnitDecision struct {
	ProjectID string `json:"project_id,omitempty"`
	Stream    string `json:"stream,omitempty"`
	// ItemName is the item whose durable identity namespace Unit lives in.
	// Empty when the decision arrived for content this store has never held.
	ItemName string `json:"item"`
	// Unit is the durable unit identity (convergence.BlockKey — the source_id
	// a stored row carries).
	Unit string `json:"unit"`
	// Variant is the locale (and optional tone/channel) in VariantKey text form.
	Variant string `json:"variant"`
	// Status is the target-ladder rung the decision lands the unit on.
	Status string `json:"status,omitempty"`
	// TargetHash is the content hash of the translation the decision blesses
	// (state.TargetHash of the trimmed target text).
	TargetHash string `json:"targetHash,omitempty"`
	// ContentHash is the BASIS: the content hash of the SOURCE the decision
	// blessed that translation for (state.SourceHash). Empty on a record written
	// before the basis was tracked — unknown, which readers must not confuse
	// with a source that has moved.
	ContentHash string `json:"contentHash,omitempty"`
	ReviewState string `json:"reviewState,omitempty"` // approved | rejected | signed-off
	DecidedBy   string `json:"by,omitempty"`          // "" human · "ai/<model>" · "agent/<client>" · server identity
	DecidedAt   string `json:"at,omitempty"`          // RFC 3339
	Note        string `json:"note,omitempty"`
	Parked      bool   `json:"parked,omitempty"`
	Assignee    string `json:"assignee,omitempty"`
	// GoverningFingerprint is the governing context the record's answer stands
	// under (state.UnitState.GoverningFingerprint): the voice guidance and term
	// rules in force when the decision was made, or the producer's stamp on a
	// basis. Empty on an ungoverned record and on one written before the field
	// existed.
	GoverningFingerprint string `json:"governingFingerprint,omitempty"`
	// Updated orders conflicting records for last-writer-wins reconciliation.
	Updated string `json:"updated,omitempty"`
}
