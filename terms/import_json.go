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
	// Revisions is the revision each concept had when the file was
	// exported, by id, from a store that keeps revisions. Importing the file
	// back applies each concept as an edit at that revision, and a concept
	// that changed since is reported.
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
