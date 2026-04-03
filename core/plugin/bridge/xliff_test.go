package bridge

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsXLIFFFilter(t *testing.T) {
	tests := []struct {
		filterClass string
		want        bool
	}{
		{"net.sf.okapi.filters.xliff.XLIFFFilter", true},
		{"net.sf.okapi.filters.xliff2.XLIFF2Filter", true},
		{"com.example.CustomXLIFFFilter", true},
		{"com.example.CustomXLIFF2Filter", true},
		{"net.sf.okapi.filters.html.HtmlFilter", false},
		{"net.sf.okapi.filters.json.JSONFilter", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.filterClass, func(t *testing.T) {
			assert.Equal(t, tt.want, isXLIFFFilter(tt.filterClass))
		})
	}
}

func TestStripEmptyTargetLanguage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty double quotes",
			input:    `<file source-language="en" target-language="" original="test.txt">`,
			expected: `<file source-language="en" original="test.txt">`,
		},
		{
			name:     "empty single quotes",
			input:    `<file source-language="en" target-language='' original="test.txt">`,
			expected: `<file source-language="en" original="test.txt">`,
		},
		{
			name:     "non-empty value preserved",
			input:    `<file source-language="en" target-language="fr" original="test.txt">`,
			expected: `<file source-language="en" target-language="fr" original="test.txt">`,
		},
		{
			name:     "no target-language attribute",
			input:    `<file source-language="en" original="test.txt">`,
			expected: `<file source-language="en" original="test.txt">`,
		},
		{
			name:     "spaces around equals",
			input:    `<file source-language="en" target-language = "" original="test.txt">`,
			expected: `<file source-language="en" original="test.txt">`,
		},
		{
			name:     "multiple empty attributes",
			input:    `<file target-language=""><file target-language="">`,
			expected: `<file><file>`,
		},
		{
			name:     "xliff2 format",
			input:    `<xliff srcLang="en" trgLang="fr"><file id="f1" target-language="">`,
			expected: `<xliff srcLang="en" trgLang="fr"><file id="f1">`,
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripEmptyTargetLanguage([]byte(tt.input))
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestStripEmptyTargetLanguageFile(t *testing.T) {
	t.Run("file with empty target-language creates temp", func(t *testing.T) {
		content := `<?xml version="1.0"?>
<xliff version="1.2">
  <file source-language="en" target-language="" original="test.txt">
    <body><trans-unit id="1"><source>Hello</source></trans-unit></body>
  </file>
</xliff>`

		tmpFile, err := os.CreateTemp("", "xliff-test-*")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)
		require.NoError(t, tmpFile.Close())

		resultPath, cleanup, err := stripEmptyTargetLanguageFile(tmpFile.Name())
		require.NoError(t, err)
		require.NotNil(t, cleanup, "cleanup should be non-nil when stripping occurred")
		defer cleanup()

		assert.NotEqual(t, tmpFile.Name(), resultPath, "should return a different temp file")

		result, err := os.ReadFile(resultPath)
		require.NoError(t, err)
		assert.NotContains(t, string(result), `target-language=""`)
		assert.Contains(t, string(result), `source-language="en"`)
	})

	t.Run("file without empty target-language returns same path", func(t *testing.T) {
		content := `<?xml version="1.0"?>
<xliff version="1.2">
  <file source-language="en" target-language="fr" original="test.txt">
    <body><trans-unit id="1"><source>Hello</source></trans-unit></body>
  </file>
</xliff>`

		tmpFile, err := os.CreateTemp("", "xliff-test-*")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)
		require.NoError(t, tmpFile.Close())

		resultPath, cleanup, err := stripEmptyTargetLanguageFile(tmpFile.Name())
		require.NoError(t, err)
		assert.Nil(t, cleanup, "cleanup should be nil when no stripping needed")
		assert.Equal(t, tmpFile.Name(), resultPath, "should return the original path")
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		_, _, err := stripEmptyTargetLanguageFile("/nonexistent/file.xlf")
		assert.Error(t, err)
	})
}
