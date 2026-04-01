// Package schema provides generalized JSON Schema types for component configuration.
// Both format filters and tools use these types to declare their parameters.
package schema

import "encoding/json"

// ComponentSchema represents a JSON Schema for a component's parameters.
// It supports parameter grouping, UI hints, and validation metadata.
type ComponentSchema struct {
	ID          string `json:"$id,omitempty"`
	Version     string `json:"$version,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"` // "object"

	// Component metadata
	Meta ComponentMeta `json:"x-component,omitempty"`

	// Parameter groupings for UI
	Groups []ParameterGroup `json:"x-groups,omitempty"`

	// Properties contains the parameter definitions.
	Properties map[string]PropertySchema `json:"properties,omitempty"`

	// Raw JSON for full schema access
	RawJSON json.RawMessage `json:"-"`
}

// ComponentMeta identifies the component this schema belongs to.
type ComponentMeta struct {
	ID          string `json:"id"`
	Type        string `json:"type"`                  // "format", "tool", "step"
	Category    string `json:"category,omitempty"`     // "translate","validate","enrich","convert","transform","pipeline"
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`

	// Inputs declares which part types this component accepts.
	// Empty means the component accepts all part types (pass-through).
	Inputs []string `json:"inputs,omitempty"` // "block","data","media","layer","group"

	// Outputs declares which part types this component produces or modifies.
	// Empty means same as inputs (in-place modification).
	Outputs []string `json:"outputs,omitempty"`

	// Tags are freeform classification labels for UI filtering and grouping.
	Tags []string `json:"tags,omitempty"` // "ai-powered","batch","regex","configurable"

	// Requires declares external resources this component needs at runtime.
	Requires []string `json:"requires,omitempty"` // "target-language","source-language","tm","termbase","credentials"
}

// Standard part type names for Inputs/Outputs declarations.
const (
	PartTypeBlock = "block"
	PartTypeData  = "data"
	PartTypeMedia = "media"
	PartTypeLayer = "layer"
	PartTypeGroup = "group"
)

// Standard tool categories.
const (
	CategoryTranslate = "translate"
	CategoryValidate  = "validate"
	CategoryEnrich    = "enrich"
	CategoryConvert   = "convert"
	CategoryTransform = "transform"
	CategoryPipeline  = "pipeline"
)

// Standard requirement names for the Requires field.
const (
	RequiresTargetLanguage = "target-language"
	RequiresSourceLanguage = "source-language"
	RequiresTM             = "tm"
	RequiresTermbase       = "termbase"
	RequiresCredentials    = "credentials"
	RequiresRetryable      = "retryable"
)

// ParameterGroup defines a UI grouping of parameters.
type ParameterGroup struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Collapsed   bool     `json:"collapsed,omitempty"`
	Fields      []string `json:"fields"`
}

// ShowIfRule controls conditional visibility of a property.
// The property is shown only when the referenced field matches the given value.
type ShowIfRule struct {
	Field string `json:"field"`          // name of the field to check
	Value any    `json:"value"`          // value that makes this field visible
	Empty bool   `json:"empty,omitempty"` // if true, show when the field is empty/unset
}

// PropertySchema represents a single parameter's schema.
type PropertySchema struct {
	Type        string `json:"type"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Deprecated  bool   `json:"deprecated,omitempty"`

	// Validation constraints
	Enum      []any    `json:"enum,omitempty"`
	Min       *float64 `json:"minimum,omitempty"`
	Max       *float64 `json:"maximum,omitempty"`
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`

	// UI hints
	Widget      string         `json:"x-widget,omitempty"`
	Placeholder string         `json:"x-placeholder,omitempty"`
	Presets     map[string]any `json:"x-presets,omitempty"`
	ShowIf      *ShowIfRule    `json:"x-showIf,omitempty"`

	// Nested properties for object types
	Properties map[string]PropertySchema `json:"properties,omitempty"`

	// Array item schema
	Items *PropertySchema `json:"items,omitempty"`
}

// Validate checks parameter values against this schema.
func (s *ComponentSchema) Validate(params map[string]any) []ValidationError {
	if s == nil || len(s.Properties) == 0 {
		return nil
	}
	var errs []ValidationError
	for name, value := range params {
		prop, ok := s.Properties[name]
		if !ok {
			errs = append(errs, ValidationError{
				Field:   name,
				Message: "unknown parameter",
			})
			continue
		}
		if err := validateValue(name, value, &prop); err != nil {
			errs = append(errs, *err)
		}
	}
	return errs
}

// ValidationError describes a single validation failure.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

func validateValue(name string, value any, prop *PropertySchema) *ValidationError {
	if value == nil {
		return nil
	}
	switch prop.Type {
	case "boolean":
		if _, ok := value.(bool); !ok {
			return &ValidationError{Field: name, Message: "expected boolean"}
		}
	case "string":
		s, ok := value.(string)
		if !ok {
			return &ValidationError{Field: name, Message: "expected string"}
		}
		if len(prop.Enum) > 0 && !enumContains(prop.Enum, s) {
			return &ValidationError{Field: name, Message: "value not in allowed enum"}
		}
	case "integer":
		switch v := value.(type) {
		case int:
			// ok
		case int64:
			// ok
		case float64:
			if v != float64(int(v)) {
				return &ValidationError{Field: name, Message: "expected integer, got float"}
			}
		default:
			return &ValidationError{Field: name, Message: "expected integer"}
		}
	case "number":
		switch value.(type) {
		case int, int64, float64:
			// ok
		default:
			return &ValidationError{Field: name, Message: "expected number"}
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return &ValidationError{Field: name, Message: "expected object"}
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return &ValidationError{Field: name, Message: "expected array"}
		}
	}
	return nil
}

func enumContains(enum []any, value any) bool {
	for _, v := range enum {
		if v == value {
			return true
		}
	}
	return false
}

// MarshalJSON builds the raw JSON representation.
func (s *ComponentSchema) MarshalJSON() ([]byte, error) {
	type Alias ComponentSchema
	return json.Marshal((*Alias)(s))
}

// BuildRawJSON pre-builds and caches the JSON representation.
func (s *ComponentSchema) BuildRawJSON() {
	if data, err := json.Marshal(s); err == nil {
		s.RawJSON = data
	}
}
