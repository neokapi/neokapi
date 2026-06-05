package termbase

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// JSONTermBase is the JSON-serializable representation of a termbase.
type JSONTermBase struct {
	Name     string    `json:"name"`
	Version  string    `json:"version"`
	Concepts []Concept `json:"concepts"`
}

// ImportJSON reads a JSON termbase file and imports all concepts.
func ImportJSON(ctx context.Context, tb TermBase, reader io.Reader) (int, error) {
	var doc JSONTermBase
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&doc); err != nil {
		return 0, fmt.Errorf("parse JSON termbase: %w", err)
	}

	imported := 0
	for _, concept := range doc.Concepts {
		if concept.ID == "" {
			concept.ID = fmt.Sprintf("json-%d", imported+1)
		}
		if err := tb.AddConcept(ctx, concept); err != nil {
			return imported, fmt.Errorf("add concept %s: %w", concept.ID, err)
		}
		imported++
	}

	return imported, nil
}

// ExportJSON writes all concepts as a JSON termbase.
func ExportJSON(ctx context.Context, tb TermBase, writer io.Writer, name string) error {
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return fmt.Errorf("list concepts: %w", err)
	}
	doc := JSONTermBase{
		Name:     name,
		Version:  "1.0",
		Concepts: concepts,
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return fmt.Errorf("encode JSON termbase: %w", err)
	}
	return nil
}
