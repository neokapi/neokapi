package main

import "encoding/json"

// Source identifies which engine provides an entry.
const (
	SourceBuiltIn = "built-in"
	SourceOkapi   = "okapi"
)

// Kind discriminates formats from tools in the unified dataset.
const (
	KindFormat = "format"
	KindTool   = "tool"
)

// Entry is one format or tool in the reference dataset. The shape is shared
// by the website reference pages and the kapi-desktop Storybook via the
// @neokapi/reference-data package — keep the JSON tags in sync with
// packages/reference-data/src/types.ts.
type Entry struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Kind        string `json:"kind"`
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`

	// Format-only metadata.
	Extensions []string `json:"extensions,omitempty"`
	MimeTypes  []string `json:"mimeTypes,omitempty"`
	HasReader  bool     `json:"hasReader,omitempty"`
	HasWriter  bool     `json:"hasWriter,omitempty"`

	// Tool-only metadata.
	Category    string   `json:"category,omitempty"`
	Inputs      []string `json:"inputs,omitempty"`
	Outputs     []string `json:"outputs,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Requires    []string `json:"requires,omitempty"`
	Cardinality string   `json:"cardinality,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`

	// Schema is the ComponentSchema/FormatSchema JSON consumed verbatim by the
	// shared SchemaForm renderer. Passed through unchanged for bridge entries
	// and marshalled from the Go schema struct for native ones.
	Schema json.RawMessage `json:"schema,omitempty"`

	// Doc holds human-authored reference content (overview, examples,
	// per-parameter help). doc.parameters maps 1:1 to SchemaForm's paramDocs.
	Doc *Doc `json:"doc,omitempty"`

	// Presets are named parameter sets keyed by preset id.
	Presets map[string]map[string]any `json:"presets,omitempty"`
}

// Doc mirrors the bridge doc.json shape and the @neokapi/flow-editor ToolDoc
// type, with an added `config` field on examples for YAML config snippets.
type Doc struct {
	Overview        string              `json:"overview,omitempty"`
	Parameters      map[string]DocParam `json:"parameters,omitempty"`
	Limitations     []string            `json:"limitations,omitempty"`
	ProcessingNotes []string            `json:"processingNotes,omitempty"`
	Examples        []DocExample        `json:"examples,omitempty"`
	WikiURL         string              `json:"wikiUrl,omitempty"`
}

// DocParam is per-parameter help. Field set matches ToolDocParam so it can be
// handed to SchemaForm's paramDocs unchanged.
type DocParam struct {
	Description  string       `json:"description,omitempty"`
	Help         string       `json:"help,omitempty"`
	Values       string       `json:"values,omitempty"`
	Notes        []string     `json:"notes,omitempty"`
	Examples     []string     `json:"examples,omitempty"`
	DependsOn    []DocDepends `json:"dependsOn,omitempty"`
	IntroducedIn string       `json:"introducedIn,omitempty"`
	SeeAlso      string       `json:"seeAlso,omitempty"`
}

// DocDepends records a dependency between parameters for the docs.
type DocDepends struct {
	Property  string `json:"property"`
	Condition string `json:"condition"`
}

// DocExample is a worked configuration example.
type DocExample struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Config      string `json:"config,omitempty"`
	Input       string `json:"input,omitempty"`
	Output      string `json:"output,omitempty"`
}

// Dataset is the top-level JSON document for formats.json / tools.json.
type Dataset struct {
	GeneratedAt string  `json:"generatedAt"`
	Kind        string  `json:"kind"` // "format" | "tool"
	Entries     []Entry `json:"entries"`
}

// Gap is one missing-metadata finding flagged for follow-up.
type Gap struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
	ID     string `json:"id"`
	Field  string `json:"field"`
	Detail string `json:"detail"`
}

// GapReport is the JSON document for reference-gaps.json.
type GapReport struct {
	GeneratedAt string         `json:"generatedAt"`
	Summary     map[string]int `json:"summary"`
	Gaps        []Gap          `json:"gaps"`
}
