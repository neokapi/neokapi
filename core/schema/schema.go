// Package schema provides generalized JSON Schema types for component configuration.
// Both format filters and tools use these types to declare their parameters.
package schema

import (
	"encoding/json"

	"github.com/neokapi/neokapi/core/model"
)

// ComponentSchema represents a JSON Schema for a component's parameters.
// It supports parameter grouping, UI hints, and validation metadata.
//
// Extension namespaces:
//   - ui:*          — UI rendering hints (widget, visibility, layout, groups)
//   - (no prefix)   — neokapi data/metadata (formatMeta, toolMeta, presets)
//   - x-okapi-*     — Okapi bridge internals (produced by okapi-bridge only)
type ComponentSchema struct {
	ID          string `json:"$id,omitempty"`
	Version     string `json:"$version,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"` // "object"

	// Component metadata (tool identification)
	ToolMeta *ToolMeta `json:"toolMeta,omitempty"`

	// Parameter groupings for UI
	Groups []ParameterGroup `json:"ui:groups,omitempty"`

	// StepMeta holds Okapi bridge step metadata (only present for bridge step tools).
	StepMeta *StepMeta `json:"x-step,omitempty"`

	// Properties contains the parameter definitions.
	Properties map[string]PropertySchema `json:"properties,omitempty"`

	// Raw JSON for full schema access
	RawJSON json.RawMessage `json:"-"`
}

// Port pseudo-type names for the IO contract: produced/consumed outputs that
// are not stand-off layers but participate in data-flow validation. PortTarget
// is the committed Target; PortSource is a rewritten source.
const (
	PortTarget = "target"
	PortSource = "source"
)

// IOPort is one entry in a tool's IO contract: a typed stand-off output the
// tool consumes (reads upstream) or produces (writes). Type names an overlay
// type (OverlayTerm, OverlayQA, …), a block-annotation type (AnnoBrandVoice,
// …), or a pseudo-port (PortTarget/PortSource). Optional consumed ports enable
// graceful degradation — the tool runs without them and does more with them;
// non-optional consumed ports are hard requirements the flow validator enforces
// against upstream producers and the source binding.
type IOPort struct {
	Type     string     `json:"type"`
	Side     model.Side `json:"side,omitempty"`
	Optional bool       `json:"optional,omitempty"`
	Layer    string     `json:"layer,omitempty"` // segmentation granularity; "" = primary
}

// Port builds an IOPort for any stand-off type name — an OverlayType, a block
// annotation key, or a pseudo-port constant — without a string() at call sites.
func Port[T ~string](t T, side model.Side) IOPort { return IOPort{Type: string(t), Side: side} }

// ToolMeta identifies a tool and its capabilities. The IO contract is expressed
// over typed ports (Consumes/Produces, AD-006); part-type Inputs/Outputs are retired
// — any coarse part-type set the runtime needs is derived from the tool's
// capability and handlers, not declared here.
type ToolMeta struct {
	ID          string `json:"id"`
	Category    string `json:"category,omitempty"` // canonical: see Category* consts (translation/quality/analysis/text-processing/convert/pipeline)
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`

	// Tags are freeform classification labels for UI filtering and grouping.
	Tags []string `json:"tags,omitempty"` // "ai-powered","batch","regex","configurable"

	// Requires declares external resources this tool needs at runtime.
	Requires []string `json:"requires,omitempty"` // "target-language","source-language","tm","termbase","credentials"

	// Cardinality declares how many locales the tool operates on per execution.
	Cardinality LocaleCardinality `json:"cardinality,omitempty"`

	// DefaultLocale is an optional fallback locale for the tool.
	// For bilingual tools, this is the default second locale (e.g., "qps"
	// for pseudo-translate). Empty means the runner must provide one.
	DefaultLocale model.LocaleID `json:"defaultLocale,omitempty"`

	// Consumes lists the ports this tool reads upstream. Non-Optional entries
	// are requirements; Optional entries upgrade behaviour when present.
	Consumes []IOPort `json:"consumes,omitempty"`

	// Produces lists the ports this tool writes to Blocks.
	Produces []IOPort `json:"produces,omitempty"`

	// SideEffects lists external systems this tool reads from or writes to.
	SideEffects []SideEffect `json:"sideEffects,omitempty"`

	// Recoverable marks a transformer that vaults the originals it removes and
	// restores them later (redaction). The placement pass (AD-006) holds such a
	// transformer to the remote-egress rule: it must run before any step that
	// sends source to a remote sink, because its purpose is protecting content
	// from exactly that egress.
	Recoverable bool `json:"recoverable,omitempty"`

	// WritesOutput indicates the tool produces modified output files.
	// When true, the CLI adds an -o/--output flag.
	WritesOutput bool `json:"writesOutput,omitempty"`

	// DefaultParallelBlocks is the default number of blocks to process
	// concurrently for IO-bound tools (e.g., AI-powered). 0 means sequential.
	DefaultParallelBlocks int `json:"defaultParallelBlocks,omitempty"`

	// Aliases lists alternative CLI command names (e.g., "translate" for "ai-translate").
	Aliases []string `json:"aliases,omitempty"`
}

// StepMeta holds metadata for Okapi bridge pipeline steps. Parsed from the
// "x-step" section of bridge step schema JSON files. Used for pipeline
// composition — steps with compatible inputType/outputType can be chained
// inside a single Java bridge Process RPC.
type StepMeta struct {
	Class             string   `json:"class"`                       // Fully-qualified Java class name
	InputType         string   `json:"inputType,omitempty"`         // "filter-events" or "raw-document"
	OutputType        string   `json:"outputType,omitempty"`        // "filter-events" or "file"
	ParameterMappings []string `json:"parameterMappings,omitempty"` // Runtime parameter injection points
}

// Standard part type names for Inputs/Outputs declarations.
const (
	PartTypeBlock = "block"
	PartTypeData  = "data"
	PartTypeMedia = "media"
	PartTypeLayer = "layer"
	PartTypeGroup = "group"
)

// Standard tool categories — the single canonical vocabulary shared by native
// tools, okapi-bridge tools (via NormalizeCategory), gen-refs, and the flow
// editor. The first four double as CLI command group IDs and must match the
// cobra group IDs in cli.AddCommandGroups.
const (
	CategoryTranslation    = "translation"     // produces target content
	CategoryQuality        = "quality"         // validates target / produces qa·term findings
	CategoryAnalysis       = "analysis"        // read-only metrics / reports
	CategoryTextProcessing = "text-processing" // rewrites / segments / redacts source
	CategoryConvert        = "convert"         // format conversion
	CategoryPipeline       = "pipeline"        // composite / sub-pipeline
)

// bridgeCategoryAliases maps the okapi-bridge category vocabulary onto the
// canonical set above. The bridge classifies steps with its own labels
// (translate/validate/transform/…); NormalizeCategory folds them in so the
// whole stack speaks one vocabulary.
var bridgeCategoryAliases = map[string]string{
	"translate": CategoryTranslation,
	"validate":  CategoryQuality,
	"transform": CategoryTextProcessing,
	"enrich":    CategoryAnalysis,
}

// NormalizeCategory maps any tool category string onto the canonical vocabulary.
// Canonical values pass through unchanged; bridge aliases are folded in; an
// empty value falls back to CategoryPipeline (the neutral group); an unknown
// value passes through so genuinely new categories are visible, not swallowed.
func NormalizeCategory(c string) string {
	switch c {
	case CategoryTranslation, CategoryQuality, CategoryAnalysis,
		CategoryTextProcessing, CategoryConvert, CategoryPipeline:
		return c
	}
	if canon, ok := bridgeCategoryAliases[c]; ok {
		return canon
	}
	if c == "" {
		return CategoryPipeline
	}
	return c
}

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
	Collapsible *bool    `json:"collapsible,omitempty"`
	Collapsed   bool     `json:"collapsed,omitempty"`
	Icon        string   `json:"icon,omitempty"` // lucide icon name
	Fields      []string `json:"fields"`
}

// ConditionExpr is an expression for conditional visibility/enablement.
// Supports simple field comparisons and compound AND/OR/NOT.
//
// Examples:
//
//	{ "field": "mode", "eq": "advanced" }
//	{ "field": "path", "empty": true }
//	{ "all": [{ "field": "a", "eq": true }, { "field": "b", "eq": true }] }
//	{ "not": { "field": "mode", "eq": "simple" } }
type ConditionExpr struct {
	// Simple condition: field comparison
	Field string `json:"field,omitempty"`
	Eq    any    `json:"eq,omitempty"`
	Empty *bool  `json:"empty,omitempty"`

	// Compound conditions
	All []*ConditionExpr `json:"all,omitempty"`
	Any []*ConditionExpr `json:"any,omitempty"`
	Not *ConditionExpr   `json:"not,omitempty"`
}

// PropertySchema represents a single parameter's schema.
type PropertySchema struct {
	Type        string `json:"type"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Deprecated  bool   `json:"deprecated,omitempty"`

	// Validation constraints
	Min       *float64 `json:"minimum,omitempty"`
	Max       *float64 `json:"maximum,omitempty"`
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`

	// Labeled enum options (consolidated from enum + ui:enum-labels).
	// Each option has a value and a human label.
	Options []OptionItem `json:"options,omitempty"`

	// UI rendering hints (ui: prefix)
	Widget            string            `json:"ui:widget,omitempty"`
	WidgetOptions     map[string]any    `json:"ui:widget-options,omitempty"`
	Placeholder       string            `json:"ui:placeholder,omitempty"`
	Visible           *ConditionExpr    `json:"ui:visible,omitempty"`
	Enabled           *ConditionExpr    `json:"ui:enabled,omitempty"`
	Layout            *LayoutHints      `json:"ui:layout,omitempty"`
	EnumDescriptions  map[string]string `json:"ui:enum-descriptions,omitempty"`
	Order             *int              `json:"ui:order,omitempty"`
	DeprecatedMessage string            `json:"ui:deprecated-message,omitempty"`
	IntroducedIn      string            `json:"ui:introduced-in,omitempty"`

	// Nested properties for object types
	Properties map[string]PropertySchema `json:"properties,omitempty"`

	// Array item schema
	Items *PropertySchema `json:"items,omitempty"`

	// Path annotation for resource/file path properties
	PathInfo *PathAnnotation `json:"x-path,omitempty"`
}

// OptionItem represents a labeled enum value for select/radio properties.
type OptionItem struct {
	Value any    `json:"value"`
	Label string `json:"label"`
}

// PathAnnotation describes a property that references a file path or named resource.
// Used by the resource resolver to resolve URI prefixes (tm:, termbase:, srx:) and
// relative paths, and by the UI to render ResourcePicker widgets.
type PathAnnotation struct {
	// Type is "file" or "directory".
	Type string `json:"type,omitempty"`

	// Role is "input" or "output". Output paths get auto-placed into the output directory.
	Role string `json:"role,omitempty"`

	// ResourceKind enables URI prefix resolution: "tm", "termbase", or "srx".
	ResourceKind string `json:"resourceKind,omitempty"`

	// Accepts lists file extensions for validation and UI filtering (e.g. ["html", "txt"]).
	Accepts []string `json:"accepts,omitempty"`

	// BrowseTitle is the file dialog title for file/folder picker UI.
	BrowseTitle string `json:"browseTitle,omitempty"`

	// ForSaveAs indicates the file dialog should be a save-as dialog.
	ForSaveAs bool `json:"forSaveAs,omitempty"`

	// Filters lists file type filters for the file dialog (e.g. [{name: "HTML", extensions: "*.html"}]).
	Filters []FileFilter `json:"filters,omitempty"`
}

// FileFilter describes a file type filter for file picker dialogs.
type FileFilter struct {
	Name       string `json:"name"`
	Extensions string `json:"extensions"`
}

// LayoutHints controls field-level layout.
type LayoutHints struct {
	HideLabel bool `json:"hideLabel,omitempty"`
	Vertical  bool `json:"vertical,omitempty"`
	Columns   int  `json:"columns,omitempty"`
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
		if len(prop.Options) > 0 && !optionContains(prop.Options, s) {
			return &ValidationError{Field: name, Message: "value not in allowed options"}
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

func optionContains(options []OptionItem, value any) bool {
	for _, opt := range options {
		if opt.Value == value {
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
