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

// CommandFlag describes one flag on a CLI command.
type CommandFlag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Usage     string `json:"usage,omitempty"`
	Default   string `json:"default,omitempty"`
	Type      string `json:"type,omitempty"`
}

// CommandEntry describes one command or subcommand in the kapi CLI.
// The JSON shape is consumed by packages/reference-data/src/types.ts.
type CommandEntry struct {
	// ID is the dot-joined command path, e.g. "formats.list".
	ID string `json:"id"`
	// Path is the ordered list of command names from root to this command,
	// e.g. ["formats", "list"]. The root "kapi" is excluded.
	Path []string `json:"path"`
	// Use is the cobra Use string (first word is the command name).
	Use string `json:"use"`
	// Short is the one-line description.
	Short string `json:"short,omitempty"`
	// Long is the multi-line description.
	Long string `json:"long,omitempty"`
	// Aliases is the list of alternative command names.
	Aliases []string `json:"aliases,omitempty"`
	// GroupID is the cobra command group, e.g. "processing", "management".
	GroupID string `json:"groupID,omitempty"`
	// Flags are the command-local flags (not inherited persistent flags).
	Flags []CommandFlag `json:"flags,omitempty"`
	// Examples is the cobra Example string, split into individual lines.
	Examples []string `json:"examples,omitempty"`
	// OfflineCapable is true when this command needs no network (editorial
	// "run here vs watch" signal). Derived from a curated override map + heuristic.
	OfflineCapable bool `json:"offlineCapable"`
	// RunnableInBrowser is true when the command is present in the kapi-wasm-cli
	// buildRoot and can actually execute in the playground — offline commands
	// directly, and AI/MT commands via the demo provider. This gates the Run button.
	RunnableInBrowser bool `json:"runnableInBrowser"`
	// DemoMode is true for commands that run in-browser only via the deterministic
	// demo provider (AI/MT): their output is illustrative, not from a real model.
	DemoMode bool `json:"demoMode"`
}

// CommandDataset is the top-level JSON document for commands.json.
type CommandDataset struct {
	GeneratedAt string         `json:"generatedAt"`
	Commands    []CommandEntry `json:"commands"`
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
