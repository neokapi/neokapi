// Package structrec defines the universal structural record shared by
// `kapi inspect` and the structural JSON/YAML conversion writers: one record per
// content block, carrying the block's text, a stable content-hash anchor, and
// its structural role/level. Any format — a Word document, a JSON catalog,
// Markdown, HTML, DocLang — yields the same shape, so a document with no catalog
// keys serializes faithfully instead of collapsing onto an empty key.
package structrec

import (
	"bytes"
	"encoding/json"

	"github.com/neokapi/neokapi/core/model"
	yamlv3 "gopkg.in/yaml.v3"
)

// Record is one content block: its text plus a stable content-hash anchor and
// the block's structural role and nesting level. The read end of read →
// retrieve → edit → write-back — an AI agent or RAG pipeline reads these
// records, retrieves against the anchors, and writes edits back to the same
// blocks (by ID).
type Record struct {
	// File is the source filename. Set by `kapi inspect` (which may span several
	// files); omitted by the single-stream conversion writers.
	File   string `json:"file,omitempty" yaml:"file,omitempty"`
	Number int    `json:"number" yaml:"number"`
	ID     string `json:"id,omitempty" yaml:"id,omitempty"`
	// ContentHash is the canonical block identity: a SHA-256 over the block's
	// NORMALIZED (whitespace-trimmed) plain source text (model.ComputeContentHash
	// of SourceText). It is NOT a hash of Text — Text carries inline-code
	// placeholders, the hash does not — so the two anchor different things: Text
	// for editing, ContentHash for identity and drift detection.
	ContentHash string `json:"content_hash,omitempty" yaml:"content_hash,omitempty"`
	Role        string `json:"role,omitempty" yaml:"role,omitempty"`
	Level       int    `json:"level,omitempty" yaml:"level,omitempty"`
	Text        string `json:"text" yaml:"text"`
}

// New builds a Record for one block from an explicit text rendering, computing
// the content hash over that same text. Prefer FromBlock for content blocks —
// it renders inline codes as placeholders while keeping the hash canonical.
func New(number int, id, text, role string, level int) Record {
	return Record{
		Number:      number,
		ID:          id,
		ContentHash: model.ComputeContentHash(text),
		Role:        role,
		Level:       level,
		Text:        text,
	}
}

// FromBlock builds a Record from a block, rendering the given runs (the block's
// source, or a resolved target) as the record Text.
//
// Text is the placeholder rendering (model.RunsPlaceholderText): inline codes
// appear as <x id="…"/> tokens so the read leg is symmetric with the write-back
// leg — an agent that reads a record and edits its Text can round-trip the edit
// without dropping a link, bold span, or placeholder.
//
// ContentHash is deliberately NOT computed over that placeholder text. It stays
// the canonical block identity — model.ComputeContentHash over the block's plain
// source text — the same hash the sync engine, content stores, and desktop
// status view use (a frozen on-the-wire contract). Text (for editing) and
// ContentHash (for identity/drift) are therefore on different bases by design.
func FromBlock(number int, b *model.Block, runs []model.Run) Record {
	rec := Record{
		Number:      number,
		ID:          b.ID,
		ContentHash: model.ComputeContentHash(b.SourceText()),
		Text:        model.RunsPlaceholderText(runs),
	}
	if s, ok := b.Structure(); ok {
		rec.Role, rec.Level = s.Role, s.Level
	}
	return rec
}

// MarshalJSONArray renders records as an indented JSON array with a trailing
// newline. An empty slice renders as "[]\n" — never "{}".
func MarshalJSONArray(recs []Record) ([]byte, error) {
	if recs == nil {
		recs = []Record{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(recs); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalYAML renders records as a YAML sequence with a trailing newline. An
// empty slice renders as "[]\n".
func MarshalYAML(recs []Record) ([]byte, error) {
	if len(recs) == 0 {
		return []byte("[]\n"), nil
	}
	var buf bytes.Buffer
	enc := yamlv3.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(recs); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}
