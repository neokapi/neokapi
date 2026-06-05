package project

import (
	"github.com/neokapi/neokapi/core/registry"
)

// InstalledPlugin describes a plugin that is available in the local environment.
// This is the minimal interface needed for plugin checking — callers map from
// their concrete plugin info types (loader.PluginInfo, etc.) to this struct.
type InstalledPlugin struct {
	Name             string
	Version          string
	FrameworkVersion string
}

// PluginStatus reports whether a project's plugin requirements are satisfied
// by the locally installed plugins.
type PluginStatus struct {
	Satisfied bool          `json:"satisfied"`
	Issues    []PluginIssue `json:"issues,omitempty"`
}

// PluginIssue describes a single plugin requirement that is not met.
type PluginIssue struct {
	Plugin           string `json:"plugin"`
	Type             string `json:"type"` // "missing" or "version_mismatch"
	Required         string `json:"required,omitempty"`
	InstalledVersion string `json:"installed_version,omitempty"`
}

// CheckPlugins verifies that the project's declared plugin requirements are
// satisfied by the given set of installed plugins. It checks both plugin
// presence and version constraints (Version and FrameworkVersion).
//
// When a plugin name appears multiple times in installed (e.g. multiple
// versions of the same plugin), the highest version is used for matching.
//
// Projects with no declared plugins always return Satisfied: true.
func CheckPlugins(proj *KapiProject, installed []InstalledPlugin) *PluginStatus {
	if len(proj.Plugins) == 0 {
		return &PluginStatus{Satisfied: true}
	}

	// Build lookup: name → best installed plugin (highest version).
	byName := make(map[string]InstalledPlugin, len(installed))
	for _, p := range installed {
		if existing, ok := byName[p.Name]; !ok || p.Version > existing.Version {
			byName[p.Name] = p
		}
	}

	var issues []PluginIssue
	for name, spec := range proj.Plugins {
		info, ok := byName[name]
		if !ok {
			issues = append(issues, PluginIssue{
				Plugin:   name,
				Type:     "missing",
				Required: spec.Version,
			})
			continue
		}
		// Check plugin version constraint.
		if spec.Version != "" && !MatchVersionConstraint(spec.Version, info.Version) {
			issues = append(issues, PluginIssue{
				Plugin:           name,
				Type:             "version_mismatch",
				Required:         spec.Version,
				InstalledVersion: info.Version,
			})
			continue
		}
		// Check framework version constraint.
		if spec.FrameworkVersion != "" && !MatchVersionConstraint(spec.FrameworkVersion, info.FrameworkVersion) {
			issues = append(issues, PluginIssue{
				Plugin:           name,
				Type:             "version_mismatch",
				Required:         spec.FrameworkVersion,
				InstalledVersion: info.FrameworkVersion,
			})
		}
	}

	return &PluginStatus{
		Satisfied: len(issues) == 0,
		Issues:    issues,
	}
}

// PopulatePlugins adds entries to proj.Plugins for each installed plugin
// that is not already declared. Uses an empty PluginSpec (no version
// constraint) so the project is immediately compatible.
func PopulatePlugins(proj *KapiProject, installed []InstalledPlugin) {
	seen := make(map[string]bool)
	for _, p := range installed {
		if seen[p.Name] {
			continue
		}
		seen[p.Name] = true
		if proj.Plugins == nil {
			proj.Plugins = make(map[string]PluginSpec)
		}
		if _, exists := proj.Plugins[p.Name]; !exists {
			proj.Plugins[p.Name] = PluginSpec{}
		}
	}
}

// AllowedFormatSources returns the set of format source names that a project
// allows. This always includes "built-in". If the project declares plugins,
// their names are added (e.g., "okapi-bridge"). If the project has no plugins
// section, only "built-in" is returned.
//
// Use this with FormatRegistry.DetectByExtensionForSources to restrict
// auto-detection to formats the project can actually process.
func AllowedFormatSources(proj *KapiProject) []string {
	sources := []string{registry.SourceBuiltIn}
	for name := range proj.Plugins {
		sources = append(sources, name)
	}
	return sources
}

// MatchVersionConstraint lives in constraint.go, alongside the shared
// version-constraint grammar it uses.
