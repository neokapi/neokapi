package loader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FilterSchema represents a JSON Schema for a filter's parameters.
type FilterSchema struct {
	ID          string `json:"$id"`
	Version     string `json:"$version"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`

	// Filter metadata
	FilterMeta FilterSchemaMeta `json:"x-filter"`

	// Parameter groupings for UI
	Groups []ParameterGroup `json:"x-groups,omitempty"`

	// Properties (parameters)
	Properties map[string]PropertySchema `json:"properties"`

	// Raw JSON for full schema access
	RawJSON json.RawMessage `json:"-"`
}

// FilterSchemaMeta contains filter identification metadata.
type FilterSchemaMeta struct {
	ID         string   `json:"id"`
	Class      string   `json:"class"`
	Extensions []string `json:"extensions"`
	MimeTypes  []string `json:"mimeTypes"`
}

// ParameterGroup defines a UI grouping of parameters.
type ParameterGroup struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Collapsed   bool     `json:"collapsed,omitempty"`
	Fields      []string `json:"fields"`
}

// PropertySchema represents a single parameter's schema.
type PropertySchema struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Deprecated  bool   `json:"deprecated,omitempty"`

	// UI hints
	Widget      string         `json:"x-widget,omitempty"`
	Placeholder string         `json:"x-placeholder,omitempty"`
	Presets     map[string]any `json:"x-presets,omitempty"`
	OkapiFormat string         `json:"x-okapiFormat,omitempty"`

	// Nested properties for object types
	Properties map[string]PropertySchema `json:"properties,omitempty"`
}

// SchemaRegistry manages filter parameter schemas.
type SchemaRegistry struct {
	mu      sync.RWMutex
	schemas map[string]*FilterSchema // filterID -> schema
}

// NewSchemaRegistry creates a new schema registry.
func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		schemas: make(map[string]*FilterSchema),
	}
}

// LoadFromDirectory loads all *.schema.json files from a directory.
func (r *SchemaRegistry) LoadFromDirectory(dir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No schemas directory is OK
		}
		return fmt.Errorf("reading schemas directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".schema.json") {
			continue
		}

		schemaPath := filepath.Join(dir, entry.Name())
		if err := r.loadSchemaFile(schemaPath); err != nil {
			// Log but continue loading other schemas
			fmt.Fprintf(os.Stderr, "warning: failed to load schema %s: %v\n", entry.Name(), err)
		}
	}

	return nil
}

// loadSchemaFile loads a single schema file.
func (r *SchemaRegistry) loadSchemaFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading schema file: %w", err)
	}

	var schema FilterSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return fmt.Errorf("parsing schema JSON: %w", err)
	}

	// Store the raw JSON for full access
	schema.RawJSON = data

	// Use the filter ID from x-filter.id, or derive from filename
	filterID := schema.FilterMeta.ID
	if filterID == "" {
		// Derive from filename: okf_json.schema.json -> okf_json
		base := filepath.Base(path)
		filterID = strings.TrimSuffix(base, ".schema.json")
	}

	r.schemas[filterID] = &schema
	return nil
}

// GetSchema returns the schema for a filter ID.
func (r *SchemaRegistry) GetSchema(filterID string) (*FilterSchema, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try exact match first
	if schema, ok := r.schemas[filterID]; ok {
		return schema, true
	}

	// Try with okf_ prefix
	if !strings.HasPrefix(filterID, "okf_") {
		if schema, ok := r.schemas["okf_"+filterID]; ok {
			return schema, true
		}
	}

	return nil, false
}

// GetSchemaJSON returns the raw JSON schema for a filter ID.
func (r *SchemaRegistry) GetSchemaJSON(filterID string) (json.RawMessage, bool) {
	schema, ok := r.GetSchema(filterID)
	if !ok {
		return nil, false
	}
	return schema.RawJSON, true
}

// ListFilters returns metadata for all registered filter schemas.
func (r *SchemaRegistry) ListFilters() []FilterSchemaMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]FilterSchemaMeta, 0, len(r.schemas))
	for _, schema := range r.schemas {
		result = append(result, schema.FilterMeta)
	}
	return result
}

// HasSchema returns true if a schema exists for the filter ID.
func (r *SchemaRegistry) HasSchema(filterID string) bool {
	_, ok := r.GetSchema(filterID)
	return ok
}

// Count returns the number of loaded schemas.
func (r *SchemaRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.schemas)
}

// ValidateParams validates filter parameters against the schema.
// Returns nil if valid, or an error describing validation failures.
func (r *SchemaRegistry) ValidateParams(filterID string, params map[string]any) error {
	schema, ok := r.GetSchema(filterID)
	if !ok {
		// No schema available - skip validation
		return nil
	}

	var errors []string

	for paramName, value := range params {
		prop, ok := schema.Properties[paramName]
		if !ok {
			// Unknown parameter
			suggestion := r.findSimilarParam(schema, paramName)
			if suggestion != "" {
				errors = append(errors, fmt.Sprintf("%s: unknown parameter (did you mean %q?)", paramName, suggestion))
			} else {
				errors = append(errors, fmt.Sprintf("%s: unknown parameter", paramName))
			}
			continue
		}

		// Type validation
		if err := validateType(paramName, value, prop.Type); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("invalid filter parameters for %s:\n  %s", filterID, strings.Join(errors, "\n  "))
	}

	return nil
}

// findSimilarParam finds a similar parameter name for suggestions.
func (r *SchemaRegistry) findSimilarParam(schema *FilterSchema, name string) string {
	name = strings.ToLower(name)
	for paramName := range schema.Properties {
		if strings.ToLower(paramName) == name {
			return paramName
		}
		// Check for common typos (missing 's', wrong case)
		if strings.HasPrefix(strings.ToLower(paramName), name) ||
			strings.HasPrefix(name, strings.ToLower(paramName)) {
			return paramName
		}
	}
	return ""
}

// validateType checks if a value matches the expected JSON Schema type.
func validateType(paramName string, value any, expectedType string) error {
	if value == nil {
		return nil // Null is valid for any type
	}

	switch expectedType {
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: expected boolean, got %T", paramName, value)
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s: expected string, got %T", paramName, value)
		}
	case "integer":
		switch v := value.(type) {
		case int, int64, float64:
			if f, ok := v.(float64); ok && f != float64(int(f)) {
				return fmt.Errorf("%s: expected integer, got float", paramName)
			}
		default:
			return fmt.Errorf("%s: expected integer, got %T", paramName, value)
		}
	case "number":
		switch value.(type) {
		case int, int64, float64:
			// OK
		default:
			return fmt.Errorf("%s: expected number, got %T", paramName, value)
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("%s: expected object, got %T", paramName, value)
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("%s: expected array, got %T", paramName, value)
		}
	}

	return nil
}
