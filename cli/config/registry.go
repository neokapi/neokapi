package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// readConfigOrInit reads the config file, returning an error only when the
// file exists but cannot be read or parsed (not-found is silently ignored).
func readConfigOrInit(v *viper.Viper) error {
	if err := v.ReadInConfig(); err != nil {
		// A missing config file is fine — we're about to create it. With an
		// explicit SetConfigFile, viper surfaces that as an os not-exist error
		// rather than ConfigFileNotFoundError, so check both. Any other error
		// (malformed YAML, permission denied) is real and must not be ignored.
		var nf viper.ConfigFileNotFoundError
		if !errors.As(err, &nf) && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("reading config: %w", err)
		}
	}
	return nil
}

// AddGlobalRegistry adds a named registry to the global config file.
// Returns an error if a registry with the same name already exists.
// Channels is optional — pass nil for no channels.
func AddGlobalRegistry(name, url string, channels []string) error {
	path := GlobalConfigFilePath()

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := readConfigOrInit(v); err != nil {
		return err
	}

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
	if err := readConfigOrInit(v); err != nil {
		return err
	}

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
	if err := readConfigOrInit(v); err != nil {
		return nil, err
	}

	entries := parseRegistryEntries(v.Get("registries"))
	if len(entries) > 0 {
		return entries, nil
	}

	// Fallback to plugins.registry or default URL.
	url := v.GetString(KeyPluginsRegistry)
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
