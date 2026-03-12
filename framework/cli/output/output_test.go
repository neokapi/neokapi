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

func TestGetFormat(t *testing.T) {
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

			got := GetFormat(cmd)
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

func TestVersionOutputText(t *testing.T) {
	var buf bytes.Buffer
	v := VersionOutput{Program: "kapi", Version: "1.0.0"}
	err := v.FormatText(&buf)
	require.NoError(t, err)
	assert.Equal(t, "kapi 1.0.0\n", buf.String())
}

func TestVersionOutputTextFull(t *testing.T) {
	var buf bytes.Buffer
	v := VersionOutput{
		Program:   "bowrain",
		Version:   "1.2.3",
		Commit:    "abc1234",
		BuildDate: "2025-01-01",
	}
	err := v.FormatText(&buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "bowrain 1.2.3")
	assert.Contains(t, buf.String(), "abc1234")
}
