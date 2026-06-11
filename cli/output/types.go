package output

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"time"
)

// VersionOutput represents version information.
type VersionOutput struct {
	Program   string `json:"program"`
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
}

func (v VersionOutput) FormatText(w io.Writer) error {
	if v.Commit != "" && v.BuildDate != "" {
		fmt.Fprintf(w, "%s %s (commit: %s, built: %s)\n", v.Program, v.Version, v.Commit, v.BuildDate)
	} else {
		fmt.Fprintf(w, "%s %s\n", v.Program, v.Version)
	}
	return nil
}

// FormatInfo represents a single format entry.
type FormatInfo struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	HasReader   bool     `json:"has_reader"`
	HasWriter   bool     `json:"has_writer"`
	Source      string   `json:"source,omitempty"`
	Extensions  []string `json:"extensions,omitempty"`
	MimeTypes   []string `json:"mime_types,omitempty"`
}

// FormatsListOutput represents the list of formats.
type FormatsListOutput struct {
	Formats []FormatInfo `json:"formats"`
	Total   int          `json:"total"`
}

func (f FormatsListOutput) FormatText(w io.Writer) error {
	fmt.Fprintln(w, T("formats.available"))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %-26s %-30s %-6s %-6s %-12s %-24s %s\n",
		T("formats.header.format"), T("formats.header.name"), T("formats.header.read"),
		T("formats.header.write"), T("formats.header.source"),
		T("formats.header.extensions"), T("formats.header.mimeTypes"))
	fmt.Fprintf(w, "  %-26s %-30s %-6s %-6s %-12s %-24s %s\n",
		"------", "----", "----", "-----", "------", "----------", "----------")

	for _, info := range f.Formats {
		read := "-"
		write := "-"
		if info.HasReader {
			read = "yes"
		}
		if info.HasWriter {
			write = "yes"
		}
		displayName := info.DisplayName
		if len(displayName) > 28 {
			displayName = displayName[:25] + "..."
		}
		exts := strings.Join(info.Extensions, ", ")
		if len(exts) > 22 {
			exts = exts[:19] + "..."
		}
		mimes := strings.Join(info.MimeTypes, ", ")
		if len(mimes) > 44 {
			mimes = mimes[:41] + "..."
		}
		fmt.Fprintf(w, "  %-26s %-30s %-6s %-6s %-12s %-24s %s\n",
			info.Name, displayName, read, write, info.Source, exts, mimes)
	}
	fmt.Fprintf(w, "\n"+T("formats.total")+"\n", f.Total)
	return nil
}

// PluginInfo represents a single plugin entry.
type PluginInfo struct {
	Name             string `json:"name"`
	Version          string `json:"version,omitempty"`
	FrameworkVersion string `json:"framework_version,omitempty"`
	PluginType       string `json:"plugin_type,omitempty"`
	Status           string `json:"status"`
	Formats          int    `json:"formats,omitempty"`
	Path             string `json:"path,omitempty"`
}

// PluginsListOutput represents the list of plugins.
type PluginsListOutput struct {
	Plugins []PluginInfo `json:"plugins"`
	Total   int          `json:"total"`
}

func (p PluginsListOutput) FormatText(w io.Writer) error {
	if len(p.Plugins) == 0 {
		fmt.Fprintln(w, T("plugins.none"))
		return nil
	}

	fmt.Fprintln(w, T("plugins.installed"))
	fmt.Fprintln(w)

	// Compute dynamic column width for version.
	versionWidth := len("VERSION")
	for _, plugin := range p.Plugins {
		v := plugin.Version
		if plugin.FrameworkVersion != "" {
			v += " (" + plugin.FrameworkVersion + ")"
		}
		if len(v) > versionWidth {
			versionWidth = len(v)
		}
	}
	versionWidth += 2 // padding

	hdrFmt := fmt.Sprintf("  %%-20s %%-%ds %%-10s %%-10s %%s\n", versionWidth)
	rowFmt := fmt.Sprintf("  %%-20s %%-%ds %%-10s %%-10s %%d\n", versionWidth)
	fmt.Fprintf(w, hdrFmt, T("plugins.header.name"), T("plugins.header.version"),
		T("plugins.header.type"), T("plugins.header.status"), T("plugins.header.formats"))
	fmt.Fprintf(w, hdrFmt, "----", "-------", "----", "------", "-------")

	for _, plugin := range p.Plugins {
		pluginType := plugin.PluginType
		if pluginType == "" {
			pluginType = "-"
		}
		version := plugin.Version
		if plugin.FrameworkVersion != "" {
			version += " (" + plugin.FrameworkVersion + ")"
		}
		fmt.Fprintf(w, rowFmt, plugin.Name, version, pluginType, plugin.Status, plugin.Formats)
	}
	fmt.Fprintf(w, "\n"+T("plugins.total")+"\n", p.Total)
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

	fmt.Fprintln(w, T("tools.available"))

	for _, cat := range categoryOrder {
		tools := grouped[cat]
		if len(tools) == 0 {
			continue
		}
		title := T("tools.category." + cat)
		fmt.Fprintf(w, "\n%s:\n", title)
		for _, tool := range tools {
			fmt.Fprintf(w, "  %-24s %s\n", tool.Name, tool.Description)
		}
	}

	// Print any tools in unknown categories.
	for cat, tools := range grouped {
		if categoryTitles[cat] != "" {
			continue
		}
		fmt.Fprintf(w, "\n%s:\n", T("tools.category.other"))
		for _, tool := range tools {
			fmt.Fprintf(w, "  %-24s %s\n", tool.Name, tool.Description)
		}
	}

	fmt.Fprintf(w, "\n"+T("tools.total")+"\n", t.Total)
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

	fmt.Fprintln(w, T("flows.available"))
	fmt.Fprintln(w)
	for _, flow := range f.Flows {
		fmt.Fprintf(w, "  %s", flow.Name)
		if flow.Description != "" {
			fmt.Fprintf(w, " - %s", flow.Description)
		}
		if flow.Steps > 0 {
			fmt.Fprintf(w, " (%d steps)", flow.Steps)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "\n"+T("flows.total")+"\n", f.Total)
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
}

func (o FlowRunOutput) FormatText(w io.Writer) error {
	if o.FilesProcessed > 0 {
		fmt.Fprintf(w, "Flow %s completed: processed %d files\n", o.FlowName, o.FilesProcessed)
	} else {
		fmt.Fprintf(w, "Flow %s completed: %s → %s\n", o.FlowName, o.InputPath, o.OutputPath)
	}
	if o.Stats != nil {
		fmt.Fprintf(w, "  Parts: %d, Blocks: %d\n", o.Stats.PartCount, o.Stats.BlockCount)
	}
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
		fmt.Fprintln(w, "Framework Presets:")
		for _, p := range fwPresets {
			fmt.Fprintf(w, "  %-20s %s [%s]\n", p.Name, p.Description, p.Source)
		}
		fmt.Fprintln(w)
	}

	if len(fmtPresets) > 0 {
		fmt.Fprintln(w, "Format Presets:")

		// Filter out default presets — they're just the format's built-in config.
		var nonDefault []PresetEntry
		for _, p := range fmtPresets {
			if !p.IsDefault {
				nonDefault = append(nonDefault, p)
			}
		}

		// Group by format for readability.
		var currentFormat string
		first := true
		for _, p := range nonDefault {
			if p.Format != currentFormat {
				currentFormat = p.Format
				if !first {
					fmt.Fprintln(w)
				}
				first = false
				fmt.Fprintf(w, "  %s\n", currentFormat)
			}
			presetName := p.Name
			if idx := strings.Index(presetName, ":"); idx >= 0 {
				presetName = presetName[idx:]
			}
			fmt.Fprintf(w, "    %-30s %s\n", presetName, p.Description)
		}
	}

	fmt.Fprintf(w, "\nTotal: %d preset(s)\n", p.Total)
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
	PluginType  string `json:"plugin_type"`
	Description string `json:"description,omitempty"`
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

	fmt.Fprintf(w, "  %-25s %-10s %-10s %s\n", "NAME", "VERSION", "TYPE", "DESCRIPTION")
	fmt.Fprintf(w, "  %-25s %-10s %-10s %s\n", "----", "-------", "----", "-----------")
	for _, p := range o.Plugins {
		fmt.Fprintf(w, "  %-25s %-10s %-10s %s\n", p.Name, p.Version, p.PluginType, p.Description)
	}
	fmt.Fprintf(w, "\nTotal: %d plugin(s)\n", o.Total)
	return nil
}

// PluginInstallOutput represents the result of a plugin install.
type PluginInstallOutput struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	InstallType string   `json:"install_type"`
	Files       []string `json:"files,omitempty"`
}

func (o PluginInstallOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Installed %s v%s (%s)\n", o.Name, o.Version, o.InstallType)
	for _, f := range o.Files {
		fmt.Fprintf(w, "  → %s\n", f)
	}
	return nil
}

// PluginUpdateEntry represents a single plugin that was updated.
type PluginUpdateEntry struct {
	Name       string `json:"name"`
	OldVersion string `json:"old_version,omitempty"`
	NewVersion string `json:"new_version"`
}

// PluginUpdateOutput represents the result of a plugin update.
type PluginUpdateOutput struct {
	Updated  []PluginUpdateEntry `json:"updated"`
	UpToDate bool                `json:"up_to_date"`
}

func (o PluginUpdateOutput) FormatText(w io.Writer) error {
	if o.UpToDate {
		fmt.Fprintln(w, "All plugins are up to date.")
		return nil
	}
	for _, u := range o.Updated {
		fmt.Fprintf(w, "Updated %s to v%s\n", u.Name, u.NewVersion)
	}
	return nil
}

// PluginRemoveOutput represents the result of a plugin remove.
type PluginRemoveOutput struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

func (o PluginRemoveOutput) FormatText(w io.Writer) error {
	if o.Version != "" {
		fmt.Fprintf(w, "Removed %s@%s\n", o.Name, o.Version)
	} else {
		fmt.Fprintf(w, "Removed %s\n", o.Name)
	}
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
			fmt.Fprintf(w, "  [%s]", g.Label)
			if g.Description != "" {
				fmt.Fprintf(w, " — %s", g.Description)
			}
			fmt.Fprintln(w)

			for _, p := range g.Parameters {
				fmt.Fprintf(w, "    %-20s %-10s", p.Name, p.Type)
				if p.Default != nil {
					fmt.Fprintf(w, " (default: %v)", p.Default)
				}
				if p.Description != "" {
					fmt.Fprintf(w, "  %s", p.Description)
				}
				fmt.Fprintln(w)
			}
		}
	}

	return nil
}

// TermbaseImportOutput represents the result of a termbase import.
type TermbaseImportOutput struct {
	Imported int    `json:"imported"`
	DBPath   string `json:"db_path"`
	Total    int    `json:"total"`
}

func (o TermbaseImportOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Imported %d concepts into %s (total: %d)\n", o.Imported, o.DBPath, o.Total)
	return nil
}

// TermbaseExportOutput represents the result of a termbase export.
type TermbaseExportOutput struct {
	Count      int    `json:"count"`
	OutputPath string `json:"output_path"`
}

func (o TermbaseExportOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Exported %d concepts to %s\n", o.Count, o.OutputPath)
	return nil
}

// TermbaseLookupTarget represents a target term in a lookup result.
type TermbaseLookupTarget struct {
	Text   string `json:"text"`
	Locale string `json:"locale"`
	Status string `json:"status"`
}

// TermbaseLookupEntry represents a single match from a termbase lookup.
type TermbaseLookupEntry struct {
	Term      string                 `json:"term"`
	Locale    string                 `json:"locale"`
	Status    string                 `json:"status"`
	MatchType string                 `json:"match_type"`
	Score     float64                `json:"score"`
	ConceptID string                 `json:"concept_id"`
	Domain    string                 `json:"domain,omitempty"`
	Targets   []TermbaseLookupTarget `json:"targets,omitempty"`
}

// TermbaseLookupOutput represents the result of a termbase lookup.
type TermbaseLookupOutput struct {
	Matches []TermbaseLookupEntry `json:"matches"`
	Total   int                   `json:"total"`
}

func (o TermbaseLookupOutput) FormatText(w io.Writer) error {
	if len(o.Matches) == 0 {
		fmt.Fprintln(w, "No matches found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  TERM\tLOCALE\tSTATUS\tMATCH\tSCORE\tCONCEPT\tDOMAIN\n")
	fmt.Fprintf(tw, "  ----\t------\t------\t-----\t-----\t-------\t------\n")
	for _, m := range o.Matches {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%.2f\t%s\t%s\n",
			m.Term, m.Locale, m.Status, m.MatchType, m.Score, m.ConceptID, m.Domain)
		for _, t := range m.Targets {
			fmt.Fprintf(tw, "    -> %s\t%s\t%s\t\t\t\t\n", t.Text, t.Locale, t.Status)
		}
	}
	tw.Flush()
	fmt.Fprintf(w, "\nTotal: %d match(es)\n", o.Total)
	return nil
}

// TermbaseSearchTerm represents a term within a concept search result.
type TermbaseSearchTerm struct {
	Text   string `json:"text"`
	Locale string `json:"locale"`
}

// TermbaseSearchEntry represents a single concept from a termbase search.
type TermbaseSearchEntry struct {
	ID         string               `json:"id"`
	Domain     string               `json:"domain,omitempty"`
	Definition string               `json:"definition,omitempty"`
	Terms      []TermbaseSearchTerm `json:"terms"`
}

// TermbaseSearchOutput represents the result of a termbase search.
type TermbaseSearchOutput struct {
	Concepts []TermbaseSearchEntry `json:"concepts"`
	Total    int                   `json:"total"`
	Shown    int                   `json:"shown"`
}

func (o TermbaseSearchOutput) FormatText(w io.Writer) error {
	if len(o.Concepts) == 0 {
		fmt.Fprintln(w, "No concepts found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  CONCEPT\tDOMAIN\tTERMS\tDEFINITION\n")
	fmt.Fprintf(tw, "  -------\t------\t-----\t----------\n")
	for _, c := range o.Concepts {
		var terms []string
		for _, t := range c.Terms {
			terms = append(terms, fmt.Sprintf("%s [%s]", t.Text, t.Locale))
		}
		def := c.Definition
		if len(def) > 40 {
			def = def[:37] + "..."
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", c.ID, c.Domain, strings.Join(terms, ", "), def)
	}
	tw.Flush()

	if o.Total > o.Shown {
		fmt.Fprintf(w, "\nShowing %d of %d results. Use --limit to see more.\n", o.Shown, o.Total)
	}
	return nil
}

// TermbaseStatsOutput represents the result of termbase stats.
type TermbaseStatsOutput struct {
	DBPath   string         `json:"db_path"`
	Concepts int            `json:"concepts"`
	Terms    int            `json:"terms"`
	Locales  map[string]int `json:"locales,omitempty"`
	Domains  map[string]int `json:"domains,omitempty"`
	Statuses map[string]int `json:"statuses,omitempty"`
}

func (o TermbaseStatsOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Termbase: %s\n", o.DBPath)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Concepts:  %d\n", o.Concepts)
	fmt.Fprintf(w, "  Terms:     %d\n", o.Terms)

	if len(o.Locales) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Locales:")
		keys := SortedKeys(o.Locales)
		for _, k := range keys {
			fmt.Fprintf(w, "    %-10s %d terms\n", k, o.Locales[k])
		}
	}

	if len(o.Domains) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Domains:")
		keys := SortedKeys(o.Domains)
		for _, k := range keys {
			fmt.Fprintf(w, "    %-20s %d concepts\n", k, o.Domains[k])
		}
	}

	if len(o.Statuses) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Term statuses:")
		keys := SortedKeys(o.Statuses)
		for _, k := range keys {
			fmt.Fprintf(w, "    %-12s %d\n", k, o.Statuses[k])
		}
	}

	return nil
}

// --- TM output types ---

// TMImportOutput represents the result of a TM import.
type TMImportOutput struct {
	Imported int    `json:"imported"`
	DBPath   string `json:"db_path"`
	Total    int    `json:"total"`
}

func (o TMImportOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Imported %d entries into %s (total: %d)\n", o.Imported, o.DBPath, o.Total)
	return nil
}

// TMExportOutput represents the result of a TM export.
type TMExportOutput struct {
	Count      int    `json:"count"`
	OutputPath string `json:"output_path"`
}

func (o TMExportOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Exported %d entries to %s\n", o.Count, o.OutputPath)
	return nil
}

// TMLookupEntry represents a single TM match.
type TMLookupEntry struct {
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Score     float64 `json:"score"`
	MatchType string  `json:"match_type"`
	EntryID   string  `json:"entry_id"`
}

// TMLookupOutput represents the result of a TM lookup.
type TMLookupOutput struct {
	Matches []TMLookupEntry `json:"matches"`
	Total   int             `json:"total"`
}

func (o TMLookupOutput) FormatText(w io.Writer) error {
	if len(o.Matches) == 0 {
		fmt.Fprintln(w, "No matches found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  SOURCE\tTARGET\tSCORE\tMATCH TYPE\n")
	fmt.Fprintf(tw, "  ------\t------\t-----\t----------\n")
	for _, m := range o.Matches {
		source := m.Source
		if len(source) > 40 {
			source = source[:37] + "..."
		}
		target := m.Target
		if len(target) > 40 {
			target = target[:37] + "..."
		}
		fmt.Fprintf(tw, "  %s\t%s\t%.0f%%\t%s\n", source, target, m.Score*100, m.MatchType)
	}
	tw.Flush()
	fmt.Fprintf(w, "\nTotal: %d match(es)\n", o.Total)
	return nil
}

// TMSearchEntry represents a single TM entry in search results.
type TMSearchEntry struct {
	ID             string `json:"id"`
	Source         string `json:"source"`
	Target         string `json:"target"`
	SourceLanguage string `json:"source_language"`
	TargetLanguage string `json:"target_language"`
}

// TMSearchOutput represents the result of a TM search.
type TMSearchOutput struct {
	Entries []TMSearchEntry `json:"entries"`
	Total   int             `json:"total"`
	Shown   int             `json:"shown"`
}

func (o TMSearchOutput) FormatText(w io.Writer) error {
	if len(o.Entries) == 0 {
		fmt.Fprintln(w, "No entries found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  SOURCE\tTARGET\tLOCALES\n")
	fmt.Fprintf(tw, "  ------\t------\t-------\n")
	for _, e := range o.Entries {
		source := e.Source
		if len(source) > 40 {
			source = source[:37] + "..."
		}
		target := e.Target
		if len(target) > 40 {
			target = target[:37] + "..."
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s → %s\n", source, target, e.SourceLanguage, e.TargetLanguage)
	}
	tw.Flush()

	if o.Total > o.Shown {
		fmt.Fprintf(w, "\nShowing %d of %d results. Use --limit to see more.\n", o.Shown, o.Total)
	}
	return nil
}

// TMStatsOutput represents the result of TM stats.
type TMStatsOutput struct {
	DBPath      string         `json:"db_path"`
	Entries     int            `json:"entries"`
	LocalePairs map[string]int `json:"locale_pairs,omitempty"`
}

func (o TMStatsOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Translation Memory: %s\n", o.DBPath)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Entries: %d\n", o.Entries)

	if len(o.LocalePairs) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Locale pairs:")
		keys := SortedKeys(o.LocalePairs)
		for _, k := range keys {
			fmt.Fprintf(w, "    %-20s %d entries\n", k, o.LocalePairs[k])
		}
	}

	return nil
}

// --- Resource list output types ---

// ResourceListEntry represents a named resource (termbase or TM) in KAPI_HOME.
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

	fmt.Fprintf(w, "Named %ss:\n\n", o.Kind)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  NAME\tSIZE\tMODIFIED\tPATH\n")
	fmt.Fprintf(tw, "  ----\t----\t--------\t----\n")
	for _, r := range o.Resources {
		sizeStr := formatBytes(r.Size)
		modStr := r.Modified.Format("2006-01-02 15:04")
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", r.Name, sizeStr, modStr, r.Path)
	}
	tw.Flush()
	fmt.Fprintf(w, "\nTotal: %d %s(s)\n", o.Total, o.Kind)
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
