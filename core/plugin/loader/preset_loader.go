package loader

import (
	"fmt"
	"os"

	"github.com/gokapi/gokapi/core/preset"
	"gopkg.in/yaml.v3"
)

// presetManifest is the YAML structure for presets.yaml files.
type presetManifest struct {
	Kind            string                         `yaml:"kind"`
	FormatPresets   map[string]formatPresetEntry    `yaml:"format_presets,omitempty"`
	FrameworkPresets map[string]frameworkPresetEntry `yaml:"framework_presets,omitempty"`
}

type formatPresetEntry struct {
	Description string                   `yaml:"description"`
	Formats     []formatPresetFormatEntry `yaml:"formats"`
}

type formatPresetFormatEntry struct {
	Format string         `yaml:"format"`
	Config map[string]any `yaml:"config"`
}

type frameworkPresetEntry struct {
	Description   string                    `yaml:"description"`
	Mappings      []frameworkMappingEntry    `yaml:"mappings,omitempty"`
	Exclude       []string                  `yaml:"exclude,omitempty"`
	FormatPresets map[string]map[string]any `yaml:"format_presets,omitempty"`
	Flows         map[string]map[string]any `yaml:"flows,omitempty"`
}

type frameworkMappingEntry struct {
	Local      string `yaml:"local"`
	Remote     string `yaml:"remote,omitempty"`
	Format     string `yaml:"format"`
	TargetPath string `yaml:"target_path,omitempty"`
}

// LoadPresetsFromFile loads format and framework presets from a presets.yaml file.
func LoadPresetsFromFile(path string, reg *preset.PresetRegistry, source string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading presets file: %w", err)
	}

	var manifest presetManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parsing presets YAML: %w", err)
	}

	// Load format presets
	for name, entry := range manifest.FormatPresets {
		for _, f := range entry.Formats {
			reg.RegisterFormatPreset(f.Format, name, &preset.FormatPreset{
				Name:        name,
				Description: entry.Description,
				Format:      f.Format,
				Config:      f.Config,
				Source:      source,
			})
		}
	}

	// Load framework presets
	for name, entry := range manifest.FrameworkPresets {
		fp := &preset.FrameworkPreset{
			Name:          name,
			Description:   entry.Description,
			Exclude:       entry.Exclude,
			FormatPresets: entry.FormatPresets,
			Flows:         entry.Flows,
			Source:        source,
		}
		for _, m := range entry.Mappings {
			fp.Mappings = append(fp.Mappings, preset.MappingTemplate{
				Local:      m.Local,
				Remote:     m.Remote,
				Format:     m.Format,
				TargetPath: m.TargetPath,
			})
		}
		reg.RegisterFrameworkPreset(name, fp)
	}

	return nil
}
