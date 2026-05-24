package cli

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/spf13/cobra"
)

// NewPresetsCmd creates the presets command group (presets list, presets show).
func (a *App) NewPresetsCmd() *cobra.Command {
	presetsCmd := &cobra.Command{
		Use:     "presets",
		Short:   "Manage format and framework presets",
		GroupID: "management",
		Example: "  kapi presets list\n  kapi presets show AndroidStrings",
	}

	var (
		formatFilter  string
		frameworkOnly bool
	)

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List available presets",
		Example: "  kapi presets list\n  kapi presets list --format okf_xml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg := preset.NewPresetRegistry()
			preset.RegisterBuiltins(reg)

			var entries []output.PresetEntry

			if frameworkOnly {
				entries = collectFrameworkPresets(reg)
			} else if formatFilter != "" {
				entries = collectFormatPresets(reg, formatFilter)
			} else {
				entries = collectAllPresets(reg)
			}

			out := output.PresetsListOutput{
				Presets: entries,
				Total:   len(entries),
			}
			return output.Print(cmd, out)
		},
	}

	listCmd.Flags().StringVar(&formatFilter, "format", "", "filter by format")
	listCmd.Flags().BoolVar(&frameworkOnly, "framework", false, "list framework presets only")

	showCmd := &cobra.Command{
		Use:   "show [preset-name]",
		Short: "Show preset details",
		Long: `Show detailed configuration for a preset.

Accepts a bare preset name (searched across all formats) or the
qualified format:preset syntax (e.g. okf_xml:AndroidStrings).`,
		Example: "  kapi presets show AndroidStrings\n  kapi presets show okf_xml:AndroidStrings",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			reg := preset.NewPresetRegistry()
			preset.RegisterBuiltins(reg)

			// Parse format:preset syntax.
			var formatFilter string
			if i := strings.Index(name, ":"); i > 0 {
				formatFilter = name[:i]
				name = name[i+1:]
			}

			// Try framework preset first (only if no format qualifier).
			if formatFilter == "" {
				fp := reg.GetFrameworkPreset(name)
				if fp != nil {
					entry := frameworkPresetEntry(fp)
					show := output.PresetShowOutput{
						Name:        entry.Name,
						Type:        entry.Type,
						Description: entry.Description,
						Source:      entry.Source,
						Mappings:    entry.Mappings,
						Exclude:     entry.Exclude,
					}
					return output.Print(cmd, show)
				}
			}

			// Try format presets — either for the specific format or across all.
			formats := reg.FormatNames()
			if formatFilter != "" {
				formats = []string{formatFilter}
			}
			for _, format := range formats {
				p := reg.GetFormatPreset(format, name)
				if p != nil {
					show := output.PresetShowOutput{
						Name:        p.Name,
						Type:        "format",
						Description: p.Description,
						Format:      p.Format,
						Source:      p.Source,
						IsDefault:   p.IsDefault,
						Config:      p.Config,
					}
					return output.Print(cmd, show)
				}
			}

			if formatFilter != "" {
				return fmt.Errorf("preset %q not found for format %q", name, formatFilter)
			}
			return fmt.Errorf("preset %q not found", name)
		},
	}

	presetsCmd.AddCommand(listCmd)
	presetsCmd.AddCommand(showCmd)
	return presetsCmd
}

func collectAllPresets(reg *preset.PresetRegistry) []output.PresetEntry {
	var entries []output.PresetEntry

	for _, p := range reg.ListFrameworkPresets() {
		entries = append(entries, frameworkPresetEntry(p))
	}

	for _, format := range reg.FormatNames() {
		for _, p := range reg.ListFormatPresets(format) {
			entries = append(entries, formatPresetEntry(format, p))
		}
	}

	return entries
}

func collectFrameworkPresets(reg *preset.PresetRegistry) []output.PresetEntry {
	var entries []output.PresetEntry
	for _, p := range reg.ListFrameworkPresets() {
		entries = append(entries, frameworkPresetEntry(p))
	}
	return entries
}

func collectFormatPresets(reg *preset.PresetRegistry, format string) []output.PresetEntry {
	var entries []output.PresetEntry
	for _, p := range reg.ListFormatPresets(format) {
		entries = append(entries, formatPresetEntry(format, p))
	}
	return entries
}

func frameworkPresetEntry(p *preset.FrameworkPreset) output.PresetEntry {
	entry := output.PresetEntry{
		Name:        p.Name,
		Type:        "framework",
		Description: p.Description,
		Source:      p.Source,
	}
	for _, m := range p.Mappings {
		entry.Mappings = append(entry.Mappings, output.MappingEntry{
			Local:      m.Local,
			Format:     m.Format,
			TargetPath: m.TargetPath,
		})
	}
	if len(p.Exclude) > 0 {
		entry.Exclude = p.Exclude
	}
	return entry
}

func formatPresetEntry(format string, p *preset.FormatPreset) output.PresetEntry {
	return output.PresetEntry{
		Name:        format + ":" + p.Name,
		Type:        "format",
		Description: p.Description,
		Format:      p.Format,
		Source:      p.Source,
		IsDefault:   p.IsDefault,
		Config:      p.Config,
	}
}
