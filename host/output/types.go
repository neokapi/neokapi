package output

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// VersionOutput represents version information.
type VersionOutput struct {
	Program string `json:"program"`
	Version string `json:"version"`
	// Channel is the release channel of this build ("beta" for prereleases),
	// shown as a badge so it is obvious a user is on a pre-release. Empty or
	// "stable" renders nothing.
	Channel   string `json:"channel,omitempty"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
}

func (v VersionOutput) FormatText(w io.Writer) error {
	// A leading badge for non-stable builds, e.g. "kapi 1.2.0-rc.1 (beta, …)".
	badge := ""
	if v.Channel != "" && v.Channel != "stable" {
		badge = v.Channel
	}
	switch {
	case badge != "" && v.Commit != "" && v.BuildDate != "":
		fmt.Fprintf(w, "%s %s (%s, commit: %s, built: %s)\n", v.Program, v.Version, badge, v.Commit, v.BuildDate)
	case badge != "":
		fmt.Fprintf(w, "%s %s (%s)\n", v.Program, v.Version, badge)
	case v.Commit != "" && v.BuildDate != "":
		fmt.Fprintf(w, "%s %s (commit: %s, built: %s)\n", v.Program, v.Version, v.Commit, v.BuildDate)
	default:
		fmt.Fprintf(w, "%s %s\n", v.Program, v.Version)
	}
	return nil
}

// FormatInfo represents a single format entry.
type FormatInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	HasReader   bool   `json:"has_reader"`
	HasWriter   bool   `json:"has_writer"`
	// Generative reports the writer can produce a complete document from the
	// content model alone — so this format is a valid cross-format conversion
	// target (`kconv --to`). False = skeleton-bound (writes back into its own
	// original file only). Declarative; resolved without loading any plugin.
	Generative bool `json:"generative,omitempty"`
	// Interchange reports a bilingual translation-interchange format (XLIFF, PO,
	// TMX, …) — the extract/merge loop, excluded as a `convert` target.
	Interchange bool `json:"interchange,omitempty"`
	// Editable reports kapi can faithfully write this format back after an edit
	// (reader + writer, not interchange) — the set a caller-supplied content edit
	// (`kapi apply` / `kapi rewrite --edits`) can target, including binary.
	Editable bool `json:"editable"`
	// RoundTrip reports the writer reconstructs from a skeleton, so an edit
	// changes only the edited text and the rest is byte-for-byte preserved.
	RoundTrip  bool     `json:"round_trip,omitempty"`
	Source     string   `json:"source,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
	MimeTypes  []string `json:"mime_types,omitempty"`
}

// FormatsListOutput represents the list of formats.
type FormatsListOutput struct {
	Formats []FormatInfo `json:"formats"`
	Total   int          `json:"total"`
}

func (f FormatsListOutput) FormatText(w io.Writer) error {
	Title(w, T("formats.available"))

	t := NewTable(w).Accent(0).Headers(
		T("formats.header.format"), T("formats.header.name"), T("formats.header.read"),
		T("formats.header.write"), T("formats.header.edit"), T("formats.header.source"),
		T("formats.header.extensions"), T("formats.header.mimeTypes"))
	s := t.Styles()

	for _, info := range f.Formats {
		// Faithful round-tripping is the stronger guarantee, so it reads as a
		// word rather than a checkmark; plain editable is a plain yes.
		edit := s.Muted.Render("—")
		switch {
		case info.Editable && info.RoundTrip:
			edit = s.Success.Render("faithful")
		case info.Editable:
			edit = s.Success.Render("yes")
		}
		t.Row(
			info.Name,
			info.DisplayName,
			s.YesNo(info.HasReader),
			s.YesNo(info.HasWriter),
			edit,
			s.Dim(info.Source),
			s.Dim(strings.Join(info.Extensions, ", ")),
			s.Dim(strings.Join(info.MimeTypes, ", ")),
		)
	}
	t.Render()

	fmt.Fprintln(w)
	Note(w, T("formats.total"), f.Total)
	return nil
}

// ToolInfo represents a single tool entry.
type ToolInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	Source      string `json:"source,omitempty"`
}

// ToolsListOutput represents the list of tools.
type ToolsListOutput struct {
	Tools []ToolInfo `json:"tools"`
	Total int        `json:"total"`
}

// categoryOrder defines the display order for tool categories.
var categoryOrder = []string{"translation", "quality", "analysis", "text-processing"}

// categoryTitles maps category IDs to display titles.
var categoryTitles = map[string]string{
	"translation":     "Translation",
	"quality":         "Quality",
	"analysis":        "Analysis",
	"text-processing": "Text Processing",
}

func (t ToolsListOutput) FormatText(w io.Writer) error {
	if len(t.Tools) == 0 {
		fmt.Fprintln(w, T("tools.none"))
		return nil
	}

	// Group tools by category, preserving order within each group.
	grouped := make(map[string][]ToolInfo)
	for _, tool := range t.Tools {
		cat := tool.Category
		if cat == "" {
			cat = "other"
		}
		grouped[cat] = append(grouped[cat], tool)
	}

	section := func(title string, tools []ToolInfo) {
		Title(w, title+":")
		tbl := NewTable(w).Accent(0).
			Headers(T("tools.header.tool"), T("tools.header.description"))
		s := tbl.Styles()
		for _, tool := range tools {
			tbl.Row(tool.Name, s.Text(tool.Description))
		}
		tbl.Render()
		fmt.Fprintln(w)
	}

	Title(w, T("tools.available"))

	for _, cat := range categoryOrder {
		if tools := grouped[cat]; len(tools) > 0 {
			section(T("tools.category."+cat), tools)
		}
	}

	// Print any tools in unknown categories.
	for cat, tools := range grouped {
		if categoryTitles[cat] != "" {
			continue
		}
		section(T("tools.category.other"), tools)
	}

	Note(w, T("tools.total"), t.Total)
	return nil
}

// FlowInfo represents a single flow entry.
type FlowInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path,omitempty"`
	Steps       int    `json:"steps,omitempty"`
}

// FlowsListOutput represents the list of flows.
type FlowsListOutput struct {
	Flows []FlowInfo `json:"flows"`
	Total int        `json:"total"`
}

func (f FlowsListOutput) FormatText(w io.Writer) error {
	if len(f.Flows) == 0 {
		fmt.Fprintln(w, T("flows.none"))
		return nil
	}

	Title(w, T("flows.available"))

	t := NewTable(w).Accent(0).Headers(
		T("flows.header.flow"), T("flows.header.description"), T("flows.header.steps"))
	s := t.Styles()
	for _, flow := range f.Flows {
		steps := ""
		if flow.Steps > 0 {
			steps = strconv.Itoa(flow.Steps)
		}
		// The description is primary content, so it stays in the default
		// foreground; only the step count, which is often absent, is muted.
		t.Row(flow.Name, s.Text(flow.Description), s.Dim(steps))
	}
	t.Render()

	fmt.Fprintln(w)
	Note(w, T("flows.total"), f.Total)
	return nil
}

// FlowStats holds optional processing statistics collected during a flow run.
type FlowStats struct {
	BlockCount int `json:"block_count"`
	PartCount  int `json:"part_count"`
}

// FlowRunOutput represents the result of a flow run.
type FlowRunOutput struct {
	FlowName       string     `json:"flow_name"`
	InputPath      string     `json:"input_path,omitempty"`
	OutputPath     string     `json:"output_path,omitempty"`
	FilesProcessed int        `json:"files_processed,omitempty"`
	Stats          *FlowStats `json:"stats,omitempty"`
	// ProcessOnly reports the in-project store-committing run (AD-026 §3):
	// the run landed `targets/<locale>` overlays in the project block store
	// instead of writing an output file.
	ProcessOnly bool `json:"process_only,omitempty"`
}

func (o FlowRunOutput) FormatText(w io.Writer) error {
	s := Theme(w)
	if o.ProcessOnly {
		fmt.Fprintf(w,
			"Committed overlays for %s to the project store. Run `kapi merge` to write target files.\n",
			s.Accent.Render(filepath.Base(o.InputPath)))
		return nil
	}
	if o.FilesProcessed > 0 {
		fmt.Fprintf(w, "Flow %s completed: processed %d files\n",
			s.Accent.Render(o.FlowName), o.FilesProcessed)
	} else {
		fmt.Fprintf(w, "Flow %s completed: %s → %s\n",
			s.Accent.Render(o.FlowName), o.InputPath, o.OutputPath)
	}
	if o.Stats != nil {
		Note(w, "  Parts: %d, Blocks: %d", o.Stats.PartCount, o.Stats.BlockCount)
	}
	return nil
}

// LeverageOutput is a content-memory leverage tally (extract pre-fill).
type LeverageOutput struct {
	Exact int `json:"exact"`
	Fuzzy int `json:"fuzzy"`
	New   int `json:"new"`
}

// Total is the sum of all leverage buckets.
func (l LeverageOutput) Total() int { return l.Exact + l.Fuzzy + l.New }

// ExtractPairOutput is one target-locale pass of an extract batch.
type ExtractPairOutput struct {
	TargetLocale string         `json:"target_locale"`
	Files        int            `json:"files"`
	Blocks       int            `json:"blocks"`
	Leverage     LeverageOutput `json:"leverage"`
}

// ExtractOutput represents the result of `kapi extract` (project batch
// extraction).
type ExtractOutput struct {
	BatchID  string              `json:"batch_id"`
	Format   string              `json:"format"`
	Targets  []string            `json:"targets"`
	Sources  int                 `json:"sources"`
	Manifest string              `json:"manifest"`
	Pairs    []ExtractPairOutput `json:"pairs"`
	Reused   int                 `json:"reused,omitempty"`
	Leverage LeverageOutput      `json:"leverage"`
	Failures int                 `json:"failures,omitempty"`
	// Redaction reporting (set when the batch ran redacted).
	RedactionRules string `json:"redaction_rules,omitempty"`
	RedactionVault string `json:"redaction_vault,omitempty"`
}

func (o ExtractOutput) FormatText(w io.Writer) error {
	s := Theme(w)

	if o.RedactionVault != "" {
		fmt.Fprintln(w, s.Warn.Render(fmt.Sprintf(
			"Redaction enabled (rules=%s): originals stay in %s",
			o.RedactionRules, o.RedactionVault)))
	}
	fmt.Fprintf(w, "Extracting batch %s (format=%s, targets=%s, sources=%d)\n\n",
		s.Accent.Render(o.BatchID), o.Format, strings.Join(o.Targets, ", "), o.Sources)

	// One row per target locale. These were "fr: 2 files, 10 blocks, content memory
	// exact=4 fuzzy=1 new=5" lines — a record set spelled out as prose, which
	// stops being readable the moment there is more than a handful of locales.
	t := NewTable(w).Accent(0).Headers("LOCALE", "FILES", "BLOCKS", "EXACT", "FUZZY", "NEW")
	for _, p := range o.Pairs {
		t.Rowf(p.TargetLocale, p.Files, p.Blocks,
			p.Leverage.Exact, p.Leverage.Fuzzy, p.Leverage.New)
	}
	t.Render()

	fmt.Fprintf(w, "\nBatch %s complete. Manifest: %s\n",
		s.Accent.Render(o.BatchID), s.Muted.Render(o.Manifest))
	if o.Reused > 0 {
		Note(w, "Reused %d unchanged file(s) from a prior batch (no re-parse).", o.Reused)
	}
	Note(w, "Aggregate content-memory leverage: exact=%d fuzzy=%d new=%d (total=%d)",
		o.Leverage.Exact, o.Leverage.Fuzzy, o.Leverage.New, o.Leverage.Total())
	return nil
}

// MergeFileOutput is one merged input file of a `kapi merge -i` run. A file
// that failed carries its error (details also go to stderr as they happen)
// and zero counts.
type MergeFileOutput struct {
	Input         string `json:"input"`
	Applied       int    `json:"applied"`
	Stale         int    `json:"stale"`
	Skipped       int    `json:"skipped"`
	MemoryNew     int    `json:"tm_new"`
	MemoryUpdated int    `json:"tm_updated"`
	Error         string `json:"error,omitempty"`
}

// MergeOutput represents the result of `kapi merge -i` (applying returned
// bilingual files).
type MergeOutput struct {
	Files          []MergeFileOutput `json:"files"`
	Applied        int               `json:"applied"`
	Stale          int               `json:"stale"`
	Skipped        int               `json:"skipped"`
	MemoryNew      int               `json:"tm_new"`
	MemoryUpdated  int               `json:"tm_updated"`
	ConflictPolicy string            `json:"conflict_policy"`
	Failures       int               `json:"failures,omitempty"`
}

func (o MergeOutput) FormatText(w io.Writer) error {
	t := NewTable(w).Accent(0).
		Headers("FILE", "APPLIED", "STALE", "SKIPPED", "content memory NEW", "content memory UPDATED")
	s := t.Styles()

	for _, f := range o.Files {
		if f.Error != "" {
			// The error itself already streamed to stderr when it happened, so
			// the row only has to mark which file it was.
			t.Row(f.Input, s.Error.Render("failed"), "", "", "", "")
			continue
		}
		t.Rowf(f.Input, f.Applied, f.Stale, f.Skipped, f.MemoryNew, f.MemoryUpdated)
	}
	t.Render()

	fmt.Fprintf(w,
		"\nMerge complete. applied=%d stale=%d skipped=%d tm_new=%d tm_updated=%d (conflict_policy=%s)\n",
		o.Applied, o.Stale, o.Skipped, o.MemoryNew, o.MemoryUpdated, o.ConflictPolicy)
	return nil
}

// MergeStoreOutput represents the result of `kapi merge` with no -i in a
// project: localized files materialized from the project block store.
type MergeStoreOutput struct {
	Written          int  `json:"written"`
	FromProjectStore bool `json:"from_project_store"`
}

func (o MergeStoreOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "\nMerge complete. wrote=%d file(s) from the project store.\n", o.Written)
	return nil
}

// PresetEntry represents a single preset in the list output.
type PresetEntry struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Description string         `json:"description,omitempty"`
	Format      string         `json:"format,omitempty"`
	Source      string         `json:"source,omitempty"`
	IsDefault   bool           `json:"is_default,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	Mappings    []MappingEntry `json:"mappings,omitempty"`
	Exclude     []string       `json:"exclude,omitempty"`
}

// MappingEntry represents a mapping in a framework preset.
type MappingEntry struct {
	Local      string `json:"local"`
	Format     string `json:"format,omitempty"`
	TargetPath string `json:"target_path,omitempty"`
}

// PresetsListOutput represents the list of presets.
type PresetsListOutput struct {
	Presets []PresetEntry `json:"presets"`
	Total   int           `json:"total"`
}

func (p PresetsListOutput) FormatText(w io.Writer) error {
	if len(p.Presets) == 0 {
		fmt.Fprintln(w, "No presets available.")
		return nil
	}

	var fwPresets, fmtPresets []PresetEntry
	for _, e := range p.Presets {
		switch e.Type {
		case "framework":
			fwPresets = append(fwPresets, e)
		default:
			fmtPresets = append(fmtPresets, e)
		}
	}

	if len(fwPresets) > 0 {
		Title(w, "Framework Presets:")
		t := NewTable(w).Accent(0).Headers(
			T("presets.header.preset"), T("presets.header.description"), T("presets.header.source"))
		s := t.Styles()
		for _, e := range fwPresets {
			t.Row(e.Name, s.Text(e.Description), s.Dim(e.Source))
		}
		t.Render()
		fmt.Fprintln(w)
	}

	if len(fmtPresets) > 0 {
		Title(w, "Format Presets:")
		t := NewTable(w).Accent(0).Headers(
			T("presets.header.preset"), T("presets.header.format"), T("presets.header.description"))
		s := t.Styles()
		for _, e := range fmtPresets {
			// Default presets are just the format's built-in config, so they say
			// nothing a reader could act on.
			if e.IsDefault {
				continue
			}
			t.Row(e.Name, s.Dim(e.Format), s.Text(e.Description))
		}
		t.Render()
		fmt.Fprintln(w)
	}

	Note(w, "Total: %d preset(s)", p.Total)
	return nil
}

// PresetShowOutput represents the details of a single preset.
type PresetShowOutput struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Description string         `json:"description,omitempty"`
	Format      string         `json:"format,omitempty"`
	Source      string         `json:"source,omitempty"`
	IsDefault   bool           `json:"is_default,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	Mappings    []MappingEntry `json:"mappings,omitempty"`
	Exclude     []string       `json:"exclude,omitempty"`
}

func (p PresetShowOutput) FormatText(w io.Writer) error {
	switch p.Type {
	case "framework":
		fmt.Fprintf(w, "Framework Preset: %s\n", p.Name)
		fmt.Fprintf(w, "Description: %s\n", p.Description)
		fmt.Fprintf(w, "Source: %s\n", p.Source)
		if len(p.Mappings) > 0 {
			fmt.Fprintln(w, "\nMappings:")
			for _, m := range p.Mappings {
				fmt.Fprintf(w, "  local: %s\n", m.Local)
				fmt.Fprintf(w, "  format: %s\n", m.Format)
				if m.TargetPath != "" {
					fmt.Fprintf(w, "  target_path: %s\n", m.TargetPath)
				}
				fmt.Fprintln(w)
			}
		}
		if len(p.Exclude) > 0 {
			fmt.Fprintf(w, "Exclude: %s\n", strings.Join(p.Exclude, ", "))
		}
	default:
		displayName := p.Name
		if p.Format != "" {
			displayName = p.Format + "@" + p.Name
		}
		fmt.Fprintf(w, "Format Preset: %s\n", displayName)
		fmt.Fprintf(w, "Description: %s\n", p.Description)
		fmt.Fprintf(w, "Source: %s\n", p.Source)
		if p.IsDefault {
			fmt.Fprintln(w, "Default: yes")
		}
		if len(p.Config) > 0 {
			fmt.Fprintln(w, "\nConfiguration:")
			for k, v := range p.Config {
				fmt.Fprintf(w, "  %s: %v\n", k, v)
			}
		}
	}
	return nil
}

// PluginSearchEntry represents a single plugin from a registry search.
type PluginSearchEntry struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	PluginType  string `json:"plugin_type,omitempty"`
	Description string `json:"description,omitempty"`
	// Installable is false when the registry has no build for this OS/arch.
	Installable bool `json:"installable"`
}

// PluginSearchOutput represents the result of a plugin search.
type PluginSearchOutput struct {
	Plugins []PluginSearchEntry `json:"plugins"`
	Total   int                 `json:"total"`
}

func (o PluginSearchOutput) FormatText(w io.Writer) error {
	if len(o.Plugins) == 0 {
		fmt.Fprintln(w, "No plugins found.")
		return nil
	}

	t := NewTable(w).Accent(0).Headers(
		T("plugins.header.name"), T("plugins.header.version"),
		T("plugins.header.type"), T("plugins.header.description"))
	s := t.Styles()
	for _, p := range o.Plugins {
		t.Row(p.Name, p.Version, s.Dim(p.PluginType), s.Text(p.Description))
	}
	t.Render()

	fmt.Fprintln(w)
	Note(w, T("plugins.total"), o.Total)
	return nil
}

// FormatInfoParam represents a parameter in a format info group.
type FormatInfoParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Default     any    `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

// FormatInfoGroup represents a group of parameters in format info.
type FormatInfoGroup struct {
	Label       string            `json:"label"`
	Description string            `json:"description,omitempty"`
	Parameters  []FormatInfoParam `json:"parameters"`
}

// FormatInfoOutput represents the detailed info for a single format.
type FormatInfoOutput struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name,omitempty"`
	ConfigKind  string            `json:"config_kind,omitempty"`
	FilterID    string            `json:"filter_id,omitempty"`
	Class       string            `json:"class,omitempty"`
	Version     string            `json:"version,omitempty"`
	Source      string            `json:"source,omitempty"`
	HasReader   bool              `json:"has_reader"`
	HasWriter   bool              `json:"has_writer"`
	Editable    bool              `json:"editable"`
	RoundTrip   bool              `json:"round_trip,omitempty"`
	Extensions  []string          `json:"extensions,omitempty"`
	MimeTypes   []string          `json:"mime_types,omitempty"`
	Groups      []FormatInfoGroup `json:"groups,omitempty"`
	HasSchema   bool              `json:"has_schema"`
}

func (o FormatInfoOutput) FormatText(w io.Writer) error {
	displayName := o.DisplayName
	if displayName == "" {
		displayName = o.Name
	}
	fmt.Fprintf(w, "Format: %s\n", displayName)
	fmt.Fprintln(w)

	if o.ConfigKind != "" {
		fmt.Fprintf(w, "  Config:     %s\n", o.ConfigKind)
	}
	if o.FilterID != "" {
		fmt.Fprintf(w, "  Filter ID:  %s\n", o.FilterID)
	}
	if o.Class != "" {
		fmt.Fprintf(w, "  Class:      %s\n", o.Class)
	}
	if o.Version != "" {
		fmt.Fprintf(w, "  Version:    %s\n", o.Version)
	}
	if o.Source != "" {
		fmt.Fprintf(w, "  Source:     %s\n", o.Source)
	}

	read := "no"
	write := "no"
	if o.HasReader {
		read = "yes"
	}
	if o.HasWriter {
		write = "yes"
	}
	fmt.Fprintf(w, "  Reader:     %s\n", read)
	fmt.Fprintf(w, "  Writer:     %s\n", write)
	edit := "no"
	if o.Editable {
		edit = "yes"
		if o.RoundTrip {
			edit = "yes (faithful round-trip)"
		}
	}
	fmt.Fprintf(w, "  Editable:   %s\n", edit)

	if len(o.Extensions) > 0 {
		fmt.Fprintf(w, "  Extensions: %s\n", strings.Join(o.Extensions, ", "))
	}
	if len(o.MimeTypes) > 0 {
		fmt.Fprintf(w, "  MIME types: %s\n", strings.Join(o.MimeTypes, ", "))
	}

	if len(o.Groups) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Parameters:")
		for _, g := range o.Groups {
			fmt.Fprintln(w)
			label := "[" + g.Label + "]"
			if g.Description != "" {
				label += ": " + g.Description
			}
			Title(w, label)

			t := NewTable(w).Accent(0).Headers(
				T("formats.header.parameter"), T("formats.header.type"),
				T("formats.header.default"), T("formats.header.description"))
			s := t.Styles()
			for _, p := range g.Parameters {
				def := ""
				if p.Default != nil {
					def = fmt.Sprintf("%v", p.Default)
				}
				t.Row(p.Name, s.Dim(p.Type), s.Dim(def), s.Text(p.Description))
			}
			t.Render()
		}
	}

	return nil
}

// TermsImportOutput represents the result of a terms store import.
type TermsImportOutput struct {
	Imported int    `json:"imported"`
	DBPath   string `json:"db_path"`
	Total    int    `json:"total"`
}

func (o TermsImportOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Imported %d concepts into %s (total: %d)\n", o.Imported, o.DBPath, o.Total)
	return nil
}

// TermsExportOutput represents the result of a terms store export.
type TermsExportOutput struct {
	Count      int    `json:"count"`
	OutputPath string `json:"output_path"`
}

func (o TermsExportOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Exported %d concepts to %s\n", o.Count, o.OutputPath)
	return nil
}

// TermsLookupTarget represents a target term in a lookup result.
type TermsLookupTarget struct {
	Text   string `json:"text"`
	Locale string `json:"locale"`
	Status string `json:"status"`
}

// TermsLookupEntry represents a single match from a terms store lookup.
type TermsLookupEntry struct {
	Term      string              `json:"term"`
	Locale    string              `json:"locale"`
	Status    string              `json:"status"`
	MatchType string              `json:"match_type"`
	Score     float64             `json:"score"`
	ConceptID string              `json:"concept_id"`
	Domain    string              `json:"domain,omitempty"`
	Targets   []TermsLookupTarget `json:"targets,omitempty"`
}

// TermsLookupOutput represents the result of a terms store lookup.
type TermsLookupOutput struct {
	Matches []TermsLookupEntry `json:"matches"`
	Total   int                `json:"total"`
}

func (o TermsLookupOutput) FormatText(w io.Writer) error {
	if len(o.Matches) == 0 {
		fmt.Fprintln(w, "No matches found.")
		return nil
	}

	t := NewTable(w).Accent(0).Headers(
		T("terms.header.term"), T("terms.header.locale"), T("terms.header.status"),
		T("terms.header.match"), T("terms.header.score"), T("terms.header.concept"),
		T("terms.header.domain"))
	s := t.Styles()
	for _, m := range o.Matches {
		t.Row(m.Term, m.Locale, s.Dim(m.Status), s.Dim(m.MatchType),
			fmt.Sprintf("%.2f", m.Score), s.Dim(m.ConceptID), s.Dim(m.Domain))
		// Targets hang off their source term rather than standing as rows of
		// their own, so they are indented into the term column.
		for _, tgt := range m.Targets {
			t.Row("  → "+tgt.Text, tgt.Locale, s.Dim(tgt.Status), "", "", "", "")
		}
	}
	t.Render()

	fmt.Fprintln(w)
	Note(w, "Total: %d match(es)", o.Total)
	return nil
}

// TermsSearchTerm represents a term within a concept search result.
type TermsSearchTerm struct {
	Text   string `json:"text"`
	Locale string `json:"locale"`
}

// TermsSearchEntry represents a single concept from a terms store search.
type TermsSearchEntry struct {
	ID         string            `json:"id"`
	Domain     string            `json:"domain,omitempty"`
	Definition string            `json:"definition,omitempty"`
	Terms      []TermsSearchTerm `json:"terms"`
}

// TermsSearchOutput represents the result of a terms store search.
type TermsSearchOutput struct {
	Concepts []TermsSearchEntry `json:"concepts"`
	Total    int                `json:"total"`
	Shown    int                `json:"shown"`
}

func (o TermsSearchOutput) FormatText(w io.Writer) error {
	if len(o.Concepts) == 0 {
		fmt.Fprintln(w, "No concepts found.")
		return nil
	}

	t := NewTable(w).Accent(0).Headers(
		T("terms.header.concept"), T("terms.header.domain"),
		T("terms.header.terms"), T("terms.header.definition"))
	s := t.Styles()
	for _, c := range o.Concepts {
		terms := make([]string, 0, len(c.Terms))
		for _, term := range c.Terms {
			terms = append(terms, fmt.Sprintf("%s [%s]", term.Text, term.Locale))
		}
		t.Row(c.ID, s.Dim(c.Domain), strings.Join(terms, ", "), s.Text(c.Definition))
	}
	t.Render()

	if o.Total > o.Shown {
		fmt.Fprintln(w)
		Note(w, "Showing %d of %d results. Use --limit to see more.", o.Shown, o.Total)
	}
	return nil
}

// TermsStatsOutput represents the result of terms stats.
type TermsStatsOutput struct {
	DBPath   string         `json:"db_path"`
	Concepts int            `json:"concepts"`
	Terms    int            `json:"terms"`
	Locales  map[string]int `json:"locales,omitempty"`
	Domains  map[string]int `json:"domains,omitempty"`
	Statuses map[string]int `json:"statuses,omitempty"`
}

func (o TermsStatsOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Terms: %s\n", o.DBPath)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Concepts:  %d\n", o.Concepts)
	fmt.Fprintf(w, "  Terms:     %d\n", o.Terms)

	if len(o.Locales) > 0 {
		fmt.Fprintln(w)
		Title(w, "Locales:")
		t := NewTable(w).Accent(0).
			Headers(T("terms.header.locale"), T("terms.header.terms"))
		for _, k := range SortedKeys(o.Locales) {
			t.Rowf(k, o.Locales[k])
		}
		t.Render()
	}

	if len(o.Domains) > 0 {
		fmt.Fprintln(w)
		Title(w, "Domains:")
		t := NewTable(w).Accent(0).
			Headers(T("terms.header.domain"), T("terms.header.concepts"))
		for _, k := range SortedKeys(o.Domains) {
			t.Rowf(k, o.Domains[k])
		}
		t.Render()
	}

	if len(o.Statuses) > 0 {
		fmt.Fprintln(w)
		Title(w, "Term statuses:")
		t := NewTable(w).Accent(0).
			Headers(T("terms.header.status"), T("terms.header.terms"))
		for _, k := range SortedKeys(o.Statuses) {
			t.Rowf(k, o.Statuses[k])
		}
		t.Render()
	}

	return nil
}

// --- content memory output types ---

// MemoryImportOutput represents the result of a content-memory import.
type MemoryImportOutput struct {
	Imported int    `json:"imported"`
	DBPath   string `json:"db_path"`
	Total    int    `json:"total"`
}

func (o MemoryImportOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Imported %d entries into %s (total: %d)\n", o.Imported, o.DBPath, o.Total)
	return nil
}

// MemoryExportOutput represents the result of a content-memory export.
type MemoryExportOutput struct {
	Count      int    `json:"count"`
	OutputPath string `json:"output_path"`
}

func (o MemoryExportOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Exported %d entries to %s\n", o.Count, o.OutputPath)
	return nil
}

// MemoryLookupEntry represents a single content-memory match.
type MemoryLookupEntry struct {
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Score     float64 `json:"score"`
	MatchType string  `json:"match_type"`
	EntryID   string  `json:"entry_id"`
}

// MemoryLookupOutput represents the result of a content-memory lookup.
type MemoryLookupOutput struct {
	Matches []MemoryLookupEntry `json:"matches"`
	Total   int                 `json:"total"`
}

func (o MemoryLookupOutput) FormatText(w io.Writer) error {
	if len(o.Matches) == 0 {
		fmt.Fprintln(w, "No matches found.")
		return nil
	}

	t := NewTable(w).Accent(0).Headers(
		T("tm.header.source"), T("tm.header.target"),
		T("tm.header.score"), T("tm.header.matchType"))
	s := t.Styles()
	for _, m := range o.Matches {
		t.Row(m.Source, m.Target, fmt.Sprintf("%.0f%%", m.Score*100), s.Dim(m.MatchType))
	}
	t.Render()

	fmt.Fprintln(w)
	Note(w, "Total: %d match(es)", o.Total)
	return nil
}

// MemorySearchEntry represents a single content-memory entry in search results.
type MemorySearchEntry struct {
	ID             string `json:"id"`
	Source         string `json:"source"`
	Target         string `json:"target"`
	SourceLanguage string `json:"source_language"`
	TargetLanguage string `json:"target_language"`
}

// MemorySearchOutput represents the result of a content-memory search.
type MemorySearchOutput struct {
	Entries []MemorySearchEntry `json:"entries"`
	Total   int                 `json:"total"`
	Shown   int                 `json:"shown"`
}

func (o MemorySearchOutput) FormatText(w io.Writer) error {
	if len(o.Entries) == 0 {
		fmt.Fprintln(w, "No entries found.")
		return nil
	}

	t := NewTable(w).Accent(0).Headers(
		T("tm.header.source"), T("tm.header.target"), T("tm.header.locales"))
	s := t.Styles()
	for _, e := range o.Entries {
		t.Row(e.Source, e.Target, s.Dim(e.SourceLanguage+" → "+e.TargetLanguage))
	}
	t.Render()

	if o.Total > o.Shown {
		fmt.Fprintln(w)
		Note(w, "Showing %d of %d results. Use --limit to see more.", o.Shown, o.Total)
	}
	return nil
}

// MemoryStatsOutput represents the result of content-memory stats.
type MemoryStatsOutput struct {
	DBPath      string         `json:"db_path"`
	Entries     int            `json:"entries"`
	LocalePairs map[string]int `json:"locale_pairs,omitempty"`
}

func (o MemoryStatsOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Content Memory: %s\n", o.DBPath)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Entries: %d\n", o.Entries)

	if len(o.LocalePairs) > 0 {
		fmt.Fprintln(w)
		Title(w, "Locale pairs:")
		t := NewTable(w).Accent(0).
			Headers(T("tm.header.localePair"), T("tm.header.entries"))
		for _, k := range SortedKeys(o.LocalePairs) {
			t.Rowf(k, o.LocalePairs[k])
		}
		t.Render()
	}

	return nil
}

// --- Resource list output types ---

// ResourceListEntry represents a named resource (terms or content memory) in KAPI_HOME.
type ResourceListEntry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// ResourceListOutput represents the list of named resources.
type ResourceListOutput struct {
	Kind      string              `json:"kind"`
	Resources []ResourceListEntry `json:"resources"`
	Total     int                 `json:"total"`
}

func (o ResourceListOutput) FormatText(w io.Writer) error {
	if len(o.Resources) == 0 {
		fmt.Fprintf(w, "No named %ss found.\n", o.Kind)
		return nil
	}

	Title(w, fmt.Sprintf("Named %ss:", o.Kind))

	t := NewTable(w).Accent(0).Headers(
		T("resources.header.name"), T("resources.header.size"),
		T("resources.header.modified"), T("resources.header.path"))
	s := t.Styles()
	for _, r := range o.Resources {
		t.Row(r.Name, formatBytes(r.Size),
			s.Dim(r.Modified.Format("2006-01-02 15:04")), s.Dim(r.Path))
	}
	t.Render()

	fmt.Fprintln(w)
	Note(w, "Total: %d %s(s)", o.Total, o.Kind)
	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// SortedKeys returns the keys of a map[string]int sorted alphabetically.
func SortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
