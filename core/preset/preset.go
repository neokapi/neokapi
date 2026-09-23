package preset

// FormatPreset is a named configuration bundle for a specific format.
type FormatPreset struct {
	Name        string         // Preset name (e.g., "wellFormed")
	Description string         // Human-readable description
	Format      string         // Target format ID (e.g., "okf_html")
	Config      map[string]any // Complete or partial parameter set
	Source      string         // "bridge", plugin name, or "local"
	IsDefault   bool           // Whether this is the format's default config
}

// FrameworkPreset provides a complete project setup template.
type FrameworkPreset struct {
	Name          string                    // Preset name (e.g., "nextjs")
	Description   string                    // Human-readable description
	Detect        []string                  // Files whose presence indicates this framework (e.g., "next.config.*")
	Mappings      []MappingTemplate         // Default file mappings
	Exclude       []string                  // Default exclude patterns
	FormatPresets map[string]map[string]any // format -> config overrides
	Flows         map[string]map[string]any // flow -> config defaults
	Source        string                    // "built-in" or plugin name
}

// MappingTemplate is a mapping entry from a framework preset.
type MappingTemplate struct {
	Local      string // Glob pattern
	Remote     string // Remote template
	Format     string // Format ID (may include :preset)
	TargetPath string // Target locale template
}
