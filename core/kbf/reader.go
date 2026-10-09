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
// Unknown minor versions within a known major are accepted per the
// forward-compatibility contract in RFC 0001 §Versioning.
//
// A file in schema 1 reads as the editions it describes (Block.UnmarshalJSON)
// and comes back stamped [SchemaVersion], the shape it now holds, so writing it
// again writes the current schema.
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

// Decode streams a .kbf.json payload from an io.Reader. It reads a file in
// schema 1 the way [Unmarshal] does.
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

// checkEnvelope refuses a kind or a major version this build does not read,
// and restamps a schema 1 file, whose blocks decoded into editions, with the
// version of the shape it now holds.
func checkEnvelope(f *File) error {
	if f.Kind == "" {
		return fmt.Errorf("kbf: missing kind (want %q)", Kind)
	}
	if !slices.Contains(ReadableKinds, f.Kind) {
		return fmt.Errorf("kbf: unexpected kind %q (want %q)", f.Kind, Kind)
	}
	major, err := schemaversion.Check("kbf", f.SchemaVersion, SchemaVersionV1, SchemaVersion)
	if err != nil {
		return err
	}
	if v1, _ := schemaversion.Major(SchemaVersionV1); major == v1 {
		f.SchemaVersion = SchemaVersion
	}
	return nil
}
