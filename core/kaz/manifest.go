package kaz

// Manifest is the manifest.yaml inside a .kaz package.
type Manifest struct {
	Name            string         `yaml:"name"`
	Version         string         `yaml:"version"`
	GokapiVersion   string         `yaml:"gokapi_version"`
	SourceLocale    string         `yaml:"source_locale"`
	TargetLocales   []string       `yaml:"target_locales"`
	CreatedAt       string         `yaml:"created_at"`
	ModifiedAt      string         `yaml:"modified_at"`
	FormatsRequired []string       `yaml:"formats_required"`
	PluginsRequired []string       `yaml:"plugins_required"`
	Items           []ItemManifest `yaml:"items"`
}

// ItemManifest describes an item entry in the manifest.
type ItemManifest struct {
	Path       string `yaml:"path"`
	Format     string `yaml:"format"`
	Type       string `yaml:"type"` // "file", "data", etc.
	Size       int64  `yaml:"size"`
	BlockCount int    `yaml:"block_count"`
	WordCount  int    `yaml:"word_count"`
}
