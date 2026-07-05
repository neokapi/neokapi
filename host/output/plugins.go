package output

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// PluginListRow is one installed plugin in a PluginListOutput.
type PluginListRow struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	License string `json:"license,omitempty"`
	Source  string `json:"source,omitempty"`
	// Status is "active" normally, or "retired" when the plugin is installed but
	// no longer loaded by this kapi version. Retired plugins also set Retirement.
	Status string `json:"status,omitempty"`
	// Retirement is the multi-line retirement notice, set only for retired rows.
	Retirement string `json:"retirement,omitempty"`
}

// PluginListOutput lists installed plugins.
type PluginListOutput struct {
	Plugins []PluginListRow `json:"plugins"`
	Total   int             `json:"total"`
}

func (o PluginListOutput) FormatText(w io.Writer) error {
	if o.Total == 0 {
		fmt.Fprintln(w, "No plugins installed.")
		fmt.Fprintln(w, "Search the registry: kapi plugin search")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tVERSION\tLICENSE\tSOURCE\tSTATUS")
	var retired []PluginListRow
	for _, p := range o.Plugins {
		status := p.Status
		if status == "" {
			status = "active"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.Version, p.License, p.Source, status)
		if p.Retirement != "" {
			retired = append(retired, p)
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, p := range retired {
		fmt.Fprintf(w, "\n⚠ %s\n", p.Retirement)
	}
	return nil
}

// PluginInfoOutput describes one installed plugin.
type PluginInfoOutput struct {
	Plugin           string `json:"plugin"`
	Version          string `json:"version"`
	License          string `json:"license,omitempty"`
	Author           string `json:"author,omitempty"`
	Homepage         string `json:"homepage,omitempty"`
	InstallDir       string `json:"install_dir,omitempty"`
	Source           string `json:"source,omitempty"`
	Binary           string `json:"binary,omitempty"`
	Commands         int    `json:"commands,omitempty"`
	MCPTools         int    `json:"mcp_tools,omitempty"`
	Formats          int    `json:"formats,omitempty"`
	SchemaExtensions int    `json:"schema_extensions,omitempty"`
	Models           int    `json:"models,omitempty"`
}

func (o PluginInfoOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Plugin:        %s\n", o.Plugin)
	fmt.Fprintf(w, "Version:       %s\n", o.Version)
	fmt.Fprintf(w, "License:       %s\n", o.License)
	if o.Author != "" {
		fmt.Fprintf(w, "Author:        %s\n", o.Author)
	}
	if o.Homepage != "" {
		fmt.Fprintf(w, "Homepage:      %s\n", o.Homepage)
	}
	fmt.Fprintf(w, "Install dir:   %s\n", o.InstallDir)
	fmt.Fprintf(w, "Source:        %s\n", o.Source)
	fmt.Fprintf(w, "Binary:        %s\n", o.Binary)
	if o.Commands > 0 {
		fmt.Fprintf(w, "Commands:      %d\n", o.Commands)
	}
	if o.MCPTools > 0 {
		fmt.Fprintf(w, "MCP tools:     %d\n", o.MCPTools)
	}
	if o.Formats > 0 {
		fmt.Fprintf(w, "Formats:       %d\n", o.Formats)
	}
	if o.SchemaExtensions > 0 {
		fmt.Fprintf(w, "Schema exts:   %d\n", o.SchemaExtensions)
	}
	if o.Models > 0 {
		fmt.Fprintf(w, "Models:        %d\n", o.Models)
	}
	return nil
}

// Registry search reuses the existing PluginSearchOutput / PluginSearchEntry
// (output/types.go); the search command renders its own word-wrapped text form
// and only uses PluginSearchOutput for the --json shape.
