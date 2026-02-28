package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// AddGlobalRegistry adds a named registry to the global config file.
// Returns an error if a registry with the same name already exists.
// Channels is optional — pass nil for no channels.
func AddGlobalRegistry(name, url string, channels []string) error {
	path := GlobalConfigFilePath()

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	_ = v.ReadInConfig()

	entries := parseRegistryEntries(v.Get("registries"))
	for _, e := range entries {
		if e.Name == name {
			return fmt.Errorf("registry %q already exists", name)
		}
	}

	entries = append(entries, RegistryEntry{Name: name, URL: url, Channels: channels})
	v.Set("registries", registryEntriesToSlice(entries))

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return v.WriteConfigAs(path)
}

// RemoveGlobalRegistry removes a named registry from the global config file.
// Returns an error if the registry is not found.
func RemoveGlobalRegistry(name string) error {
	path := GlobalConfigFilePath()

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	_ = v.ReadInConfig()

	entries := parseRegistryEntries(v.Get("registries"))
	var filtered []RegistryEntry
	found := false
	for _, e := range entries {
		if e.Name == name {
			found = true
			continue
		}
		filtered = append(filtered, e)
	}
	if !found {
		return fmt.Errorf("registry %q not found", name)
	}

	v.Set("registries", registryEntriesToSlice(filtered))
	return v.WriteConfigAs(path)
}

// ListGlobalRegistries returns the registries from the global config file.
// If none are configured, returns the default official registry.
func ListGlobalRegistries() ([]RegistryEntry, error) {
	path := GlobalConfigFilePath()

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	_ = v.ReadInConfig()

	entries := parseRegistryEntries(v.Get("registries"))
	if len(entries) > 0 {
		return entries, nil
	}

	// Fallback to plugins.registry or default URL.
	url := v.GetString("plugins.registry")
	if url == "" {
		url = DefaultRegistryURL
	}
	return []RegistryEntry{{Name: "default", URL: url}}, nil
}

// registryEntriesToSlice converts []RegistryEntry to []map[string]any for Viper serialization.
func registryEntriesToSlice(entries []RegistryEntry) []map[string]any {
	result := make([]map[string]any, len(entries))
	for i, e := range entries {
		m := map[string]any{
			"name": e.Name,
			"url":  e.URL,
		}
		if len(e.Channels) > 0 {
			m["channels"] = e.Channels
		}
		result[i] = m
	}
	return result
}
