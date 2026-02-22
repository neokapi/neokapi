package output

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// AddEntry represents a single pattern added by kapi add.
type AddEntry struct {
	Pattern string `json:"pattern"`
	Format  string `json:"format,omitempty"`
	Files   int    `json:"files"`
	Skipped bool   `json:"skipped,omitempty"`
}

// AddOutput represents the result of kapi add.
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

// RmEntry represents a single pattern processed by kapi rm.
type RmEntry struct {
	Pattern string `json:"pattern"`
	Action  string `json:"action"`           // "removed", "excluded", "already_excluded"
	Format  string `json:"format,omitempty"` // only for "removed"
	Files   int    `json:"files,omitempty"`  // only for "excluded"
}

// RmOutput represents the result of kapi rm.
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
		fmt.Fprintf(w, "kapi %s (commit: %s, built: %s)\n", v.Version, v.Commit, v.BuildDate)
	} else {
		fmt.Fprintf(w, "kapi %s\n", v.Version)
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
		fmt.Fprintln(w, "  Run 'kapi init' with a server or set one in .kapi/config.yaml")
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
		fmt.Fprintln(w, "Create flows in .kapi/flows/*.yaml")
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

// InitOutput represents the result of kapi init.
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
	fmt.Fprintf(w, "Initialized .kapi/ project in: %s\n", o.Root)
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
		fmt.Fprintln(w, "  1. Run: kapi push    — upload content to the server")
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
		fmt.Fprintln(w, "  1. Edit .kapi/config.yaml to configure your project")
		fmt.Fprintln(w, "  2. Add file mappings to sync with Bowrain Server")
		fmt.Fprintln(w, "  3. Run: kapi auth login")
		fmt.Fprintln(w, "  4. Run: kapi pull to sync translations")
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

// LsOutput represents the result of kapi ls.
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
