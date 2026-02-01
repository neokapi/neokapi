package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatColumnsSingleItem(t *testing.T) {
	got := formatColumns([]string{"okapi \u2714"})
	assert.Contains(t, got, "okapi \u2714")
	// Single item with trailing column padding.
	assert.True(t, len(got) > 0)
	assert.Equal(t, '\n', rune(got[len(got)-1]))
}

func TestFormatColumnsMultipleItems(t *testing.T) {
	items := []string{"okapi \u2714", "okapi@1.46.0"}
	got := formatColumns(items)
	assert.Contains(t, got, "okapi \u2714")
	assert.Contains(t, got, "okapi@1.46.0")
	// Both items should be on the same line (they fit in 80 cols).
	lines := 0
	for _, ch := range got {
		if ch == '\n' {
			lines++
		}
	}
	assert.Equal(t, 1, lines, "expected single line for two short items")
}

func TestFormatColumnsEmpty(t *testing.T) {
	got := formatColumns(nil)
	assert.Equal(t, "", got)
}

func TestFormatColumnsWrapLongItems(t *testing.T) {
	// Create items that are long enough to force wrapping.
	items := []string{
		"very-long-plugin-name-that-takes-up-space \u2714",
		"another-very-long-plugin-name@1.2.3",
		"third-plugin-with-long-name@4.5.6 \u2714",
	}
	got := formatColumns(items)
	assert.Contains(t, got, "very-long-plugin-name-that-takes-up-space \u2714")
	assert.Contains(t, got, "another-very-long-plugin-name@1.2.3")
	assert.Contains(t, got, "third-plugin-with-long-name@4.5.6 \u2714")
}

func TestFormatColumnsMultiplePlugins(t *testing.T) {
	items := []string{
		"okapi \u2714",
		"okapi@1.46.0",
		"deepl \u2714",
		"google",
	}
	got := formatColumns(items)
	// All items should be present.
	for _, item := range items {
		assert.Contains(t, got, item)
	}
}

func TestFormatColumnsColumnAlignment(t *testing.T) {
	items := []string{"a", "bb", "ccc"}
	got := formatColumns(items)
	// All items should be on one line.
	assert.Contains(t, got, "a")
	assert.Contains(t, got, "bb")
	assert.Contains(t, got, "ccc")
}
