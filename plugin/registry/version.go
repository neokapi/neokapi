package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VersionFile tracks metadata about an installed plugin.
type VersionFile struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	InstallType string `json:"install_type"`
	InstalledAt string `json:"installed_at"`
	Checksum    string `json:"checksum"`
}

// WriteVersionFile writes a version file for the named plugin into pluginDir.
func WriteVersionFile(pluginDir, name string, vf *VersionFile) error {
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("creating plugin directory: %w", err)
	}
	path := filepath.Join(pluginDir, name+".version.json")
	data, err := json.MarshalIndent(vf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling version file: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing version file: %w", err)
	}
	return nil
}

// ReadVersionFile reads the version file for the named plugin from pluginDir.
func ReadVersionFile(pluginDir, name string) (*VersionFile, error) {
	path := filepath.Join(pluginDir, name+".version.json")
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

// ListVersionFiles returns all version files found in the plugin directory.
func ListVersionFiles(pluginDir string) ([]*VersionFile, error) {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading plugin directory: %w", err)
	}

	var result []*VersionFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".version.json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".version.json")
		vf, err := ReadVersionFile(pluginDir, name)
		if err != nil {
			continue // skip malformed version files
		}
		result = append(result, vf)
	}
	return result, nil
}
