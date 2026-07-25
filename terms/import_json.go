package terms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// JSONStore is the JSON-serializable representation of a terms.
type JSONStore struct {
	Name     string    `json:"name"`
	Version  string    `json:"version"`
	Concepts []Concept `json:"concepts"`
}

// ImportJSON reads a JSON terms file and imports all concepts.
func ImportJSON(ctx context.Context, tb Terminology, reader io.Reader) (int, error) {
	var doc JSONStore
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&doc); err != nil {
		return 0, fmt.Errorf("parse JSON terms: %w", err)
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
