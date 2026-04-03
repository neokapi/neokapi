package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseAPIVersion parses a version string like "v1", "v2" into an integer.
// The apiVersion in an envelope is simply "v{N}" — the kind carries all
// type information (format identity, namespace).
func ParseAPIVersion(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("apiVersion is required")
	}
	if !strings.HasPrefix(s, "v") {
		return 0, fmt.Errorf("apiVersion %q: expected format v{N}", s)
	}
	n, err := strconv.Atoi(s[1:])
	if err != nil || n < 1 {
		return 0, fmt.Errorf("apiVersion %q: invalid version number", s)
	}
	return n, nil
}

// FormatAPIVersion formats a version integer as a string.
func FormatAPIVersion(version int) string {
	return fmt.Sprintf("v%d", version)
}
