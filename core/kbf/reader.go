package kbf

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/neokapi/neokapi/core/schemaversion"
)

// Unmarshal decodes a .kbf.json payload into a File, returning an
// error if the payload's kind or major schema version is unknown.
// Unknown minor versions within the same major are accepted per the
// forward-compatibility contract in RFC 0001 §Versioning.
func Unmarshal(data []byte) (*File, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("kbf: decode: %w", err)
	}
	if err := checkEnvelope(&f); err != nil {
		return nil, err
	}
	return &f, nil
}

// Decode streams a .kbf.json payload from an io.Reader.
func Decode(r io.Reader) (*File, error) {
	dec := json.NewDecoder(r)
	var f File
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("kbf: decode: %w", err)
	}
	if err := checkEnvelope(&f); err != nil {
		return nil, err
	}
	return &f, nil
}

func checkEnvelope(f *File) error {
	if f.Kind == "" {
		return fmt.Errorf("kbf: missing kind (want %q)", Kind)
	}
	if !slices.Contains(ReadableKinds, f.Kind) {
		return fmt.Errorf("kbf: unexpected kind %q (want %q)", f.Kind, Kind)
	}
	major, ok := schemaversion.Major(f.SchemaVersion)
	if !ok {
		return fmt.Errorf("kbf: invalid schemaVersion %q", f.SchemaVersion)
	}
	wantMajor, _ := schemaversion.Major(SchemaVersion)
	if major != wantMajor {
		return fmt.Errorf("kbf: unsupported major schemaVersion %d (this build speaks %s)", major, SchemaVersion)
	}
	return nil
}
