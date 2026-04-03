package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/model"
)

const (
	// BowrainDir is the project directory name.
	BowrainDir = ".bowrain"

	// ConfigFile is the project configuration file.
	ConfigFile = "config.yaml"

	// FlowsDir is the flows directory.
	FlowsDir = "flows"
)

// Project represents a .bowrain/ project.
type Project struct {
	// Root is the project root directory (contains .bowrain/).
	Root string

	// ConfigDir is the .bowrain/ directory path.
	ConfigDir string

	// Config is the loaded project configuration.
	Config *Config
}

// FindProject searches for a .bowrain/ directory starting from the current directory
// and walking up the directory tree (like git).
func FindProject(startDir string) (*Project, error) {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}

	absStart, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("absolute path: %w", err)
	}

	dir := absStart
	for {
		bowrainDir := filepath.Join(dir, BowrainDir)
		if st, err := os.Stat(bowrainDir); err == nil && st.IsDir() {
			cfg, err := LoadConfig(bowrainDir)
			if err != nil {
				return nil, fmt.Errorf("load config: %w", err)
			}

			return &Project{
				Root:      dir,
				ConfigDir: bowrainDir,
				Config:    cfg,
			}, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("no .bowrain/ directory found (searched from %s)", absStart)
		}
		dir = parent
	}
}

// InitProject creates a new .bowrain/ project in the specified directory.
func InitProject(root string, cfg *Config) (*Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("absolute path: %w", err)
	}

	bowrainDir := filepath.Join(absRoot, BowrainDir)

	if _, err := os.Stat(bowrainDir); err == nil {
		return nil, fmt.Errorf(".bowrain/ directory already exists at %s", bowrainDir)
	}

	if err := os.MkdirAll(bowrainDir, 0755); err != nil {
		return nil, fmt.Errorf("create .bowrain directory: %w", err)
	}

	flowsDir := filepath.Join(bowrainDir, FlowsDir)
	if err := os.MkdirAll(flowsDir, 0755); err != nil {
		return nil, fmt.Errorf("create flows directory: %w", err)
	}

	if err := SaveConfig(bowrainDir, cfg); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}

	gitignorePath := filepath.Join(bowrainDir, ".gitignore")
	gitignoreContent := "# Bowrain sync cache (local only)\n.sync-cache\n"
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		return nil, fmt.Errorf("create .gitignore: %w", err)
	}

	return &Project{
		Root:      absRoot,
		ConfigDir: bowrainDir,
		Config:    cfg,
	}, nil
}

// ResolvePath resolves a local path relative to the project root.
func (p *Project) ResolvePath(localPath string) string {
	if filepath.IsAbs(localPath) {
		return localPath
	}
	return filepath.Join(p.Root, localPath)
}

// RelativePath returns a path relative to the project root.
func (p *Project) RelativePath(absPath string) (string, error) {
	return filepath.Rel(p.Root, absPath)
}

// FlowsDirPath returns the flows directory path.
func (p *Project) FlowsDirPath() string {
	return filepath.Join(p.ConfigDir, FlowsDir)
}

// Config represents the .bowrain/config.yaml structure.
type Config struct {
	// Version is the config schema version (e.g. "v1").
	Version string `yaml:"version,omitempty"`

	// URL is a compound project URL that encodes the server, workspace, and project ID.
	// Examples:
	//   https://bowrain.example.com/my-team/abc123     (workspace project)
	//   https://bowrain.example.com/projects/abc123     (direct project)
	URL string `yaml:"url,omitempty"`

	// Stream determines which content stream to sync with.
	// Default: "$auto" — auto-detect from git branch / CI environment.
	// Set to a specific name (e.g. "v2.0") to always use that stream.
	// Empty is treated as "$auto".
	Stream string `yaml:"stream,omitempty"`

	// Defaults contains project-wide defaults for language and organization.
	Defaults Defaults `yaml:"defaults"`

	// Content entries define which files to track (replaces v1 "mappings").
	Content []ContentEntry `yaml:"content,omitempty"`

	// Plugins is a flat list of plugin dependencies (e.g. ["okapi@1.0.0"]).
	Plugins []string `yaml:"plugins,omitempty"`

	// Plugin registries (overrides global registries when present)
	Registries []RegistryConfig `yaml:"registries,omitempty"`

	// Framework preset name (e.g., "nextjs")
	Preset string `yaml:"preset,omitempty"`

	// Local format preset definitions
	FormatPresets map[string]LocalFormatPreset `yaml:"format_presets,omitempty"`

	// Exclude patterns — files matching these are skipped during scanning
	Exclude []string `yaml:"exclude,omitempty"`

	// Flow hooks — flows that run automatically at lifecycle points
	Hooks map[string][]string `yaml:"hooks,omitempty"`

	// Flow-specific settings
	Flows map[string]map[string]any `yaml:"flows,omitempty"`

	// Automations defines local automation rules (pre-push, post-push, etc.)
	Automations []AutomationConfig `yaml:"automations,omitempty"`

	// Assets configures project-wide media asset sync behavior (AD-029).
	Assets *AssetConfig `yaml:"assets,omitempty"`

	// BrandVoice configures brand voice profile bindings for this project (AD-025).
	BrandVoice *BrandVoiceProjectConfig `yaml:"brand_voice,omitempty" json:"brand_voice,omitempty"`
}

// BrandVoiceProjectConfig holds brand voice bindings for a project.
type BrandVoiceProjectConfig struct {
	// Profile is the default brand voice profile ID for this project.
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
	// Channel is the default channel override key for this project.
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`
	// Collections maps collection names to brand voice entry configs.
	Collections map[string]*BrandVoiceEntryConfig `yaml:"collections,omitempty" json:"collections,omitempty"`
}

// BrandVoiceEntryConfig holds a brand voice binding for a specific scope.
type BrandVoiceEntryConfig struct {
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`
}

// AssetConfig controls project-wide media asset sync settings.
type AssetConfig struct {
	// Enabled is the master toggle for asset sync (default: true).
	Enabled *bool `yaml:"enabled,omitempty"`
	// Exclude is a list of filename glob patterns to skip.
	Exclude []string `yaml:"exclude,omitempty"`
	// MaxSize is the global per-asset size limit (e.g., "100MB").
	MaxSize string `yaml:"max_size,omitempty"`
}

// AssetsEnabled returns true if asset sync is enabled (default: true).
func (c *Config) AssetsEnabled() bool {
	if c.Assets == nil || c.Assets.Enabled == nil {
		return true
	}
	return *c.Assets.Enabled
}

// Defaults contains project-wide language and organization settings.
type Defaults struct {
	SourceLanguage  model.LocaleID   `yaml:"source_language"`
	TargetLanguages []model.LocaleID `yaml:"target_languages,omitempty"`
	Collection      string           `yaml:"collection,omitempty"`
}

// ContentEntry defines a tracked file pattern (replaces v1 Mapping).
type ContentEntry struct {
	// Path is the glob pattern for source files. May contain {lang} placeholder.
	Path string `yaml:"path"`

	// Dest is the output path pattern for target files. May contain {lang} placeholder.
	Dest string `yaml:"dest,omitempty"`

	// Format is the file format ID (e.g. "json", "html") or "$auto" for auto-detection.
	Format string `yaml:"format,omitempty"`

	// Base is the path prefix to strip when reporting files to Bowrain.
	Base string `yaml:"base,omitempty"`

	// Collection overrides the default collection for this content entry.
	Collection string `yaml:"collection,omitempty"`

	// Language overrides the source language for this entry.
	Language string `yaml:"language,omitempty"`

	// TargetLanguages overrides the default target languages for this entry.
	// Use "$auto" to inherit from defaults or auto-detect from server.
	TargetLanguages []model.LocaleID `yaml:"target_languages,omitempty"`

	// Overrides are per-entry format config overrides.
	Overrides map[string]any `yaml:"overrides,omitempty"`

	// Assets controls whether embedded assets are synced for this entry (default: true).
	Assets *bool `yaml:"assets,omitempty"`

	// AssetMaxSize is the per-asset size limit for this entry (e.g., "50MB").
	AssetMaxSize string `yaml:"asset_max_size,omitempty"`
}

// EffectiveLanguage returns the source language for this entry,
// falling back to the project default if not overridden.
func (ce ContentEntry) EffectiveLanguage(defaultLang model.LocaleID) string {
	if ce.Language != "" {
		return ce.Language
	}
	return string(defaultLang)
}

// EffectiveTargetLanguages returns the target languages for this entry.
// If the entry has its own target_languages, those are used.
// Otherwise falls back to the provided defaults.
func (ce ContentEntry) EffectiveTargetLanguages(defaults []model.LocaleID) []model.LocaleID {
	if len(ce.TargetLanguages) > 0 {
		return ce.TargetLanguages
	}
	return defaults
}

// RegistryConfig represents a named plugin registry in project config.
type RegistryConfig struct {
	Name     string   `yaml:"name"               json:"name"`
	URL      string   `yaml:"url"                json:"url"`
	Channels []string `yaml:"channels,omitempty" json:"channels,omitempty"`
}

// LocalFormatPreset defines a user-defined format preset in config.yaml.
type LocalFormatPreset struct {
	Description string         `yaml:"description,omitempty"`
	Base        string         `yaml:"base,omitempty"` // base format ID
	Config      map[string]any `yaml:"config"`
}

// AutomationConfig defines a local automation rule in .bowrain/config.yaml.
type AutomationConfig struct {
	Name    string         `yaml:"name"`
	Trigger string         `yaml:"trigger"` // "pre-push", "post-push", "pre-pull", "post-pull", "pre-flow", "post-flow"
	Actions []ActionConfig `yaml:"actions"`
	Enabled *bool          `yaml:"enabled,omitempty"` // nil = true
}

// IsEnabled returns whether the automation is enabled (defaults to true).
func (a AutomationConfig) IsEnabled() bool {
	return a.Enabled == nil || *a.Enabled
}

// ActionConfig defines a single action in a local automation rule.
type ActionConfig struct {
	Type   string            `yaml:"type"`             // "run_flow", "wait_translate", "pull", "push"
	Config map[string]string `yaml:"config,omitempty"` // e.g. {"flow": "qa-check", "timeout": "5m"}
}

// --- Convenience accessors for server details parsed from the compound URL ---

// ServerURL returns the base server URL extracted from the compound URL.
// Returns empty string if no URL is configured.
func (c *Config) ServerURL() string {
	info := ParseProjectURL(c.URL)
	return info.ServerURL
}

// ProjectID returns the project ID extracted from the compound URL.
func (c *Config) ProjectID() string {
	info := ParseProjectURL(c.URL)
	return info.ProjectID
}

// Workspace returns the workspace slug extracted from the compound URL.
func (c *Config) Workspace() string {
	info := ParseProjectURL(c.URL)
	return info.Workspace
}

// HasServer returns true if a server URL is configured.
func (c *Config) HasServer() bool {
	return c.URL != ""
}

// SourceLocale returns the source language as a convenience accessor.
func (c *Config) SourceLocale() model.LocaleID {
	return c.Defaults.SourceLanguage
}

// TargetLocales returns the target languages as a convenience accessor.
func (c *Config) TargetLocales() []model.LocaleID {
	return c.Defaults.TargetLanguages
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		Version: ProjectConfigVersion,
		Defaults: Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{},
		},
		Content: []ContentEntry{},
		Hooks:   map[string][]string{},
		Flows:   map[string]map[string]any{},
	}
}
