package output

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

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
				cmd.Flags().Set("json", "true")
			}
			if tt.textFlag {
				cmd.Flags().Set("text", "true")
			}
			if tt.format != "" {
				cmd.Flags().Set("output-format", tt.format)
			}

			got := GetFormat(cmd)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPrintTo_JSON(t *testing.T) {
	data := map[string]string{"key": "value"}
	var buf bytes.Buffer

	err := PrintTo(&buf, FormatJSON, data)
	require.NoError(t, err)

	var result map[string]string
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "value", result["key"])
}

func TestPrintTo_TextFormatter(t *testing.T) {
	data := VersionOutput{
		Version:   "1.0.0",
		Commit:    "abc123",
		BuildDate: "2024-01-01",
	}
	var buf bytes.Buffer

	err := PrintTo(&buf, FormatText, data)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "kapi 1.0.0")
	assert.Contains(t, buf.String(), "abc123")
}

func TestVersionOutput_FormatText(t *testing.T) {
	tests := []struct {
		name    string
		version VersionOutput
		want    string
	}{
		{
			"full version",
			VersionOutput{Version: "1.0.0", Commit: "abc123", BuildDate: "2024-01-01"},
			"kapi 1.0.0 (commit: abc123, built: 2024-01-01)\n",
		},
		{
			"version only",
			VersionOutput{Version: "1.0.0"},
			"kapi 1.0.0\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := tt.version.FormatText(&buf)
			require.NoError(t, err)
			assert.Equal(t, tt.want, buf.String())
		})
	}
}

func TestStatusOutput_FormatText(t *testing.T) {
	lastSync := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name   string
		status StatusOutput
		checks []string
	}{
		{
			"no server configured",
			StatusOutput{
				Project: ProjectInfo{Root: "/project", ConfigDir: "/project/.kapi"},
			},
			[]string{"Project root: /project", "Sync status requires a Bowrain server"},
		},
		{
			"up to date",
			StatusOutput{
				Project:  ProjectInfo{Root: "/project", ConfigDir: "/project/.kapi", Server: "http://localhost"},
				UpToDate: true,
			},
			[]string{"Up to date."},
		},
		{
			"pending changes",
			StatusOutput{
				Project:     ProjectInfo{Root: "/project", ConfigDir: "/project/.kapi", Server: "http://localhost"},
				ItemCount:   100,
				PendingPush: 5,
				PendingPull: 3,
				LastSync:    &lastSync,
			},
			[]string{"Local blocks: 100", "Pending push: 5", "Pending pull: 3", "Last sync:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := tt.status.FormatText(&buf)
			require.NoError(t, err)
			for _, check := range tt.checks {
				assert.Contains(t, buf.String(), check)
			}
		})
	}
}

func TestFormatsListOutput_FormatText(t *testing.T) {
	out := FormatsListOutput{
		Formats: []FormatInfo{
			{Name: "json", DisplayName: "JSON", HasReader: true, HasWriter: true},
			{Name: "xliff", DisplayName: "XLIFF 1.2", HasReader: true, HasWriter: false},
		},
		Total: 2,
	}

	var buf bytes.Buffer
	err := out.FormatText(&buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Available formats:")
	assert.Contains(t, buf.String(), "json")
	assert.Contains(t, buf.String(), "xliff")
	assert.Contains(t, buf.String(), "Total: 2 format(s)")
}

func TestPluginsListOutput_FormatText(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		out := PluginsListOutput{Plugins: []PluginInfo{}, Total: 0}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No plugins installed.")
	})

	t.Run("with plugins", func(t *testing.T) {
		out := PluginsListOutput{
			Plugins: []PluginInfo{
				{Name: "okapi-bridge", Version: "1.5.0", Status: "installed"},
			},
			Total: 1,
		}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "okapi-bridge")
		assert.Contains(t, buf.String(), "1.5.0")
	})
}

func TestToolsListOutput_FormatText(t *testing.T) {
	out := ToolsListOutput{
		Tools: []ToolInfo{
			{Name: "pseudo-translate", Description: "Generate pseudo-translations"},
			{Name: "ai-translate", Description: "Translate using AI"},
		},
		Total: 2,
	}

	var buf bytes.Buffer
	err := out.FormatText(&buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Available tools:")
	assert.Contains(t, buf.String(), "pseudo-translate")
	assert.Contains(t, buf.String(), "ai-translate")
	assert.Contains(t, buf.String(), "Total: 2 tool(s)")
}

func TestAuthStatusOutput_FormatText(t *testing.T) {
	expiry := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)

	tests := []struct {
		name   string
		auth   AuthStatusOutput
		checks []string
	}{
		{
			"not logged in",
			AuthStatusOutput{LoggedIn: false},
			[]string{"Not logged in."},
		},
		{
			"logged in",
			AuthStatusOutput{
				LoggedIn:  true,
				Server:    "http://localhost:8080",
				User:      "user@example.com",
				ExpiresAt: &expiry,
			},
			[]string{"Server: http://localhost:8080", "User:   user@example.com", "Token expires:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := tt.auth.FormatText(&buf)
			require.NoError(t, err)
			for _, check := range tt.checks {
				assert.Contains(t, buf.String(), check)
			}
		})
	}
}

func TestFlowsListOutput_FormatText(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		out := FlowsListOutput{Flows: []FlowInfo{}, Total: 0}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No flows defined.")
	})

	t.Run("with flows", func(t *testing.T) {
		out := FlowsListOutput{
			Flows: []FlowInfo{
				{Name: "ai-translate", Description: "AI translation", Steps: 1},
				{Name: "qa-check", Description: "Quality check", Steps: 2},
			},
			Total: 2,
		}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Available flows:")
		assert.Contains(t, buf.String(), "ai-translate")
		assert.Contains(t, buf.String(), "qa-check")
	})
}

func TestJSONOutput(t *testing.T) {
	// Verify JSON output can be parsed correctly
	version := VersionOutput{
		Version:   "1.0.0",
		Commit:    "abc123",
		BuildDate: "2024-01-01",
	}

	var buf bytes.Buffer
	err := PrintTo(&buf, FormatJSON, version)
	require.NoError(t, err)

	var parsed VersionOutput
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)
	assert.Equal(t, version, parsed)
}
