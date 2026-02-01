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
	assert.Equal(t, 18, len(tools), "expected 18 tools")

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

func TestListFlows(t *testing.T) {
	app := NewApp()
	flows := app.ListFlows()
	assert.Equal(t, 5, len(flows))

	names := make(map[string]bool)
	for _, f := range flows {
		names[f.Name] = true
	}
	assert.True(t, names["ai-translate"])
	assert.True(t, names["ai-translate-qa"])
	assert.True(t, names["pseudo-translate"])
	assert.True(t, names["qa-check"])
	assert.True(t, names["tm-leverage"])
}

func TestListPlugins(t *testing.T) {
	app := NewApp()
	plugins := app.ListPlugins()
	// Should return a non-nil slice (may contain plugins if the user has
	// plugins installed in ~/.kapi/plugins).
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

func TestConvert_MissingInput(t *testing.T) {
	app := NewApp()
	_, err := app.Convert(ConvertRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input path is required")
}

func TestConvert_MissingOutput(t *testing.T) {
	app := NewApp()
	_, err := app.Convert(ConvertRequest{InputPath: "test.html"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output path is required")
}

func TestTranslate_MissingInput(t *testing.T) {
	app := NewApp()
	_, err := app.Translate(TranslateRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input path is required")
}

func TestTranslate_MissingTargetLang(t *testing.T) {
	app := NewApp()
	_, err := app.Translate(TranslateRequest{InputPath: "test.html"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "target language is required")
}

func TestExecuteFlow_MissingFlowName(t *testing.T) {
	app := NewApp()
	_, err := app.ExecuteFlow(FlowRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "flow name is required")
}

func TestExecuteFlow_UnknownFlow(t *testing.T) {
	app := NewApp()
	_, err := app.ExecuteFlow(FlowRequest{
		FlowName:   "nonexistent",
		InputPath:  "test.html",
		TargetLang: "fr",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flow")
}

func TestBuildFlowTools(t *testing.T) {
	tests := []struct {
		name      string
		flowName  string
		wantCount int
		wantErr   bool
	}{
		{"ai-translate", "ai-translate", 1, false},
		{"ai-translate-qa", "ai-translate-qa", 2, false},
		{"pseudo-translate", "pseudo-translate", 1, false},
		{"qa-check", "qa-check", 1, false},
		{"segmentation", "segmentation", 1, false},
		{"tm-leverage", "tm-leverage", 1, false},
		{"unknown", "invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tools, err := buildFlowTools(tt.flowName, "", "", "", "en", "fr")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantCount, len(tools))
			}
		})
	}
}
