package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gokapi/gokapi/core/model"
)

const (
	// BrainDir is the project directory name.
	BrainDir = ".brain"

	// ConfigFile is the project configuration file.
	ConfigFile = "config.yaml"

	// FlowsDir is the flows directory.
	FlowsDir = "flows"
)

// Project represents a .brain/ project.
type Project struct {
	// Root is the project root directory (contains .brain/).
	Root string

	// ConfigDir is the .brain/ directory path.
	ConfigDir string

	// Config is the loaded project configuration.
	Config *Config
}

// FindProject searches for a .brain/ directory starting from the current directory
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
		brainDir := filepath.Join(dir, BrainDir)
		if st, err := os.Stat(brainDir); err == nil && st.IsDir() {
			cfg, err := LoadConfig(brainDir)
			if err != nil {
				return nil, fmt.Errorf("load config: %w", err)
			}

			return &Project{
				Root:      dir,
				ConfigDir: brainDir,
				Config:    cfg,
			}, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("no .brain/ directory found (searched from %s)", absStart)
		}
		dir = parent
	}
}

// InitProject creates a new .brain/ project in the specified directory.
func InitProject(root string, cfg *Config) (*Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("absolute path: %w", err)
	}

	brainDir := filepath.Join(absRoot, BrainDir)

	if _, err := os.Stat(brainDir); err == nil {
		return nil, fmt.Errorf(".brain/ directory already exists at %s", brainDir)
	}

	if err := os.MkdirAll(brainDir, 0755); err != nil {
		return nil, fmt.Errorf("create .brain directory: %w", err)
	}

	flowsDir := filepath.Join(brainDir, FlowsDir)
	if err := os.MkdirAll(flowsDir, 0755); err != nil {
		return nil, fmt.Errorf("create flows directory: %w", err)
	}

	if err := SaveConfig(brainDir, cfg); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}

	gitignorePath := filepath.Join(brainDir, ".gitignore")
	gitignoreContent := "# Brain sync cache (local only)\n.sync-cache\n"
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		return nil, fmt.Errorf("create .gitignore: %w", err)
	}

	return &Project{
		Root:      absRoot,
		ConfigDir: brainDir,
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

// Config represents the .brain/config.yaml structure.
type Config struct {
	Project ProjectMeta `yaml:"project"`

	// Server connection (optional)
	Server *ServerConfig `yaml:"server,omitempty"`

	// Plugin configuration
	Plugins *PluginsConfig `yaml:"plugins,omitempty"`

	// Plugin registries (overrides global registries when present)
	Registries []RegistryConfig `yaml:"registries,omitempty"`

	// Framework preset name (e.g., "nextjs")
	Preset string `yaml:"preset,omitempty"`

	// Local format preset definitions
	FormatPresets map[string]LocalFormatPreset `yaml:"format_presets,omitempty"`

	// File mappings
	Mappings []Mapping `yaml:"mappings,omitempty"`

	// Exclude patterns — files matching these are skipped during scanning
	Exclude []string `yaml:"exclude,omitempty"`

	// Flow hooks
	Hooks map[string][]string `yaml:"hooks,omitempty"`

	// Flow-specific settings
	Flows map[string]map[string]any `yaml:"flows,omitempty"`
}

// ProjectMeta contains project metadata.
type ProjectMeta struct {
	Name          string           `yaml:"name"`
	SourceLocale  model.LocaleID   `yaml:"source_locale"`
	TargetLocales []model.LocaleID `yaml:"target_locales,omitempty"`
}

// ServerConfig contains Bowrain Server connection details.
type ServerConfig struct {
	URL        string `yaml:"url"`
	ProjectID  string `yaml:"project_id"`
	Workspace  string `yaml:"workspace,omitempty"`
	ClaimToken string `yaml:"claim_token,omitempty"`
	// Auth token comes from brain auth login (stored separately)
}

// PluginsConfig specifies plugin dependencies.
type PluginsConfig struct {
	Framework []string `yaml:"framework,omitempty"` // e.g. ["okapi@1.48.0"]
	Presets   []string `yaml:"presets,omitempty"`   // e.g. ["okapi-presets@1.48.0"]
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

// Mapping defines a local - remote file mapping.
type Mapping struct {
	Local      string         `yaml:"local"`                 // Glob pattern
	Remote     string         `yaml:"remote"`                // Template with {path}, {filename}, {basename}
	Format     string         `yaml:"format"`                // Format ID (json, html, etc.)
	TargetPath string         `yaml:"target_path,omitempty"` // Target locale template (e.g. "locales/{locale}.json")
	Overrides  map[string]any `yaml:"overrides,omitempty"`   // Layer 3: per-mapping config overrides
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		Project: ProjectMeta{
			Name:          "my-project",
			SourceLocale:  "en",
			TargetLocales: []model.LocaleID{},
		},
		Mappings: []Mapping{},
		Hooks:    map[string][]string{},
		Flows:    map[string]map[string]any{},
	}
}
