package loader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BridgeDescriptor is the JSON schema for a *.bridge.json plugin descriptor.
type BridgeDescriptor struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	JAR            string   `json:"jar"`
	Java           string   `json:"java"`
	JVMArgs        []string `json:"jvm_args"`
	StartupTimeout string   `json:"startup_timeout"`
	CommandTimeout string   `json:"command_timeout"`
}

// ParsedBridgeDescriptor is a validated and resolved descriptor.
type ParsedBridgeDescriptor struct {
	BridgeDescriptor
	SourcePath             string
	ResolvedJARPath        string
	ResolvedStartupTimeout time.Duration
	ResolvedCommandTimeout time.Duration
}

// ParseBridgeDescriptor reads and validates a bridge descriptor JSON file.
// pluginDir is used to resolve relative JAR paths.
func ParseBridgeDescriptor(path, pluginDir string) (*ParsedBridgeDescriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading descriptor %s: %w", path, err)
	}

	var desc BridgeDescriptor
	if err := json.Unmarshal(data, &desc); err != nil {
		return nil, fmt.Errorf("parsing descriptor %s: %w", path, err)
	}

	if desc.Type != "bridge" {
		return nil, fmt.Errorf("descriptor %s: type must be \"bridge\", got %q", path, desc.Type)
	}
	if desc.JAR == "" {
		return nil, fmt.Errorf("descriptor %s: jar field is required", path)
	}
	if desc.Name == "" {
		return nil, fmt.Errorf("descriptor %s: name field is required", path)
	}

	// Apply defaults.
	if desc.Java == "" {
		desc.Java = "java"
	}
	if desc.StartupTimeout == "" {
		desc.StartupTimeout = "30s"
	}
	if desc.CommandTimeout == "" {
		desc.CommandTimeout = "60s"
	}

	// Resolve JAR path relative to plugin directory.
	jarPath := desc.JAR
	if !filepath.IsAbs(jarPath) {
		jarPath = filepath.Join(pluginDir, jarPath)
	}
	if _, err := os.Stat(jarPath); err != nil {
		return nil, fmt.Errorf("descriptor %s: jar file not found: %s", path, jarPath)
	}

	startupTimeout, err := time.ParseDuration(desc.StartupTimeout)
	if err != nil {
		return nil, fmt.Errorf("descriptor %s: invalid startup_timeout %q: %w", path, desc.StartupTimeout, err)
	}

	commandTimeout, err := time.ParseDuration(desc.CommandTimeout)
	if err != nil {
		return nil, fmt.Errorf("descriptor %s: invalid command_timeout %q: %w", path, desc.CommandTimeout, err)
	}

	return &ParsedBridgeDescriptor{
		BridgeDescriptor:       desc,
		SourcePath:             path,
		ResolvedJARPath:        jarPath,
		ResolvedStartupTimeout: startupTimeout,
		ResolvedCommandTimeout: commandTimeout,
	}, nil
}
