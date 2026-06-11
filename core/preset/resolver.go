package preset

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/config"
	"gopkg.in/yaml.v3"
)

// SchemaValidator validates filter parameters against a schema.
// This interface is satisfied by loader.SchemaRegistry.
type SchemaValidator interface {
	ValidateParams(filterID string, params map[string]any) error
}

// LocalFormatPreset represents a project-local format preset definition.
type LocalFormatPreset struct {
	Description string
	Base        string
	Config      map[string]any
}

// ConfigResolver resolves format + preset + overrides into final configuration.
type ConfigResolver struct {
	presets *PresetRegistry
	schemas SchemaValidator
}

// NewConfigResolver creates a new ConfigResolver.
func NewConfigResolver(presets *PresetRegistry, schemas SchemaValidator) *ConfigResolver {
	return &ConfigResolver{
		presets: presets,
		schemas: schemas,
	}
}

// ResolveFormatConfig resolves a format reference with optional preset and overrides
// into a final merged configuration map.
//
// The presetName can be:
//   - A preset name (e.g. "wellFormed") — looked up in local presets, then registry
//   - A file path (e.g. "./my-config.yaml") — loaded as YAML/JSON config
//
// File paths are detected by the presence of path separators (/ or \) or
// a .yaml/.yml/.json extension.
//
// Resolution steps:
// 1. Get preset config (from file, local preset, or registry)
// 2. Deep-merge: preset config → overrides
// 3. Validate the final config against the format's schema
func (r *ConfigResolver) ResolveFormatConfig(
	formatName string,
	presetName string,
	localPresets map[string]LocalFormatPreset,
	overrides map[string]any,
) (map[string]any, error) {
	var presetConfig map[string]any

	if presetName != "" {
		if IsConfigFilePath(presetName) {
			cfg, err := LoadConfigFile(presetName)
			if err != nil {
				return nil, fmt.Errorf("load config file %q: %w", presetName, err)
			}
			presetConfig = cfg
		} else if lp, ok := localPresets[presetName]; ok {
			// Check local presets first
			presetConfig = lp.Config
		} else {
			// Check registry (bridge configs, plugin presets)
			p := r.presets.GetFormatPreset(formatName, presetName)
			if p == nil {
				return nil, fmt.Errorf("preset %q not found for format %q", presetName, formatName)
			}
			presetConfig = p.Config
		}
	}

	// Merge: preset → overrides (no separate defaults layer for now)
	merged := MergeConfig(nil, presetConfig, overrides)

	// Validate against schema if available
	if r.schemas != nil && len(merged) > 0 {
		if err := r.schemas.ValidateParams(formatName, merged); err != nil {
			suffix := presetName
			if suffix == "" {
				suffix = "overrides"
			}
			return nil, fmt.Errorf("config for %s:%s: %w", formatName, suffix, err)
		}
	}

	return merged, nil
}

// ValidatePreset validates a preset's config against its format's schema.
func (r *ConfigResolver) ValidatePreset(p *FormatPreset) error {
	if r.schemas == nil {
		return nil
	}
	return r.schemas.ValidateParams(p.Format, p.Config)
}

// ValidateOverrides validates user overrides against a format's schema.
func (r *ConfigResolver) ValidateOverrides(formatID string, overrides map[string]any) error {
	if r.schemas == nil {
		return nil
	}
	return r.schemas.ValidateParams(formatID, overrides)
}

// ValidateAllPresets validates all local presets and returns all errors.
func (r *ConfigResolver) ValidateAllPresets(
	localPresets map[string]LocalFormatPreset,
	formatFilter string,
) []string {
	var errors []string

	for name, lp := range localPresets {
		// Determine the format: either Base or the preset name itself
		format := lp.Base
		if format == "" {
			format = name
		}
		if formatFilter != "" && format != formatFilter {
			continue
		}
		if r.schemas != nil {
			if err := r.schemas.ValidateParams(format, lp.Config); err != nil {
				errors = append(errors, fmt.Sprintf("preset %q: %s", name, stripPrefix(err.Error())))
			}
		}
	}

	return errors
}

// IsConfigFilePath reports whether s looks like a file path rather than a
// preset name. It checks for path separators (/ or \) or common config
// file extensions (.yaml, .yml, .json).
func IsConfigFilePath(s string) bool {
	if strings.ContainsAny(s, "/\\") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(s))
	return ext == ".yaml" || ext == ".yml" || ext == ".json"
}

// LoadConfigFile reads a YAML or JSON config file and returns the parameters
// as a map. If the file contains a k8s-style envelope (apiVersion + kind),
// it is parsed as an Envelope and the spec is returned. Any registered
// migrations are applied automatically. Otherwise, the file is parsed as a
// bare parameter map for backward compatibility.
func LoadConfigFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(path))

	// Try envelope parsing first: probe for apiVersion
	var probe struct {
		APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	}
	if ext == ".json" {
		_ = json.Unmarshal(data, &probe)
	} else {
		_ = yaml.Unmarshal(data, &probe)
	}

	if probe.APIVersion != "" {
		// Parse as envelope
		env, err := config.Parse(data, ext)
		if err != nil {
			return nil, fmt.Errorf("parse enveloped config: %w", err)
		}
		// Apply migrations
		if err := config.DefaultMigrations.Upgrade(env); err != nil {
			return nil, fmt.Errorf("migrate config: %w", err)
		}
		return env.Spec, nil
	}

	// Fallback: bare parameter map
	var cfg map[string]any
	if ext == ".json" {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse YAML: %w", err)
		}
	}

	return cfg, nil
}

// LoadEnvelopedConfigFile reads a config file that must be in envelope format.
// Unlike LoadConfigFile, this returns an error if the file lacks an apiVersion.
func LoadEnvelopedConfigFile(path string) (*config.Envelope, error) {
	return config.Load(path)
}

// TransformConfigSpec transforms a config spec from one kind to another
// using the global transform registry. Returns the original spec unchanged if
// no transform is registered for the given (from, to) pair.
func TransformConfigSpec(fromKind, toKind config.Kind, spec map[string]any) (map[string]any, error) {
	if !config.DefaultTransforms.Has(fromKind, toKind) {
		return spec, nil
	}
	return config.DefaultTransforms.Transform(fromKind, toKind, spec)
}

// stripPrefix removes common error prefixes for cleaner messages.
func stripPrefix(s string) string {
	if _, after, ok := strings.Cut(s, ":\n"); ok {
		return strings.TrimSpace(after)
	}
	return s
}
