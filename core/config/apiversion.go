package config

import (
	"fmt"
	"strconv"
	"strings"
)

// APIVersion represents a parsed apiVersion string.
// Format: "{namespace}/{resource}-v{N}" (e.g., "gokapi/html-v1").
type APIVersion struct {
	Namespace string // "gokapi" or "okapi"
	Resource  string // "html", "json", "project", "flow", "preset"
	Version   int    // 1, 2, ...
}

// String returns the canonical apiVersion string.
func (v APIVersion) String() string {
	return fmt.Sprintf("%s/%s-v%d", v.Namespace, v.Resource, v.Version)
}

// ParseAPIVersion parses an apiVersion string into its components.
// Expected format: "{namespace}/{resource}-v{N}".
func ParseAPIVersion(s string) (APIVersion, error) {
	if s == "" {
		return APIVersion{}, fmt.Errorf("apiVersion is required")
	}

	slash := strings.IndexByte(s, '/')
	if slash < 0 {
		return APIVersion{}, fmt.Errorf("apiVersion %q: missing namespace (expected {namespace}/{resource}-v{N})", s)
	}

	ns := s[:slash]
	rest := s[slash+1:]

	if ns == "" {
		return APIVersion{}, fmt.Errorf("apiVersion %q: empty namespace", s)
	}
	if rest == "" {
		return APIVersion{}, fmt.Errorf("apiVersion %q: missing resource-version", s)
	}

	// Find the last "-v" followed by digits
	dashV := strings.LastIndex(rest, "-v")
	if dashV < 0 {
		return APIVersion{}, fmt.Errorf("apiVersion %q: missing version suffix (expected -v{N})", s)
	}

	resource := rest[:dashV]
	versionStr := rest[dashV+2:]

	if resource == "" {
		return APIVersion{}, fmt.Errorf("apiVersion %q: empty resource name", s)
	}

	version, err := strconv.Atoi(versionStr)
	if err != nil || version < 1 {
		return APIVersion{}, fmt.Errorf("apiVersion %q: invalid version number %q", s, versionStr)
	}

	return APIVersion{
		Namespace: ns,
		Resource:  resource,
		Version:   version,
	}, nil
}

// ResourceKey returns the "{namespace}/{resource}" portion without the version.
func (v APIVersion) ResourceKey() string {
	return fmt.Sprintf("%s/%s", v.Namespace, v.Resource)
}
