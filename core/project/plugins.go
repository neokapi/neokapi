package project

import (
	"strconv"
	"strings"
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

// --- Version constraint matching ---

// MatchVersionConstraint checks whether an installed version satisfies a
// version constraint string. Supported constraint forms:
//
//   - "" or "*"    → any version satisfies
//   - "^1.2.3"    → compatible: same major version, installed >= constraint
//   - ">=1.2.3"   → installed >= constraint
//   - "1.2.3"     → exact match
func MatchVersionConstraint(constraint, installed string) bool {
	if constraint == "" || constraint == "*" {
		return true
	}
	if strings.HasPrefix(constraint, "^") {
		return compatibleVersion(strings.TrimPrefix(constraint, "^"), installed)
	}
	if strings.HasPrefix(constraint, ">=") {
		return compareVersions(installed, strings.TrimPrefix(constraint, ">=")) >= 0
	}
	// Exact match.
	return normalizeVersion(installed) == normalizeVersion(constraint)
}

// compatibleVersion checks semver compatibility: same major version and
// installed >= required.
func compatibleVersion(required, installed string) bool {
	rParts := parseVersion(required)
	iParts := parseVersion(installed)
	if rParts[0] != iParts[0] {
		return false // major version mismatch
	}
	return compareVersions(installed, required) >= 0
}

// compareVersions returns -1, 0, or 1 comparing two version strings.
func compareVersions(a, b string) int {
	ap := parseVersion(a)
	bp := parseVersion(b)
	for i := 0; i < 3; i++ {
		if ap[i] < bp[i] {
			return -1
		}
		if ap[i] > bp[i] {
			return 1
		}
	}
	return 0
}

// parseVersion splits "1.2.3" into [1, 2, 3]. Missing parts default to 0.
func parseVersion(v string) [3]int {
	v = normalizeVersion(v)
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i := 0; i < len(parts) && i < 3; i++ {
		result[i], _ = strconv.Atoi(parts[i])
	}
	return result
}

// normalizeVersion strips leading "v" and any pre-release/build metadata.
func normalizeVersion(v string) string {
	v = strings.TrimPrefix(v, "v")
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		v = v[:idx]
	}
	if idx := strings.IndexByte(v, '+'); idx >= 0 {
		v = v[:idx]
	}
	return v
}
