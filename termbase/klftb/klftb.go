package klftb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/termbase"
)

// SchemaVersion is the klftb wire format version. Same MAJOR.MINOR
// forward-compatibility contract as core/klf. The format carries concepts and
// their relations; a document with no relations simply omits the array.
const SchemaVersion = "1.0"

// Kind is the magic string on the root of a klftb document.
const Kind = "kapi-termbase-format"

// File is the top-level shape of a klftb document: an envelope plus the full
// set of termbase concepts and the relations between them. Concepts and
// relations reuse the termbase types directly (they already carry JSON tags),
// so the wire form cannot drift from the model.
type File struct {
	SchemaVersion string                     `json:"schemaVersion"`
	Kind          string                     `json:"kind"`
	Created       string                     `json:"created,omitempty"`
	Generator     *GeneratorInfo             `json:"generator,omitempty"`
	Concepts      []termbase.Concept         `json:"concepts"`
	Relations     []termbase.ConceptRelation `json:"relations,omitempty"`
}

// GeneratorInfo identifies the tool that produced the file.
type GeneratorInfo struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

// FromConcepts builds a klftb File from a set of concepts (e.g. the output of
// TermBase.Concepts()).
func FromConcepts(concepts []termbase.Concept) *File {
	return &File{SchemaVersion: SchemaVersion, Kind: Kind, Concepts: concepts}
}

// Marshal encodes a File to deterministic UTF-8 JSON: concepts and relations
// sorted by id, terms sorted, timestamps normalized to UTC, HTML escaping off,
// 2-space indent, trailing newline.
func Marshal(f *File) ([]byte, error) {
	if f == nil {
		return nil, errors.New("klftb: marshal nil file")
	}
	if f.SchemaVersion == "" {
		f.SchemaVersion = SchemaVersion
	}
	if f.Kind == "" {
		f.Kind = Kind
	}
	f.Concepts = canonicalConcepts(f.Concepts)
	f.Relations = canonicalRelations(f.Relations)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return nil, fmt.Errorf("klftb: encode: %w", err)
	}
	return buf.Bytes(), nil
}

// canonicalConcepts returns a deterministically ordered, UTC-normalized copy of
// the concepts so Marshal is byte-stable for hashing.
func canonicalConcepts(in []termbase.Concept) []termbase.Concept {
	out := make([]termbase.Concept, len(in))
	copy(out, in)
	for i := range out {
		out[i].CreatedAt = out[i].CreatedAt.UTC()
		out[i].UpdatedAt = out[i].UpdatedAt.UTC()
		terms := make([]termbase.Term, len(out[i].Terms))
		copy(terms, out[i].Terms)
		for j := range terms {
			terms[j].Validity = canonicalValidity(terms[j].Validity)
		}
		sort.SliceStable(terms, func(a, b int) bool {
			if terms[a].Locale != terms[b].Locale {
				return terms[a].Locale < terms[b].Locale
			}
			return terms[a].Text < terms[b].Text
		})
		out[i].Terms = terms
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// canonicalRelations returns a deterministically ordered, UTC-normalized copy
// of the relations, mirroring canonicalConcepts. An empty set normalizes to
// nil so the relations key is omitted rather than emitted as [].
func canonicalRelations(in []termbase.ConceptRelation) []termbase.ConceptRelation {
	if len(in) == 0 {
		return nil
	}
	out := make([]termbase.ConceptRelation, len(in))
	copy(out, in)
	for i := range out {
		out[i].CreatedAt = out[i].CreatedAt.UTC()
		out[i].Validity = canonicalValidity(out[i].Validity)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// canonicalValidity returns a UTC-normalized copy of v (the caller's value is
// left untouched). A nil validity stays nil.
func canonicalValidity(v *graph.Validity) *graph.Validity {
	if v == nil {
		return nil
	}
	out := *v
	if out.ValidFrom != nil {
		from := out.ValidFrom.UTC()
		out.ValidFrom = &from
	}
	if out.ValidTo != nil {
		to := out.ValidTo.UTC()
		out.ValidTo = &to
	}
	return &out
}

// Unmarshal decodes a klftb payload into a File, rejecting an unknown kind or
// major schema version (unknown minors of a known major are accepted).
func Unmarshal(data []byte) (*File, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("klftb: decode: %w", err)
	}
	if f.Kind != Kind {
		return nil, fmt.Errorf("klftb: unexpected kind %q (want %q)", f.Kind, Kind)
	}
	major, ok := majorVersion(f.SchemaVersion)
	if !ok {
		return nil, fmt.Errorf("klftb: invalid schemaVersion %q", f.SchemaVersion)
	}
	wantMajor, _ := majorVersion(SchemaVersion)
	if major != wantMajor {
		return nil, fmt.Errorf("klftb: unsupported major schemaVersion %d (this build speaks %s)", major, SchemaVersion)
	}
	return &f, nil
}

// Decode streams a klftb payload from r.
func Decode(r io.Reader) (*File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("klftb: read: %w", err)
	}
	return Unmarshal(data)
}

func majorVersion(v string) (int, bool) {
	major := 0
	seen := false
	for _, r := range v {
		if r == '.' {
			return major, seen
		}
		if r < '0' || r > '9' {
			return 0, false
		}
		major = major*10 + int(r-'0')
		seen = true
	}
	return 0, false
}
