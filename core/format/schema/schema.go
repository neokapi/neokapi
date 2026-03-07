package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gokapi/gokapi/core/preset"
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

	// Properties contains the raw schema structure (section objects).
	Properties map[string]PropertySchema `json:"properties"`

	// FlatProperties maps flat Okapi parameter names to their schemas.
	// Built from x-flattenPath annotations during loading.
	FlatProperties map[string]PropertySchema `json:"-"`

	// SectionMap maps flat parameter names to their schema section keys.
	// Used to wrap flat params into the hierarchical format the bridge expects.
	// E.g., "elements" → "elements", "extractAllPairs" → "extraction".
	SectionMap map[string]string `json:"-"`

	// Raw JSON for full schema access
	RawJSON json.RawMessage `json:"-"`
}

// FilterSchemaMeta contains filter identification metadata.
type FilterSchemaMeta struct {
	ID             string                `json:"id"`
	Class          string                `json:"class"`
	Extensions     []string              `json:"extensions"`
	MimeTypes      []string              `json:"mimeTypes"`
	Configurations []FilterConfiguration `json:"configurations,omitempty"`
}

// FilterConfiguration represents a named configuration from x-filter.configurations.
type FilterConfiguration struct {
	ConfigID    string         `json:"configId"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	MimeType    string         `json:"mimeType"`
	Extensions  string         `json:"extensions"`
	Parameters  map[string]any `json:"parameters"`
	IsDefault   bool           `json:"isDefault"`
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

	// Parameter flattening path (maps hierarchical schema key to flat Okapi name)
	FlattenPath string `json:"x-flattenPath,omitempty"`

	// Nested properties for object types
	Properties map[string]PropertySchema `json:"properties,omitempty"`
}

// SchemaRegistry manages filter parameter schemas.
type SchemaRegistry struct {
	mu            sync.RWMutex
	schemas       map[string]*FilterSchema    // filterID -> schema
	classSections map[string]map[string]string // filterClass -> sectionMap
}

// NewSchemaRegistry creates a new schema registry.
func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		schemas:       make(map[string]*FilterSchema),
		classSections: make(map[string]map[string]string),
	}
}

// RegisterSchema programmatically registers a schema for a filter ID.
// Used by native formats to register their schemas without loading from files.
func (r *SchemaRegistry) RegisterSchema(filterID string, schema *FilterSchema) {
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

	if schema.FilterMeta.Class != "" {
		r.classSections[schema.FilterMeta.Class] = schema.SectionMap
	}

	r.schemas[filterID] = schema
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

// loadSchemaFile loads a single schema file.
func (r *SchemaRegistry) loadSchemaFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading schema file: %w", err)
	}

	var s FilterSchema
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

	// Use the filter ID from x-filter.id, or derive from filename
	filterID := s.FilterMeta.ID
	if filterID == "" {
		// Derive from filename: okf_json.v4.schema.json -> okf_json
		base := filepath.Base(path)
		filterID = strings.TrimSuffix(base, ".schema.json")
		// Strip version suffix: okf_json.v4 -> okf_json
		if idx := strings.LastIndex(filterID, ".v"); idx > 0 {
			filterID = filterID[:idx]
		}
	}

	// Index by filter class for bridge lookups.
	if s.FilterMeta.Class != "" {
		r.classSections[s.FilterMeta.Class] = s.SectionMap
	}

	r.schemas[filterID] = &s
	return nil
}

// GetSchema returns the schema for a filter ID (exact match only).
func (r *SchemaRegistry) GetSchema(filterID string) (*FilterSchema, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.schemas[filterID]
	return s, ok
}

// GetSchemaJSON returns the raw JSON schema for a filter ID.
func (r *SchemaRegistry) GetSchemaJSON(filterID string) (json.RawMessage, bool) {
	s, ok := r.GetSchema(filterID)
	if !ok {
		return nil, false
	}
	return s.RawJSON, true
}

// ListFilters returns metadata for all registered filter schemas.
func (r *SchemaRegistry) ListFilters() []FilterSchemaMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]FilterSchemaMeta, 0, len(r.schemas))
	for _, s := range r.schemas {
		result = append(result, s.FilterMeta)
	}
	return result
}

// FilterIDSet returns a snapshot of all registered filter IDs as a set.
// Used to diff before/after schema loading.
func (r *SchemaRegistry) FilterIDSet() map[string]struct{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := make(map[string]struct{}, len(r.schemas))
	for id := range r.schemas {
		s[id] = struct{}{}
	}
	return s
}

// FilterIDs returns all registered filter IDs (e.g., "okf_html", "okf_json").
func (r *SchemaRegistry) FilterIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.schemas))
	for id := range r.schemas {
		ids = append(ids, id)
	}
	return ids
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
	s, ok := r.GetSchema(filterID)
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
func (r *SchemaRegistry) findSimilarParam(s *FilterSchema, name string) string {
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

// ExtractPresets extracts format presets from x-filter.configurations in loaded schemas.
// For each configuration, it strips the format prefix from configId to get the preset name.
// E.g., "okf_html-wellFormed" → format "okf_html", preset name "wellFormed".
func (r *SchemaRegistry) ExtractPresets(reg *preset.PresetRegistry) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for filterID, s := range r.schemas {
		for _, cfg := range s.FilterMeta.Configurations {
			presetName := cfg.ConfigID
			// Strip format prefix: "okf_html-wellFormed" → "wellFormed"
			if strings.HasPrefix(cfg.ConfigID, filterID+"-") {
				presetName = cfg.ConfigID[len(filterID)+1:]
			}

			source := "schema"
			if s.FilterMeta.Class != "" {
				source = "bridge"
			}

			reg.RegisterFormatPreset(filterID, presetName, &preset.FormatPreset{
				Name:        presetName,
				Description: cfg.Description,
				Format:      filterID,
				Config:      cfg.Parameters,
				Source:      source,
				IsDefault:   cfg.IsDefault,
			})
		}
	}
}
