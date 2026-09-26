package packs

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	coreprofile "github.com/neokapi/neokapi/core/profile"
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

// Load loads a starter pack by name: the voice, carrying the pack's terms.
func Load(name string) (*coreprofile.VoiceProfile, error) {
	data, err := packsFS.ReadFile(name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("reading pack %q: %w", name, err)
	}
	profile, err := coreprofile.LoadProfileYAML(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("pack %q: %w", name, err)
	}
	// A pack's terms apply where the pack is bound, beside the project's own,
	// and name the pack as where they come from.
	carried := profile.CarriedTerms()
	profile.Carry(From(name), carried.Rules)
	return profile, nil
}

// From names a pack as where its terms come from: "pack technical-docs".
func From(name string) string { return "pack " + name }

// LoadAll loads all starter packs.
func LoadAll() ([]*coreprofile.VoiceProfile, error) {
	names, err := List()
	if err != nil {
		return nil, err
	}
	var profiles []*coreprofile.VoiceProfile
	for _, name := range names {
		p, err := Load(name)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}
