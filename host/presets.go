package host

import (
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/host/output"
)

func CollectAllPresets(reg *preset.PresetRegistry) []output.PresetEntry {
	var entries []output.PresetEntry

	for _, p := range reg.ListFrameworkPresets() {
		entries = append(entries, FrameworkPresetEntry(p))
	}

	for _, format := range reg.FormatNames() {
		for _, p := range reg.ListFormatPresets(format) {
			entries = append(entries, formatPresetEntry(format, p))
		}
	}

	return entries
}

func CollectFrameworkPresets(reg *preset.PresetRegistry) []output.PresetEntry {
	var entries []output.PresetEntry
	for _, p := range reg.ListFrameworkPresets() {
		entries = append(entries, FrameworkPresetEntry(p))
	}
	return entries
}

func CollectFormatPresets(reg *preset.PresetRegistry, format string) []output.PresetEntry {
	var entries []output.PresetEntry
	for _, p := range reg.ListFormatPresets(format) {
		entries = append(entries, formatPresetEntry(format, p))
	}
	return entries
}

func FrameworkPresetEntry(p *preset.FrameworkPreset) output.PresetEntry {
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
