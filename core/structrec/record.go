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
	// ContentHash is model.ComputeContentHash(Text): a SHA-256 over the block's
	// NORMALIZED (whitespace-trimmed) source text.
	ContentHash string `json:"content_hash,omitempty" yaml:"content_hash,omitempty"`
	Role        string `json:"role,omitempty" yaml:"role,omitempty"`
	Level       int    `json:"level,omitempty" yaml:"level,omitempty"`
	Text        string `json:"text" yaml:"text"`
}

// New builds a Record for one block, computing the content hash from text. The
// caller supplies text (source or a resolved target translation) and the block's
// structural role/level.
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

// FromBlock builds a Record from a block using the given text (already resolved
// to source or target by the caller), reading role/level from the block's
// structure annotation when present.
func FromBlock(number int, b *model.Block, text string) Record {
	rec := New(number, b.ID, text, "", 0)
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
