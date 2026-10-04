package kbf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/schemaversion"
)

// Marshal encodes a File to deterministic UTF-8 JSON: 2-space indent,
// no HTML escaping, trailing newline. Deterministic output is what
// makes .kbf.json git-diffable and hashable.
//
// The blocks it writes are in the current schema, so a file whose version is
// not a minor of the current major (empty, schema 1, or not a version) is
// stamped [SchemaVersion].
func Marshal(f *File) ([]byte, error) {
	if f == nil {
		return nil, errors.New("kbf: marshal nil file")
	}
	if !isCurrentSchema(f.SchemaVersion) {
		f.SchemaVersion = SchemaVersion
	}
	if f.Kind == "" {
		f.Kind = Kind
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return nil, fmt.Errorf("kbf: encode: %w", err)
	}
	return buf.Bytes(), nil
}

// isCurrentSchema reports whether version is a minor of the current major.
func isCurrentSchema(version string) bool {
	major, ok := schemaversion.Major(version)
	current, _ := schemaversion.Major(SchemaVersion)
	return ok && major == current
}

// MarshalBlock encodes a single Block as JSON. Used by tests, debug
// tools, and the .overlays.jsonl (JSON-Lines kbf) variant mentioned in RFC
// 0001 §Future possibilities.
func MarshalBlock(b *Block) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(b); err != nil {
		return nil, fmt.Errorf("kbf: encode block: %w", err)
	}
	return buf.Bytes(), nil
}

// Encode streams a File to an io.Writer using the same deterministic
// formatting as Marshal.
func Encode(w io.Writer, f *File) error {
	data, err := Marshal(f)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("kbf: write: %w", err)
	}
	return nil
}
