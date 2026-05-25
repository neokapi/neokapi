package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewApp(t *testing.T) {
	app := NewApp()
	assert.NotNil(t, app)
	assert.NotNil(t, app.formatReg)
}

func TestListFormats(t *testing.T) {
	app := NewApp()
	fmts := app.ListFormats()

	assert.True(t, len(fmts) >= 14, "expected at least 14 formats, got %d", len(fmts))

	// Verify sorted
	for i := 1; i < len(fmts); i++ {
		assert.True(t, fmts[i-1].Name < fmts[i].Name,
			"formats not sorted: %s >= %s", fmts[i-1].Name, fmts[i].Name)
	}

	// Check a known format
	found := false
	for _, f := range fmts {
		if f.Name == "html" {
			found = true
			assert.True(t, f.HasReader)
			assert.True(t, f.HasWriter)
		}
	}
	assert.True(t, found, "html format not found")
}

func TestListTools(t *testing.T) {
	app := NewApp()
	tools := app.ListTools()
	// The exact count drifts as tools are added/removed; assert a floor and
	// the specific tools below rather than a brittle hardcoded number.
	assert.GreaterOrEqual(t, len(tools), 40, "expected at least 40 tools")

	names := make(map[string]bool)
	for _, tl := range tools {
		names[tl.Name] = true
	}
	assert.True(t, names["ai-translate"])
	assert.True(t, names["ai-qa"])
	assert.True(t, names["pseudo-translate"])
	assert.True(t, names["word-count"])
	assert.True(t, names["char-count"])
	assert.True(t, names["search-replace"])
	assert.True(t, names["tag-protect"])
	assert.True(t, names["term-check"])
	assert.True(t, names["segmentation"])
	assert.True(t, names["tm-leverage"])
	assert.True(t, names["qa-check"])

	// Verify category is set on all tools.
	for _, tl := range tools {
		assert.NotEmpty(t, tl.Category, "tool %q should have a category", tl.Name)
	}
}

func TestListPlugins(t *testing.T) {
	app := NewApp()
	plugins := app.ListPlugins()
	// Should return a non-nil slice (may contain plugins if the user has
	// plugins installed in ~/.config/kapi/plugins).
	assert.NotNil(t, plugins)
}

func TestPluginDir(t *testing.T) {
	app := NewApp()
	dir := app.PluginDir()
	assert.NotEmpty(t, dir)
}

func TestServiceShutdown(t *testing.T) {
	app := NewApp()
	// ServiceShutdown should not panic even without active plugins.
	assert.NotPanics(t, func() {
		err := app.ServiceShutdown()
		assert.NoError(t, err)
	})
}

func TestDetectFormat(t *testing.T) {
	app := NewApp()

	tests := []struct {
		path     string
		expected string
	}{
		{"doc.html", "html"},
		{"file.json", "json"},
		{"strings.po", "po"},
		{"data.yaml", "yaml"},
		{"doc.xml", "xml"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			detected, err := app.DetectFormat(tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, detected)
		})
	}
}
