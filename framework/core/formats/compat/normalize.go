//go:build integration

package compat

import (
	"archive/zip"
	"bytes"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zipEntries extracts all files from a ZIP archive into a map of path → content.
func zipEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()

	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err, "opening ZIP")

	entries := make(map[string][]byte)
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err, "opening ZIP entry %s", f.Name)
		content, err := io.ReadAll(rc)
		require.NoError(t, err, "reading ZIP entry %s", f.Name)
		rc.Close()
		entries[f.Name] = content
	}
	return entries
}

// assertZIPEqual compares two ZIP archives entry-by-entry, comparing XML content
// of .xml and .rels files and binary content of everything else.
func assertZIPEqual(t *testing.T, label string, expected, actual []byte) {
	t.Helper()

	expectedEntries := zipEntries(t, expected)
	actualEntries := zipEntries(t, actual)

	// Compare entry lists.
	var expectedKeys, actualKeys []string
	for k := range expectedEntries {
		expectedKeys = append(expectedKeys, k)
	}
	for k := range actualEntries {
		actualKeys = append(actualKeys, k)
	}
	sort.Strings(expectedKeys)
	sort.Strings(actualKeys)

	if !assert.Equal(t, expectedKeys, actualKeys, "%s: ZIP entry lists differ", label) {
		return
	}

	// Compare each entry.
	for _, name := range expectedKeys {
		ec := expectedEntries[name]
		ac := actualEntries[name]

		if isXMLEntry(name) {
			// Normalize whitespace for XML comparison.
			assert.Equal(t, normalizeXMLWhitespace(ec), normalizeXMLWhitespace(ac),
				"%s: XML content differs in %s", label, name)
		} else {
			assert.Equal(t, ec, ac, "%s: binary content differs in %s", label, name)
		}
	}
}

// isXMLEntry returns true for files that should be compared as XML.
func isXMLEntry(name string) bool {
	return strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels")
}

// normalizeXMLWhitespace trims leading/trailing whitespace from each line
// and collapses empty lines — a rough normalization for comparing XML across
// implementations that may differ in formatting.
func normalizeXMLWhitespace(data []byte) string {
	lines := strings.Split(string(data), "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return strings.Join(result, "\n")
}
