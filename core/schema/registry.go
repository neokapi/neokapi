package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Registry manages component parameter schemas.
type Registry struct {
	mu      sync.RWMutex
	schemas map[string]*ComponentSchema // component ID -> schema
}

// NewRegistry creates a new schema registry.
func NewRegistry() *Registry {
	return &Registry{
		schemas: make(map[string]*ComponentSchema),
	}
}

// Register adds a schema to the registry.
func (r *Registry) Register(id string, schema *ComponentSchema) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if schema.RawJSON == nil {
		schema.BuildRawJSON()
	}
	r.schemas[id] = schema
}

// Get returns the schema for a component ID.
func (r *Registry) Get(id string) (*ComponentSchema, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.schemas[id]
	return s, ok
}

// GetJSON returns the raw JSON schema for a component ID.
func (r *Registry) GetJSON(id string) (json.RawMessage, bool) {
	s, ok := r.Get(id)
	if !ok {
		return nil, false
	}
	return s.RawJSON, true
}

// Has returns true if a schema is registered for the given ID.
func (r *Registry) Has(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.schemas[id]
	return ok
}

// IDs returns all registered schema IDs.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.schemas))
	for id := range r.schemas {
		ids = append(ids, id)
	}
	return ids
}

// List returns all registered schemas.
func (r *Registry) List() []*ComponentSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*ComponentSchema, 0, len(r.schemas))
	for _, s := range r.schemas {
		result = append(result, s)
	}
	return result
}

// Count returns the number of registered schemas.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.schemas)
}

// LoadFromDirectory loads all *.schema.json files from a directory.
func (r *Registry) LoadFromDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading schemas directory: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".schema.json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := r.loadFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to load schema %s: %v\n", entry.Name(), err)
		}
	}
	return nil
}

func (r *Registry) loadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var s ComponentSchema
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("parsing schema: %w", err)
	}
	s.RawJSON = data

	var id string
	if s.ToolMeta != nil {
		id = s.ToolMeta.ID
	}
	if id == "" {
		// Derive from filename: pseudo-translate.schema.json -> pseudo-translate
		base := filepath.Base(path)
		id = strings.TrimSuffix(base, ".schema.json")
	}

	r.schemas[id] = &s
	return nil
}
