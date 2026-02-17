package output

import (
	"fmt"
	"io"
	"strings"
	"time"
)

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
		fmt.Fprintln(w, "\nSync status requires a Bowrain server connection.")
		fmt.Fprintf(w, "  Configure server in %s/config.yaml\n", s.Project.ConfigDir)
		return nil
	}

	fmt.Fprintf(w, "\nLocal blocks: %d\n", s.ItemCount)

	if s.PendingPush > 0 {
		fmt.Fprintf(w, "Pending push: %d blocks changed locally\n", s.PendingPush)
	}
	if s.PendingPull > 0 {
		fmt.Fprintf(w, "Pending pull: %d remote changes available\n", s.PendingPull)
	} else if s.PendingPull < 0 {
		fmt.Fprintln(w, "Pending pull: remote changes available (count unknown)")
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
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Status  string `json:"status"`
	Formats int    `json:"formats,omitempty"`
	Path    string `json:"path,omitempty"`
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
	fmt.Fprintf(w, "  %-20s %-10s %-10s %-8s %s\n",
		"NAME", "VERSION", "STATUS", "FORMATS", "PATH")
	fmt.Fprintf(w, "  %-20s %-10s %-10s %-8s %s\n",
		"----", "-------", "------", "-------", "----")

	for _, plugin := range p.Plugins {
		fmt.Fprintf(w, "  %-20s %-10s %-10s %-8d %s\n",
			plugin.Name, plugin.Version, plugin.Status, plugin.Formats, plugin.Path)
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
