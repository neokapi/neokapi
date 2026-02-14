package kapiproject

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gokapi/gokapi/core/model"
)

const (
	// KapiDir is the project directory name.
	KapiDir = ".kapi"

	// ConfigFile is the project configuration file.
	ConfigFile = "config.yaml"

	// StateFile is the sync state file (gitignored).
	StateFile = ".state.json"

	// FlowsDir is the flows directory.
	FlowsDir = "flows"
)

// Project represents a .kapi/ project.
type Project struct {
	// Root is the project root directory (contains .kapi/).
	Root string

	// KapiDir is the .kapi/ directory path.
	KapiDir string

	// Config is the loaded project configuration.
	Config *Config

	// State is the sync state (loaded lazily).
	state *State
}

// FindProject searches for a .kapi/ directory starting from the current directory
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
		kapiDir := filepath.Join(dir, KapiDir)
		if st, err := os.Stat(kapiDir); err == nil && st.IsDir() {
			// Found .kapi/ directory
			cfg, err := LoadConfig(kapiDir)
			if err != nil {
				return nil, fmt.Errorf("load config: %w", err)
			}

			return &Project{
				Root:    dir,
				KapiDir: kapiDir,
				Config:  cfg,
			}, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return nil, fmt.Errorf("no .kapi/ directory found (searched from %s)", absStart)
		}
		dir = parent
	}
}

// InitProject creates a new .kapi/ project in the specified directory.
func InitProject(root string, cfg *Config) (*Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("absolute path: %w", err)
	}

	kapiDir := filepath.Join(absRoot, KapiDir)

	// Check if .kapi/ already exists
	if _, err := os.Stat(kapiDir); err == nil {
		return nil, fmt.Errorf(".kapi/ directory already exists at %s", kapiDir)
	}

	// Create .kapi/ directory
	if err := os.MkdirAll(kapiDir, 0755); err != nil {
		return nil, fmt.Errorf("create .kapi directory: %w", err)
	}

	// Create flows/ subdirectory
	flowsDir := filepath.Join(kapiDir, FlowsDir)
	if err := os.MkdirAll(flowsDir, 0755); err != nil {
		return nil, fmt.Errorf("create flows directory: %w", err)
	}

	// Save config
	if err := SaveConfig(kapiDir, cfg); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}

	// Create .gitignore entry for .state.json
	gitignorePath := filepath.Join(kapiDir, ".gitignore")
	gitignoreContent := "# Kapi sync state (local only)\n.state.json\n"
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		return nil, fmt.Errorf("create .gitignore: %w", err)
	}

	return &Project{
		Root:    absRoot,
		KapiDir: kapiDir,
		Config:  cfg,
	}, nil
}

// LoadState loads the sync state (lazy-loaded).
func (p *Project) LoadState(ctx context.Context) (*State, error) {
	if p.state != nil {
		return p.state, nil
	}

	state, err := LoadState(p.KapiDir)
	if err != nil {
		return nil, err
	}

	p.state = state
	return state, nil
}

// SaveState saves the sync state.
func (p *Project) SaveState(ctx context.Context, state *State) error {
	if err := SaveState(p.KapiDir, state); err != nil {
		return err
	}
	p.state = state
	return nil
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

// FlowsDir returns the flows directory path.
func (p *Project) FlowsDirPath() string {
	return filepath.Join(p.KapiDir, FlowsDir)
}

// Config represents the .kapi/config.yaml structure.
type Config struct {
	Project ProjectMeta `yaml:"project"`

	// Server connection (optional)
	Server *ServerConfig `yaml:"server,omitempty"`

	// File mappings
	Mappings []Mapping `yaml:"mappings,omitempty"`

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
	URL       string `yaml:"url"`
	ProjectID string `yaml:"project_id"`
	// Auth token comes from kapi auth login (stored separately)
}

// Mapping defines a local ↔ remote file mapping.
type Mapping struct {
	Local  string `yaml:"local"`  // Glob pattern
	Remote string `yaml:"remote"` // Template with {path}, {filename}, {basename}
	Format string `yaml:"format"` // Format ID (json, html, etc.)
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		Project: ProjectMeta{
			Name:          "my-project",
			SourceLocale:  "en-US",
			TargetLocales: []model.LocaleID{},
		},
		Mappings: []Mapping{},
		Hooks:    map[string][]string{},
		Flows:    map[string]map[string]any{},
	}
}
