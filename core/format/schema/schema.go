package schema

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/preset"
	coreschema "github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/set"
)

// Re-export shared types from core/schema for convenience.
// These are the authoritative definitions — format/schema consumers
// can use them without importing core/schema directly.
type ParameterGroup = coreschema.ParameterGroup
type ConditionExpr = coreschema.ConditionExpr
type LayoutHints = coreschema.LayoutHints

// FormatSchema represents a JSON Schema for a format's parameters.
type FormatSchema struct {
	ID          string `json:"$id"`
	Version     string `json:"$version"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`

	// Format identification metadata (no prefix — data field)
	FormatMeta FormatMeta `json:"formatMeta"`

	// Named parameter presets (no prefix — data field)
	Presets map[string]map[string]any `json:"presets,omitempty"`

	// Parameter groupings for UI (ui: prefix)
	Groups []ParameterGroup `json:"ui:groups,omitempty"`

	// Properties contains the raw schema structure (section objects).
	Properties map[string]PropertySchema `json:"properties"`

	// FlatProperties maps flat Okapi parameter names to their schemas.
	// Built from x-okapi-flatten-path annotations during loading.
	FlatProperties map[string]PropertySchema `json:"-"`

	// SectionMap maps flat parameter names to their schema section keys.
	// Used to wrap flat params into the hierarchical format the bridge expects.
	SectionMap map[string]string `json:"-"`

	// Raw JSON for full schema access
	RawJSON json.RawMessage `json:"-"`
}

// FormatMeta contains format identification metadata.
type FormatMeta struct {
	ID         string   `json:"id"`
	Extensions []string `json:"extensions"`
	MimeTypes  []string `json:"mimeTypes"`

	// Class is the Java filter class (only present in bridge schemas).
	// Kept for bridge runtime lookups but not part of the public schema language.
	Class string `json:"class,omitempty"`
}

// PropertySchema extends core/schema.PropertySchema with okapi-bridge fields.
// All standard JSON Schema fields and ui:* rendering hints come from the
// embedded core type. Only bridge-specific extensions are added here.
type PropertySchema struct {
	coreschema.PropertySchema

	// Okapi bridge extensions (x-okapi- prefix, only in bridge schemas)
	OkapiFormat string `json:"x-okapi-format,omitempty"`
	FlattenPath string `json:"x-okapi-flatten-path,omitempty"`

	// Nested properties for object types — redeclared to use this type
	// so recursive structures use PropertySchema (with bridge fields),
	// not the embedded core type's Properties.
	Properties map[string]PropertySchema `json:"properties,omitempty"`
}

// Prop wraps a core PropertySchema into a format PropertySchema.
// Keeps native format schema.go files concise.
func Prop(p coreschema.PropertySchema) PropertySchema {
	return PropertySchema{PropertySchema: p}
}

// SchemaRegistry manages filter parameter schemas.
type SchemaRegistry struct {
	mu            sync.RWMutex
	schemas       map[string]*FormatSchema     // formatID -> schema
	classSections map[string]map[string]string // filterClass -> sectionMap
}

// NewSchemaRegistry creates a new schema registry.
func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		schemas:       make(map[string]*FormatSchema),
		classSections: make(map[string]map[string]string),
	}
}

// RegisterSchema programmatically registers a schema for a format ID.
// Used by native formats to register their schemas without loading from files.
func (r *SchemaRegistry) RegisterSchema(formatID string, schema *FormatSchema) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Build RawJSON if not set.
	if schema.RawJSON == nil {
		if data, err := json.Marshal(schema); err == nil {
			schema.RawJSON = data
		}
	}

	// Build flat properties if not set.
	if schema.FlatProperties == nil {
		schema.FlatProperties = make(map[string]PropertySchema)
		schema.SectionMap = make(map[string]string)
		for sectionKey, section := range schema.Properties {
			if section.Type == "object" && len(section.Properties) > 0 {
				for _, prop := range section.Properties {
					if prop.FlattenPath != "" {
						schema.FlatProperties[prop.FlattenPath] = prop
						schema.SectionMap[prop.FlattenPath] = sectionKey
					}
				}
			} else {
				schema.FlatProperties[sectionKey] = section
			}
		}
	}

	if schema.FormatMeta.Class != "" {
		r.classSections[schema.FormatMeta.Class] = schema.SectionMap
	}

	r.schemas[formatID] = schema
}

// GetSectionMap returns the flat-param → section-key mapping for a filter class.
// The bridge uses this to wrap flat parameters into the hierarchical JSON
// structure that the bridge's schema-based ParameterFlattener expects.
func (r *SchemaRegistry) GetSectionMap(filterClass string) map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.classSections[filterClass]
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

// LoadSchemaFile loads a single schema file and registers it under the given format ID.
// If formatID is empty, the ID is derived from x-format.id or the filename.
func (r *SchemaRegistry) LoadSchemaFile(path string, formatID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadSchemaFileWithID(path, formatID)
}

// loadSchemaFile loads a single schema file (internal, no lock).
func (r *SchemaRegistry) loadSchemaFile(path string) error {
	return r.loadSchemaFileWithID(path, "")
}

// loadSchemaFileWithID loads a single schema file with an optional explicit ID.
func (r *SchemaRegistry) loadSchemaFileWithID(path string, explicitID string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading schema file: %w", err)
	}

	var s FormatSchema
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("parsing schema JSON: %w", err)
	}

	// Store the raw JSON for full access
	s.RawJSON = data

	// Build flat properties and section map from hierarchical schema.
	s.FlatProperties = make(map[string]PropertySchema)
	s.SectionMap = make(map[string]string)
	for sectionKey, section := range s.Properties {
		if section.Type == "object" && len(section.Properties) > 0 {
			for _, prop := range section.Properties {
				if prop.FlattenPath != "" {
					s.FlatProperties[prop.FlattenPath] = prop
					s.SectionMap[prop.FlattenPath] = sectionKey
				}
			}
		} else {
			// Top-level non-section properties (e.g., inlineCodes $ref)
			s.FlatProperties[sectionKey] = section
		}
	}

	// Use explicit ID, then x-format.id, then derive from filename
	formatID := explicitID
	if formatID == "" {
		formatID = s.FormatMeta.ID
	}
	if formatID == "" {
		// Derive from filename: okf_json.v4.schema.json -> okf_json
		base := filepath.Base(path)
		formatID = strings.TrimSuffix(base, ".schema.json")
		// Strip version suffix: okf_json.v4 -> okf_json
		if idx := strings.LastIndex(formatID, ".v"); idx > 0 {
			formatID = formatID[:idx]
		}
	}

	// Index by filter class for bridge lookups.
	if s.FormatMeta.Class != "" {
		r.classSections[s.FormatMeta.Class] = s.SectionMap
	}

	r.schemas[formatID] = &s
	return nil
}

// GetSchema returns the schema for a format ID (exact match only).
func (r *SchemaRegistry) GetSchema(formatID string) (*FormatSchema, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.schemas[formatID]
	return s, ok
}

// GetSchemaJSON returns the raw JSON schema for a format ID.
func (r *SchemaRegistry) GetSchemaJSON(formatID string) (json.RawMessage, bool) {
	s, ok := r.GetSchema(formatID)
	if !ok {
		return nil, false
	}
	return s.RawJSON, true
}

// ListFormats returns metadata for all registered format schemas.
func (r *SchemaRegistry) ListFormats() []FormatMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]FormatMeta, 0, len(r.schemas))
	for _, s := range r.schemas {
		result = append(result, s.FormatMeta)
	}
	return result
}

// AllSchemas returns a copy of all registered schemas keyed by format ID.
func (r *SchemaRegistry) AllSchemas() map[string]*FormatSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]*FormatSchema, len(r.schemas))
	maps.Copy(result, r.schemas)
	return result
}

// FormatIDSet returns a snapshot of all registered format IDs as a set.
// Used to diff before/after schema loading.
func (r *SchemaRegistry) FormatIDSet() set.Set[string] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := set.New[string]()
	for id := range r.schemas {
		s.Add(id)
	}
	return s
}

// FormatIDs returns all registered format IDs (e.g., "okf_html", "okf_json").
func (r *SchemaRegistry) FormatIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.schemas))
	for id := range r.schemas {
		ids = append(ids, id)
	}
	return ids
}

// HasSchema returns true if a schema exists for the format ID.
func (r *SchemaRegistry) HasSchema(formatID string) bool {
	_, ok := r.GetSchema(formatID)
	return ok
}

// Count returns the number of loaded schemas.
func (r *SchemaRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.schemas)
}

// ValidateParams validates format parameters against the schema.
// Returns nil if valid, or an error describing validation failures.
func (r *SchemaRegistry) ValidateParams(formatID string, params map[string]any) error {
	s, ok := r.GetSchema(formatID)
	if !ok {
		// No schema available - skip validation
		return nil
	}

	var errors []string

	// Use FlatProperties if available (populated from x-flattenPath),
	// otherwise fall back to raw Properties (for hand-crafted flat schemas in tests).
	props := s.FlatProperties
	if len(props) == 0 {
		props = s.Properties
	}

	for paramName, value := range params {
		prop, ok := props[paramName]
		if !ok {
			// Unknown parameter
			suggestion := r.findSimilarParam(s, paramName)
			if suggestion != "" {
				errors = append(errors, fmt.Sprintf("%s: unknown parameter (did you mean %q?)", paramName, suggestion))
			} else {
				errors = append(errors, paramName+": unknown parameter")
			}
			continue
		}

		// Type validation
		if err := validateType(paramName, value, prop.Type); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("invalid format parameters for %s:\n  %s", formatID, strings.Join(errors, "\n  "))
	}

	return nil
}

// findSimilarParam finds a similar parameter name for suggestions.
func (r *SchemaRegistry) findSimilarParam(s *FormatSchema, name string) string {
	props := s.FlatProperties
	if len(props) == 0 {
		props = s.Properties
	}
	name = strings.ToLower(name)
	for paramName := range props {
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

// ExtractPresets registers format presets from the presets field in loaded schemas.
func (r *SchemaRegistry) ExtractPresets(reg *preset.PresetRegistry) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for formatID, s := range r.schemas {
		source := "schema"
		if s.FormatMeta.Class != "" {
			source = "bridge"
		}

		for presetName, params := range s.Presets {
			reg.RegisterFormatPreset(formatID, presetName, &preset.FormatPreset{
				Name:   presetName,
				Format: formatID,
				Config: params,
				Source: source,
			})
		}
	}
}
