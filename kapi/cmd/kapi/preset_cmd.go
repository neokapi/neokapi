package main

import (
	"fmt"

	"github.com/gokapi/gokapi/core/preset"
	"github.com/gokapi/gokapi/kapi/cmd/kapi/output"
	"github.com/spf13/cobra"
)

var presetsCmd = &cobra.Command{
	Use:   "presets",
	Short: "Manage format and framework presets",
}

var presetsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available presets",
	RunE:  runPresetsList,
}

var presetsShowCmd = &cobra.Command{
	Use:   "show [preset-name]",
	Short: "Show preset details",
	Args:  cobra.ExactArgs(1),
	RunE:  runPresetsShow,
}

var (
	presetsFormatFilter  string
	presetsFrameworkOnly bool
)

func runPresetsList(cmd *cobra.Command, _ []string) error {
	reg := pluginLoader.Presets()
	preset.RegisterBuiltins(reg)

	var entries []output.PresetEntry

	if presetsFrameworkOnly {
		entries = collectFrameworkPresets(reg)
	} else if presetsFormatFilter != "" {
		entries = collectFormatPresets(reg, presetsFormatFilter)
	} else {
		entries = collectAllPresets(reg)
	}

	out := output.PresetsListOutput{
		Presets: entries,
		Total:   len(entries),
	}
	return output.Print(cmd, out)
}

func collectAllPresets(reg *preset.PresetRegistry) []output.PresetEntry {
	var entries []output.PresetEntry

	// Framework presets
	for _, p := range reg.ListFrameworkPresets() {
		entries = append(entries, frameworkPresetEntry(p))
	}

	// Format presets grouped by format name
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
		Name:        format + "@" + p.Name,
		Type:        "format",
		Description: p.Description,
		Format:      p.Format,
		Source:      p.Source,
		IsDefault:   p.IsDefault,
		Config:      p.Config,
	}
}

func runPresetsShow(cmd *cobra.Command, args []string) error {
	name := args[0]
	reg := pluginLoader.Presets()
	preset.RegisterBuiltins(reg)

	// Try framework preset first
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

	// Try format presets across all formats
	for _, format := range reg.FormatNames() {
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

	return fmt.Errorf("preset %q not found", name)
}

func init() {
	presetsListCmd.Flags().StringVar(&presetsFormatFilter, "format", "", "filter by format")
	presetsListCmd.Flags().BoolVar(&presetsFrameworkOnly, "framework", false, "list framework presets only")

	presetsCmd.AddCommand(presetsListCmd)
	presetsCmd.AddCommand(presetsShowCmd)
	rootCmd.AddCommand(presetsCmd)
}
