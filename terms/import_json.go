package terms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// JSONStore is the JSON-serializable representation of a terms store.
type JSONStore struct {
	Name     string    `json:"name"`
	Version  string    `json:"version"`
	Concepts []Concept `json:"concepts"`
	// Origin names the store the file was exported from, and Revisions the
	// revision each concept had there, by id, when that store keeps one. A
	// file imported back into the same store updates each concept at that
	// revision; anywhere else both are ignored.
	Origin    string            `json:"origin,omitempty"`
	Revisions map[string]string `json:"revisions,omitempty"`
}

// ImportJSON reads a JSON terms file and imports all concepts. A concept
// that names no id takes ImportID of its first term.
func ImportJSON(ctx context.Context, tb Terminology, reader io.Reader) (int, error) {
	rows, err := ParseJSON(reader)
	if err != nil {
		return 0, err
	}
	return writeRows(ctx, tb, rows)
}

// ExportJSON writes all concepts as a JSON terms.
func ExportJSON(ctx context.Context, tb Terminology, writer io.Writer, name string) error {
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return fmt.Errorf("list concepts: %w", err)
	}
	doc := JSONStore{
		Name:     name,
		Version:  "1.0",
		Concepts: concepts,
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return fmt.Errorf("encode JSON terms: %w", err)
	}
	return nil
}
