package klf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Marshal encodes a File to deterministic UTF-8 JSON: 2-space indent,
// no HTML escaping, trailing newline. Deterministic output is what
// makes .klf git-diffable and what lets the .klz manifest hash be
// stable across writes. The .klz writer builds on this contract.
func Marshal(f *File) ([]byte, error) {
	if f == nil {
		return nil, errors.New("klf: marshal nil file")
	}
	if f.SchemaVersion == "" {
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
		return nil, fmt.Errorf("klf: encode: %w", err)
	}
	return buf.Bytes(), nil
}

// MarshalBlock encodes a single Block as JSON. Used by tests, debug
// tools, and the .klfl (JSON-Lines klf) variant mentioned in RFC
// 0001 §Future possibilities.
func MarshalBlock(b *Block) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(b); err != nil {
		return nil, fmt.Errorf("klf: encode block: %w", err)
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
		return fmt.Errorf("klf: write: %w", err)
	}
	return nil
}
