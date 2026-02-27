package output

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// AddEntry represents a single pattern added by brain add.
type AddEntry struct {
	Pattern string `json:"pattern"`
	Format  string `json:"format,omitempty"`
	Files   int    `json:"files"`
	Skipped bool   `json:"skipped,omitempty"`
}

// AddOutput represents the result of brain add.
type AddOutput struct {
	Added []AddEntry `json:"added"`
}

func (o AddOutput) FormatText(w io.Writer) error {
	for _, e := range o.Added {
		if e.Skipped {
			fmt.Fprintf(w, "Already tracked: %s\n", e.Pattern)
			continue
		}
		if e.Format != "" {
			fmt.Fprintf(w, "Added %s (%s) — %d file(s)\n", e.Pattern, e.Format, e.Files)
		} else {
			fmt.Fprintf(w, "Added %s — %d file(s)\n", e.Pattern, e.Files)
		}
	}
	return nil
}

// RmEntry represents a single pattern processed by brain rm.
type RmEntry struct {
	Pattern string `json:"pattern"`
	Action  string `json:"action"`           // "removed", "excluded", "already_excluded"
	Format  string `json:"format,omitempty"` // only for "removed"
	Files   int    `json:"files,omitempty"`  // only for "excluded"
}

// RmOutput represents the result of brain rm.
type RmOutput struct {
	Entries []RmEntry `json:"entries"`
}

func (o RmOutput) FormatText(w io.Writer) error {
	for _, e := range o.Entries {
		switch e.Action {
		case "removed":
			if e.Format != "" {
				fmt.Fprintf(w, "Removed %s (was: %s)\n", e.Pattern, e.Format)
			} else {
				fmt.Fprintf(w, "Removed %s\n", e.Pattern)
			}
		case "excluded":
			fmt.Fprintf(w, "Excluded %s — %d file(s) now excluded\n", e.Pattern, e.Files)
		case "already_excluded":
			fmt.Fprintf(w, "Already excluded: %s\n", e.Pattern)
		}
	}
	return nil
}

// VersionOutput represents version information.
type VersionOutput struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
}

func (v VersionOutput) FormatText(w io.Writer) error {
	if v.Commit != "" && v.BuildDate != "" {
		fmt.Fprintf(w, "brain %s (commit: %s, built: %s)\n", v.Version, v.Commit, v.BuildDate)
	} else {
		fmt.Fprintf(w, "brain %s\n", v.Version)
	}
	return nil
}

// StatusOutput represents sync status.
type StatusOutput struct {
	Project     ProjectInfo `json:"project"`
	ItemCount   int         `json:"item_count"`
	FileCount   int         `json:"file_count"`
	WordCount   int         `json:"word_count"`
	PendingPush int         `json:"pending_push"`
	PendingPull int         `json:"pending_pull"`
	LastSync    *time.Time  `json:"last_sync,omitempty"`
	Errors      []string    `json:"errors,omitempty"`
	UpToDate    bool        `json:"up_to_date"`
}

type ProjectInfo struct {
	Root      string `json:"root"`
	ConfigDir string `json:"config_dir"`
	Server    string `json:"server,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

func (s StatusOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Project root: %s\n", s.Project.Root)
	fmt.Fprintf(w, "Config:       %s\n", s.Project.ConfigDir)

	if s.Project.Server == "" {
		fmt.Fprintln(w, "\nNot connected to a server.")
		fmt.Fprintln(w, "  Run 'brain init' with a server or set one in .brain/config.yaml")
		return nil
	}

	fmt.Fprintf(w, "\nLocal: %d files, %d blocks (%d words)\n", s.FileCount, s.ItemCount, s.WordCount)

	if s.PendingPush > 0 {
		fmt.Fprintf(w, "Pending push: %d blocks changed locally\n", s.PendingPush)
	}
	if s.PendingPull > 0 {
		fmt.Fprintf(w, "Pending pull: %d remote changes available\n", s.PendingPull)
	} else if s.PendingPull < 0 {
		fmt.Fprintln(w, "Pending pull: remote changes available")
	}
	if s.UpToDate {
		fmt.Fprintln(w, "Up to date.")
	}

	if s.LastSync != nil && !s.LastSync.IsZero() {
		fmt.Fprintf(w, "Last sync:    %s\n", s.LastSync.Format("2006-01-02 15:04:05 UTC"))
	}

	if len(s.Errors) > 0 {
		fmt.Fprintln(w, "\nErrors:")
		for _, e := range s.Errors {
			fmt.Fprintf(w, "  - %s\n", e)
		}
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
	fmt.Fprintln(w, "Available formats:")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %-20s %-22s %-6s %-6s %-12s %-20s %s\n",
		"FORMAT", "DISPLAY NAME", "READ", "WRITE", "SOURCE", "EXTENSIONS", "MIME TYPES")
	fmt.Fprintf(w, "  %-20s %-22s %-6s %-6s %-12s %-20s %s\n",
		"------", "------------", "----", "-----", "------", "----------", "----------")

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
		if len(displayName) > 20 {
			displayName = displayName[:17] + "..."
		}
		exts := strings.Join(info.Extensions, ", ")
		if len(exts) > 18 {
			exts = exts[:15] + "..."
		}
		mimes := strings.Join(info.MimeTypes, ", ")
		if len(mimes) > 40 {
			mimes = mimes[:37] + "..."
		}
		fmt.Fprintf(w, "  %-20s %-22s %-6s %-6s %-12s %-20s %s\n",
			info.Name, displayName, read, write, info.Source, exts, mimes)
	}
	fmt.Fprintf(w, "\nTotal: %d format(s)\n", f.Total)
	return nil
}

// PluginInfo represents a single plugin entry.
type PluginInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	PluginType string `json:"plugin_type,omitempty"` // "bundle", "format", "tool", etc.
	Status     string `json:"status"`
	Formats    int    `json:"formats,omitempty"`
	Path       string `json:"path,omitempty"`
}

// PluginsListOutput represents the list of plugins.
type PluginsListOutput struct {
	Plugins []PluginInfo `json:"plugins"`
	Total   int          `json:"total"`
}

func (p PluginsListOutput) FormatText(w io.Writer) error {
	if len(p.Plugins) == 0 {
		fmt.Fprintln(w, "No plugins installed.")
		return nil
	}

	fmt.Fprintln(w, "Installed plugins:")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %-20s %-10s %-10s %-10s %-8s %s\n",
		"NAME", "VERSION", "TYPE", "STATUS", "FORMATS", "PATH")
	fmt.Fprintf(w, "  %-20s %-10s %-10s %-10s %-8s %s\n",
		"----", "-------", "----", "------", "-------", "----")

	for _, plugin := range p.Plugins {
		pluginType := plugin.PluginType
		if pluginType == "" {
			pluginType = "-"
		}
		fmt.Fprintf(w, "  %-20s %-10s %-10s %-10s %-8d %s\n",
			plugin.Name, plugin.Version, pluginType, plugin.Status, plugin.Formats, plugin.Path)
	}
	fmt.Fprintf(w, "\nTotal: %d plugin(s)\n", p.Total)
	return nil
}

// ToolInfo represents a single tool entry.
type ToolInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
}

// ToolsListOutput represents the list of tools.
type ToolsListOutput struct {
	Tools []ToolInfo `json:"tools"`
	Total int        `json:"total"`
}

func (t ToolsListOutput) FormatText(w io.Writer) error {
	if len(t.Tools) == 0 {
		fmt.Fprintln(w, "No tools available.")
		return nil
	}

	fmt.Fprintln(w, "Available tools:")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %-24s %-24s %-12s %s\n",
		"NAME", "DISPLAY NAME", "SOURCE", "DESCRIPTION")
	fmt.Fprintf(w, "  %-24s %-24s %-12s %s\n",
		"----", "------------", "------", "-----------")

	for _, tool := range t.Tools {
		desc := tool.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		displayName := tool.DisplayName
		if len(displayName) > 22 {
			displayName = displayName[:19] + "..."
		}
		fmt.Fprintf(w, "  %-24s %-24s %-12s %s\n",
			tool.Name, displayName, tool.Source, desc)
	}
	fmt.Fprintf(w, "\nTotal: %d tool(s)\n", t.Total)
	return nil
}

// AuthStatusOutput represents auth status.
type AuthStatusOutput struct {
	LoggedIn  bool       `json:"logged_in"`
	Server    string     `json:"server,omitempty"`
	User      string     `json:"user,omitempty"`
	UserID    string     `json:"user_id,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (a AuthStatusOutput) FormatText(w io.Writer) error {
	if !a.LoggedIn {
		fmt.Fprintln(w, "Not logged in.")
		return nil
	}

	fmt.Fprintf(w, "Server: %s\n", a.Server)
	fmt.Fprintf(w, "User:   %s", a.User)
	if a.UserID != "" {
		fmt.Fprintf(w, " (ID: %s)", a.UserID)
	}
	fmt.Fprintln(w)
	if a.ExpiresAt != nil {
		fmt.Fprintf(w, "Token expires: %s\n", a.ExpiresAt.Format("2006-01-02 15:04:05"))
	}
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
		fmt.Fprintln(w, "No flows defined.")
		fmt.Fprintln(w, "Create flows in .brain/flows/*.yaml")
		return nil
	}

	fmt.Fprintln(w, "Available flows:")
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
	fmt.Fprintf(w, "\nTotal: %d flow(s)\n", f.Total)
	return nil
}

// InitOutput represents the result of brain init.
type InitOutput struct {
	Root       string `json:"root"`
	ConfigDir  string `json:"config_dir"`
	ProjectID  string `json:"project_id,omitempty"`
	Server     string `json:"server,omitempty"`
	Workspace  string `json:"workspace,omitempty"`
	ClaimToken string `json:"claim_token,omitempty"`
	ClaimURL   string `json:"claim_url,omitempty"`
	ClaimEmail string `json:"claim_email,omitempty"`
}

func (o InitOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Initialized .brain/ project in: %s\n", o.Root)
	fmt.Fprintf(w, "Configuration: %s\n", o.ConfigDir)

	if o.ProjectID != "" {
		fmt.Fprintf(w, "\nProject created: %s\n", o.ProjectID)
	}
	if o.Workspace != "" {
		fmt.Fprintf(w, "Workspace: %s\n", o.Workspace)
	}
	if o.ClaimEmail != "" {
		fmt.Fprintf(w, "A claim link has been sent to %s\n", o.ClaimEmail)
	} else if o.ClaimURL != "" {
		fmt.Fprintf(w, "Claim URL: %s\n", o.ClaimURL)
	}

	fmt.Fprintln(w)
	if o.Server != "" {
		fmt.Fprintln(w, "Next steps:")
		fmt.Fprintln(w, "  1. Run: brain push    — upload content to the server")
		if o.ClaimEmail != "" {
			fmt.Fprintln(w, "  2. Check your email for the claim link to take ownership")
			fmt.Fprintln(w, "  3. Invite translators from the web dashboard")
		} else if o.ClaimURL != "" {
			fmt.Fprintln(w, "  2. Open the claim URL to take ownership of the project")
			fmt.Fprintln(w, "  3. Invite translators from the web dashboard")
		} else {
			fmt.Fprintln(w, "  2. Invite translators from the web dashboard")
		}
	} else {
		fmt.Fprintln(w, "Next steps:")
		fmt.Fprintln(w, "  1. Edit .brain/config.yaml to configure your project")
		fmt.Fprintln(w, "  2. Add file mappings to sync with Bowrain Server")
		fmt.Fprintln(w, "  3. Run: brain auth login")
		fmt.Fprintln(w, "  4. Run: brain pull to sync translations")
	}

	return nil
}

// LsEntry represents a single file in the ls output.
type LsEntry struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	Blocks int    `json:"blocks,omitempty"`
	Words  int    `json:"words,omitempty"`
	Dirty  int    `json:"dirty,omitempty"`
}

// LsOutput represents the result of brain ls.
type LsOutput struct {
	Files    []LsEntry `json:"files"`
	Total    int       `json:"total"`
	Blocks   int       `json:"blocks,omitempty"`
	Words    int       `json:"words,omitempty"`
	Changed  int       `json:"changed,omitempty"`
	HasStats bool      `json:"-"`
	HasDirty bool      `json:"-"`
}

func (o LsOutput) FormatText(w io.Writer) error {
	if len(o.Files) == 0 {
		fmt.Fprintln(w, "No tracked files.")
		return nil
	}

	if o.HasStats || o.HasDirty {
		// Compute column widths.
		pathW := 4 // "PATH"
		fmtW := 6  // "FORMAT"
		for _, f := range o.Files {
			if len(f.Path) > pathW {
				pathW = len(f.Path)
			}
			if len(f.Format) > fmtW {
				fmtW = len(f.Format)
			}
		}
		pathW += 2
		fmtW += 2

		// Header.
		header := fmt.Sprintf("  %-*s %-*s %8s %8s", pathW, "PATH", fmtW, "FORMAT", "BLOCKS", "WORDS")
		separator := fmt.Sprintf("  %-*s %-*s %8s %8s", pathW, "----", fmtW, "------", "------", "-----")
		if o.HasDirty {
			header += fmt.Sprintf(" %8s", "DIRTY")
			separator += fmt.Sprintf(" %8s", "-----")
		}
		fmt.Fprintln(w, header)
		fmt.Fprintln(w, separator)

		for _, f := range o.Files {
			line := fmt.Sprintf("  %-*s %-*s %8d %8d", pathW, f.Path, fmtW, f.Format, f.Blocks, f.Words)
			if o.HasDirty {
				line += fmt.Sprintf(" %8d", f.Dirty)
			}
			fmt.Fprintln(w, line)
		}

		// Summary.
		summary := fmt.Sprintf("\n%d file(s), %d blocks, %d words", o.Total, o.Blocks, o.Words)
		if o.HasDirty {
			summary += fmt.Sprintf(", %d changed", o.Changed)
		}
		fmt.Fprintln(w, summary)
	} else {
		// Simple two-column output.
		pathW := 4
		for _, f := range o.Files {
			if len(f.Path) > pathW {
				pathW = len(f.Path)
			}
		}
		pathW += 2

		for _, f := range o.Files {
			fmt.Fprintf(w, "%-*s %s\n", pathW, f.Path, f.Format)
		}
		fmt.Fprintf(w, "\n%d file(s)\n", o.Total)
	}
	return nil
}

// PullOutput represents the result of brain pull.
type PullOutput struct {
	BlocksPulled int  `json:"blocks_pulled"`
	LocalesCount int  `json:"locales_count"`
	FilesWritten int  `json:"files_written,omitempty"`
	DryRun       bool `json:"dry_run,omitempty"`
	UpToDate     bool `json:"up_to_date,omitempty"`
}

func (o PullOutput) FormatText(w io.Writer) error {
	if o.DryRun {
		fmt.Fprintf(w, "Would pull %d blocks for %d locales\n", o.BlocksPulled, o.LocalesCount)
		return nil
	}
	if o.UpToDate {
		fmt.Fprintln(w, "Already up to date.")
		return nil
	}
	fmt.Fprintf(w, "Pulled %d blocks for %d locales\n", o.BlocksPulled, o.LocalesCount)
	if o.FilesWritten > 0 {
		fmt.Fprintf(w, "Updated %d file(s)\n", o.FilesWritten)
	}
	return nil
}

// PushOutput represents the result of brain push.
type PushOutput struct {
	BlocksPushed int  `json:"blocks_pushed"`
	WordCount    int  `json:"word_count"`
	FilesScanned int  `json:"files_scanned"`
	DryRun       bool `json:"dry_run,omitempty"`
	UpToDate     bool `json:"up_to_date,omitempty"`
}

func (o PushOutput) FormatText(w io.Writer) error {
	if o.DryRun {
		fmt.Fprintf(w, "Would push %d blocks, %d words (scanned %d files)\n", o.BlocksPushed, o.WordCount, o.FilesScanned)
		return nil
	}
	if o.UpToDate {
		fmt.Fprintln(w, "Already up to date.")
		return nil
	}
	fmt.Fprintf(w, "Pushed %d blocks, %d words (scanned %d files)\n", o.BlocksPushed, o.WordCount, o.FilesScanned)
	return nil
}

// ConfigOutput represents the result of brain config.
type ConfigOutput struct {
	Path   string `json:"path,omitempty"`
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
	Action string `json:"action,omitempty"` // "path", "get", "set"
}

func (o ConfigOutput) FormatText(w io.Writer) error {
	switch o.Action {
	case "set":
		fmt.Fprintf(w, "Set %s = %s in %s\n", o.Key, o.Value, o.Path)
	case "get":
		fmt.Fprintln(w, o.Value)
	case "path":
		fmt.Fprintln(w, o.Path)
	}
	return nil
}

// TermbaseImportOutput represents the result of brain termbase import.
type TermbaseImportOutput struct {
	Imported int    `json:"imported"`
	DBPath   string `json:"db_path"`
	Total    int    `json:"total"`
}

func (o TermbaseImportOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Imported %d concepts into %s (total: %d)\n", o.Imported, o.DBPath, o.Total)
	return nil
}

// TermbaseExportOutput represents the result of brain termbase export.
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

// TermbaseLookupOutput represents the result of brain termbase lookup.
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

// TermbaseSearchOutput represents the result of brain termbase search.
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

// TermbaseStatsOutput represents the result of brain termbase stats.
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
		keys := sortedKeys(o.Locales)
		for _, k := range keys {
			fmt.Fprintf(w, "    %-10s %d terms\n", k, o.Locales[k])
		}
	}

	if len(o.Domains) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Domains:")
		keys := sortedKeys(o.Domains)
		for _, k := range keys {
			fmt.Fprintf(w, "    %-20s %d concepts\n", k, o.Domains[k])
		}
	}

	if len(o.Statuses) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Term statuses:")
		keys := sortedKeys(o.Statuses)
		for _, k := range keys {
			fmt.Fprintf(w, "    %-12s %d\n", k, o.Statuses[k])
		}
	}

	return nil
}

// sortedKeys returns the keys of a map[string]int sorted alphabetically.
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

// FlowRunOutput represents the result of brain flow run.
type FlowRunOutput struct {
	FlowName       string `json:"flow_name"`
	InputPath      string `json:"input_path,omitempty"`
	OutputPath     string `json:"output_path,omitempty"`
	FilesProcessed int    `json:"files_processed,omitempty"`
}

func (o FlowRunOutput) FormatText(w io.Writer) error {
	if o.FilesProcessed > 0 {
		fmt.Fprintf(w, "Flow %s completed: processed %d files\n", o.FlowName, o.FilesProcessed)
	} else {
		fmt.Fprintf(w, "Flow %s completed: %s → %s\n", o.FlowName, o.InputPath, o.OutputPath)
	}
	return nil
}

// PresetEntry represents a single preset in the list output.
type PresetEntry struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"` // "format" or "framework"
	Description string         `json:"description,omitempty"`
	Format      string         `json:"format,omitempty"` // target format (format presets only)
	Source      string         `json:"source,omitempty"`
	IsDefault   bool           `json:"is_default,omitempty"`
	Config      map[string]any `json:"config,omitempty"`   // parameter values (format presets only)
	Mappings    []MappingEntry `json:"mappings,omitempty"` // framework presets only
	Exclude     []string       `json:"exclude,omitempty"`  // framework presets only
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

	// Separate framework and format presets.
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
		for _, p := range fmtPresets {
			def := ""
			if p.IsDefault {
				def = " (default)"
			}
			fmt.Fprintf(w, "  %-20s %s → %s [%s]%s\n", p.Name, p.Description, p.Format, p.Source, def)
		}
	}

	fmt.Fprintf(w, "\nTotal: %d preset(s)\n", p.Total)
	return nil
}

// PresetShowOutput represents the details of a single preset.
type PresetShowOutput struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"` // "format" or "framework"
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

// PresetsValidateOutput represents the result of preset validation.
type PresetsValidateOutput struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

func (p PresetsValidateOutput) FormatText(w io.Writer) error {
	if p.Valid {
		fmt.Fprintln(w, "All presets and overrides are valid.")
		return nil
	}
	fmt.Fprintf(w, "Found %d validation error(s):\n", len(p.Errors))
	for _, e := range p.Errors {
		fmt.Fprintf(w, "  - %s\n", e)
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

// PluginSearchOutput represents the result of brain plugins search.
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

// PluginInstallOutput represents the result of brain plugins install.
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

// PluginUpdateOutput represents the result of brain plugins update.
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

// PluginRemoveOutput represents the result of brain plugins remove.
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

// AuthLoginOutput represents the result of brain auth login.
type AuthLoginOutput struct {
	Server string `json:"server"`
	User   string `json:"user,omitempty"`
}

func (o AuthLoginOutput) FormatText(w io.Writer) error {
	if o.User != "" {
		fmt.Fprintf(w, "Logged in as %s\n", o.User)
	} else {
		fmt.Fprintln(w, "Login successful! Token saved.")
	}
	return nil
}

// AuthLogoutOutput represents the result of brain auth logout.
type AuthLogoutOutput struct {
	WasLoggedIn bool `json:"was_logged_in"`
}

func (o AuthLogoutOutput) FormatText(w io.Writer) error {
	if o.WasLoggedIn {
		fmt.Fprintln(w, "Logged out. Token removed.")
	} else {
		fmt.Fprintln(w, "No stored token found.")
	}
	return nil
}

// AuthClaimOutput represents the result of brain auth claim.
type AuthClaimOutput struct {
	ProjectID     string `json:"project_id"`
	WorkspaceSlug string `json:"workspace_slug"`
}

func (o AuthClaimOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Project claimed into workspace %s\n", o.WorkspaceSlug)
	fmt.Fprintf(w, "Project ID: %s\n", o.ProjectID)
	return nil
}
