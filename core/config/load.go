package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads and parses a config file from disk.
// The file must contain a valid envelope with apiVersion and kind.
func Load(path string) (*Envelope, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(path))
	return Parse(data, ext)
}

// Parse deserializes config data into an Envelope.
// The ext parameter determines the format (".json" for JSON, anything else for YAML).
// The data must contain apiVersion and kind fields.
func Parse(data []byte, ext string) (*Envelope, error) {
	// First pass: probe for required fields
	var probe struct {
		APIVersion string `json:"apiVersion" yaml:"apiVersion"`
		Kind       Kind   `json:"kind"       yaml:"kind"`
	}

	if err := unmarshal(data, ext, &probe); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if probe.APIVersion == "" {
		return nil, fmt.Errorf("config is missing required field \"apiVersion\"")
	}
	if probe.Kind == "" {
		return nil, fmt.Errorf("config is missing required field \"kind\"")
	}

	// Validate apiVersion format (v1, v2, etc.)
	if _, err := ParseAPIVersion(probe.APIVersion); err != nil {
		return nil, err
	}

	// Validate kind
	if !probe.Kind.IsValid() {
		return nil, fmt.Errorf("unknown kind %q", probe.Kind)
	}

	// Second pass: full unmarshal
	var env Envelope
	if err := unmarshal(data, ext, &env); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &env, nil
}

func unmarshal(data []byte, ext string, v any) error {
	if ext == ".json" {
		return json.Unmarshal(data, v)
	}
	return yaml.Unmarshal(data, v)
}
