package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPresetsFromFile(t *testing.T) {
	content := `kind: format-presets
format_presets:
  strict-html:
    description: "Strict XHTML processing"
    formats:
      - format: okf_html
        config:
          assumeWellformed: true
framework_presets:
  nextjs:
    description: "Next.js with next-intl"
    mappings:
      - local: "messages/*.json"
        format: json
        target_path: "messages/{locale}.json"
    exclude:
      - "node_modules/**"
    format_presets:
      json:
        extractArrayStrings: false
    flows:
      translate:
        ai_provider: anthropic
`
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	reg := preset.NewPresetRegistry()
	err := LoadPresetsFromFile(path, reg, "test-plugin")
	require.NoError(t, err)

	// Check format preset
	fp := reg.GetFormatPreset("okf_html", "strict-html")
	require.NotNil(t, fp)
	assert.Equal(t, "Strict XHTML processing", fp.Description)
	assert.Equal(t, true, fp.Config["assumeWellformed"])
	assert.Equal(t, "test-plugin", fp.Source)

	// Check framework preset
	fwp := reg.GetFrameworkPreset("nextjs")
	require.NotNil(t, fwp)
	assert.Equal(t, "Next.js with next-intl", fwp.Description)
	assert.Len(t, fwp.Mappings, 1)
	assert.Equal(t, "messages/*.json", fwp.Mappings[0].Local)
	assert.Contains(t, fwp.Exclude, "node_modules/**")
	assert.Equal(t, "test-plugin", fwp.Source)
}

func TestLoadPresetsFromFileNotFound(t *testing.T) {
	reg := preset.NewPresetRegistry()
	err := LoadPresetsFromFile("/nonexistent/presets.yaml", reg, "test")
	assert.Error(t, err)
}
