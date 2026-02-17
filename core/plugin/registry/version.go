package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// VersionFile tracks metadata about an installed plugin.
type VersionFile struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	InstallType string `json:"install_type"`
	InstalledAt string `json:"installed_at"`
	Checksum    string `json:"checksum"`
}

// InstalledVersion pairs a VersionFile with its directory on disk.
type InstalledVersion struct {
	VersionFile
	Dir string
}

// VersionedPluginDir returns the directory for a specific plugin version:
// {baseDir}/{name}/{version}
func VersionedPluginDir(baseDir, name, version string) string {
	return filepath.Join(baseDir, name, version)
}

// WriteVersionFile writes a version file into the versioned plugin directory
// {baseDir}/{name}/{version}/version.json.
func WriteVersionFile(baseDir, name, version string, vf *VersionFile) error {
	dir := VersionedPluginDir(baseDir, name, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating plugin directory: %w", err)
	}
	path := filepath.Join(dir, "version.json")
	data, err := json.MarshalIndent(vf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling version file: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing version file: %w", err)
	}
	return nil
}

// ReadVersionFile reads the version file from {baseDir}/{name}/{version}/version.json.
func ReadVersionFile(baseDir, name, version string) (*VersionFile, error) {
	path := filepath.Join(VersionedPluginDir(baseDir, name, version), "version.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading version file: %w", err)
	}
	var vf VersionFile
	if err := json.Unmarshal(data, &vf); err != nil {
		return nil, fmt.Errorf("parsing version file: %w", err)
	}
	return &vf, nil
}

// ListInstalledVersions returns all installed versions for the named plugin.
// It scans {baseDir}/{name}/{version}/version.json.
func ListInstalledVersions(baseDir, name string) ([]InstalledVersion, error) {
	pluginDir := filepath.Join(baseDir, name)
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading plugin directory %s: %w", name, err)
	}

	var result []InstalledVersion
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		version := e.Name()
		vf, err := ReadVersionFile(baseDir, name, version)
		if err != nil {
			continue // skip directories without valid version.json
		}
		result = append(result, InstalledVersion{
			VersionFile: *vf,
			Dir:         VersionedPluginDir(baseDir, name, version),
		})
	}
	return result, nil
}

// ListAllInstalled does a two-level directory scan and returns all installed
// versions grouped by plugin name.
func ListAllInstalled(baseDir string) (map[string][]InstalledVersion, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading plugin base directory: %w", err)
	}

	result := make(map[string][]InstalledVersion)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		versions, err := ListInstalledVersions(baseDir, name)
		if err != nil {
			continue
		}
		if len(versions) > 0 {
			result[name] = versions
		}
	}
	return result, nil
}

// LatestInstalledVersion returns the installed version with the highest
// semantic version for the named plugin.
func LatestInstalledVersion(baseDir, name string) (*InstalledVersion, error) {
	versions, err := ListInstalledVersions(baseDir, name)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no installed versions found for %q", name)
	}

	best := &versions[0]
	for i := 1; i < len(versions); i++ {
		if CompareSemver(versions[i].Version, best.Version) > 0 {
			best = &versions[i]
		}
	}
	return best, nil
}
