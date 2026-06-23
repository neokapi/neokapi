package output

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveFormat(t *testing.T) {
	tests := []struct {
		name     string
		jsonFlag bool
		textFlag bool
		format   string
		want     Format
	}{
		{"default is text", false, false, "", FormatText},
		{"--json flag", true, false, "", FormatJSON},
		{"--text flag", false, true, "", FormatText},
		{"--json takes precedence over --text", true, true, "", FormatJSON},
		{"--output-format=json", false, false, "json", FormatJSON},
		{"--output-format=text", false, false, "text", FormatText},
		{"--json takes precedence over --output-format", true, false, "text", FormatJSON},
		{"invalid --output-format falls back to text", false, false, "invalid", FormatText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			AddFlags(cmd)

			if tt.jsonFlag {
				require.NoError(t, cmd.Flags().Set("json", "true"))
			}
			if tt.textFlag {
				require.NoError(t, cmd.Flags().Set("text", "true"))
			}
			if tt.format != "" {
				require.NoError(t, cmd.Flags().Set("output-format", tt.format))
			}

			got := ResolveFormat(cmd)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPrintToJSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]string{"key": "value"}
	err := PrintTo(&buf, FormatJSON, data)
	require.NoError(t, err)

	var result map[string]string
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "value", result["key"])
}

func TestPrintToText(t *testing.T) {
	var buf bytes.Buffer

	type testData struct {
		Value string `json:"value"`
	}

	data := testData{Value: "hello"}
	err := PrintTo(&buf, FormatText, data)
	require.NoError(t, err)
	// Without TextFormatter, falls back to JSON
	assert.Contains(t, buf.String(), "hello")
}

type textFormattable struct {
	Msg string
}

func (t textFormattable) FormatText(w io.Writer) error {
	_, err := w.Write([]byte(t.Msg))
	return err
}

func TestPrintToTextWithFormatter(t *testing.T) {
	var buf bytes.Buffer
	data := textFormattable{Msg: "formatted"}
	err := PrintTo(&buf, FormatText, data)
	require.NoError(t, err)
	assert.Equal(t, "formatted", buf.String())
}

func TestToolsListOutputGroupsByCategory(t *testing.T) {
	var buf bytes.Buffer
	out := ToolsListOutput{
		Tools: []ToolInfo{
			{Name: "translate", Description: "Translate with AI", Category: "translation"},
			{Name: "pseudo-translate", Description: "Pseudo-translate", Category: "translation"},
			{Name: "qa", Description: "Run QA checks", Category: "quality"},
			{Name: "word-count", Description: "Count words", Category: "analysis"},
			{Name: "search-replace", Description: "Find and replace", Category: "text-processing"},
		},
		Total: 5,
	}
	err := out.FormatText(&buf)
	require.NoError(t, err)
	text := buf.String()

	assert.Contains(t, text, "Translation:")
	assert.Contains(t, text, "Quality:")
	assert.Contains(t, text, "Analysis:")
	assert.Contains(t, text, "Text Processing:")
	assert.Contains(t, text, "translate")
	assert.Contains(t, text, "qa")
	assert.Contains(t, text, "word-count")
	assert.Contains(t, text, "search-replace")
	assert.Contains(t, text, "Total: 5 tool(s)")
}

func TestToolsListOutputUncategorizedToolsInOther(t *testing.T) {
	var buf bytes.Buffer
	out := ToolsListOutput{
		Tools: []ToolInfo{
			{Name: "translate", Description: "Translate with AI", Category: "translation"},
			{Name: "span-classify", Description: "Classify spans", Category: ""},
		},
		Total: 2,
	}
	err := out.FormatText(&buf)
	require.NoError(t, err)
	text := buf.String()

	assert.Contains(t, text, "Translation:")
	assert.Contains(t, text, "Other:")
	assert.Contains(t, text, "span-classify")
}

func TestToolsListOutputEmpty(t *testing.T) {
	var buf bytes.Buffer
	out := ToolsListOutput{Tools: nil, Total: 0}
	err := out.FormatText(&buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No tools available.")
}

func TestToolsListOutputJSON(t *testing.T) {
	out := ToolsListOutput{
		Tools: []ToolInfo{
			{Name: "qa", Description: "Run QA", Category: "quality", Source: "builtin"},
		},
		Total: 1,
	}
	var buf bytes.Buffer
	err := PrintTo(&buf, FormatJSON, out)
	require.NoError(t, err)

	var result ToolsListOutput
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "quality", result.Tools[0].Category)
}

func TestVersionOutputText(t *testing.T) {
	var buf bytes.Buffer
	v := VersionOutput{Program: "kapi", Version: "1.0.0"}
	err := v.FormatText(&buf)
	require.NoError(t, err)
	assert.Equal(t, "kapi 1.0.0\n", buf.String())
}

func TestVersionOutputTextBetaBadge(t *testing.T) {
	var buf bytes.Buffer
	v := VersionOutput{Program: "kapi", Version: "1.2.0-rc.1", Channel: "beta"}
	require.NoError(t, v.FormatText(&buf))
	assert.Equal(t, "kapi 1.2.0-rc.1 (beta)\n", buf.String())

	buf.Reset()
	v = VersionOutput{
		Program: "kapi", Version: "1.2.0-rc.1", Channel: "beta",
		Commit: "abc1234", BuildDate: "2026-06-23",
	}
	require.NoError(t, v.FormatText(&buf))
	assert.Equal(t, "kapi 1.2.0-rc.1 (beta, commit: abc1234, built: 2026-06-23)\n", buf.String())

	// A stable channel renders no badge (behaviour unchanged).
	buf.Reset()
	v = VersionOutput{Program: "kapi", Version: "1.2.0", Channel: "stable"}
	require.NoError(t, v.FormatText(&buf))
	assert.Equal(t, "kapi 1.2.0\n", buf.String())
}

func TestVersionOutputTextFull(t *testing.T) {
	var buf bytes.Buffer
	v := VersionOutput{
		Program:   "myapp",
		Version:   "1.2.3",
		Commit:    "abc1234",
		BuildDate: "2025-01-01",
	}
	err := v.FormatText(&buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "myapp 1.2.3")
	assert.Contains(t, buf.String(), "abc1234")
}
