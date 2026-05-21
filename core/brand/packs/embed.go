package packs

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/brand"
)

//go:embed *.yaml
var packsFS embed.FS

// List returns the names of all available starter packs.
func List() ([]string, error) {
	var names []string
	entries, err := fs.ReadDir(packsFS, ".")
	if err != nil {
		return nil, fmt.Errorf("reading packs directory: %w", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".yaml" {
			name := strings.TrimSuffix(e.Name(), ".yaml")
			names = append(names, name)
		}
	}
	return names, nil
}

// Load loads a starter pack by name and returns a VoiceProfile.
func Load(name string) (*brand.VoiceProfile, error) {
	data, err := packsFS.ReadFile(name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("reading pack %q: %w", name, err)
	}
	profile, err := brand.LoadProfileYAML(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("pack %q: %w", name, err)
	}
	return profile, nil
}

// LoadAll loads all starter packs.
func LoadAll() ([]*brand.VoiceProfile, error) {
	names, err := List()
	if err != nil {
		return nil, err
	}
	var profiles []*brand.VoiceProfile
	for _, name := range names {
		p, err := Load(name)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}
