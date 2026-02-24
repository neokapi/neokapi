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
				_ = cmd.Flags().Set("json", "true")
			}
			if tt.textFlag {
				_ = cmd.Flags().Set("text", "true")
			}
			if tt.format != "" {
				_ = cmd.Flags().Set("output-format", tt.format)
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
			[]string{"Project root: /project", "Not connected to a server."},
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
			[]string{"Local: 0 files, 100 blocks (0 words)", "Pending push: 5", "Pending pull: 3", "Last sync:"},
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

	t.Run("TYPE column header present", func(t *testing.T) {
		out := PluginsListOutput{
			Plugins: []PluginInfo{
				{Name: "my-plugin", Version: "1.0.0", Status: "installed"},
			},
			Total: 1,
		}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "TYPE")
	})

	t.Run("empty PluginType shows dash fallback", func(t *testing.T) {
		out := PluginsListOutput{
			Plugins: []PluginInfo{
				{Name: "legacy-plugin", Version: "1.0.0", PluginType: "", Status: "installed"},
			},
			Total: 1,
		}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)
		text := buf.String()
		assert.Contains(t, text, "legacy-plugin")
		// The TYPE column should show "-" when PluginType is empty.
		assert.Contains(t, text, "-")
	})

	t.Run("bundle PluginType displayed correctly", func(t *testing.T) {
		out := PluginsListOutput{
			Plugins: []PluginInfo{
				{Name: "okapi-bridge", Version: "1.5.0", PluginType: "bundle", Status: "installed", Formats: 46},
			},
			Total: 1,
		}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)
		text := buf.String()
		assert.Contains(t, text, "okapi-bridge")
		assert.Contains(t, text, "bundle")
	})

	t.Run("multiple plugin types in same list", func(t *testing.T) {
		out := PluginsListOutput{
			Plugins: []PluginInfo{
				{Name: "okapi-bridge", Version: "1.5.0", PluginType: "bundle", Status: "installed"},
				{Name: "custom-format", Version: "0.1.0", PluginType: "format", Status: "installed"},
				{Name: "legacy-tool", Version: "2.0.0", PluginType: "", Status: "installed"},
			},
			Total: 3,
		}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)
		text := buf.String()
		assert.Contains(t, text, "bundle")
		assert.Contains(t, text, "format")
		assert.Contains(t, text, "Total: 3 plugin(s)")
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

func TestAddPersistentFlags(t *testing.T) {
	cmd := &cobra.Command{}
	AddPersistentFlags(cmd)

	// Verify all flags are registered as persistent
	assert.NotNil(t, cmd.PersistentFlags().Lookup("json"))
	assert.NotNil(t, cmd.PersistentFlags().Lookup("text"))
	assert.NotNil(t, cmd.PersistentFlags().Lookup("output-format"))

	// output-format has no shorthand (avoids conflict with -o/--output on subcommands)
	flag := cmd.PersistentFlags().Lookup("output-format")
	assert.Equal(t, "", flag.Shorthand)
}

func TestPrintTo_FallbackToJSON(t *testing.T) {
	// Types without TextFormatter should fall back to JSON
	data := map[string]int{"count": 42}
	var buf bytes.Buffer

	err := PrintTo(&buf, FormatText, data)
	require.NoError(t, err)

	// Should be valid JSON since map doesn't implement TextFormatter
	var result map[string]int
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, 42, result["count"])
}

func TestError_Type(t *testing.T) {
	// Test the Error type JSON marshaling
	e := Error{
		Error: "something went wrong",
		Code:  "ERR_TEST",
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var parsed Error
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "something went wrong", parsed.Error)
	assert.Equal(t, "ERR_TEST", parsed.Code)
}

func TestError_OmitEmptyCode(t *testing.T) {
	e := Error{Error: "something went wrong"}
	data, err := json.Marshal(e)
	require.NoError(t, err)
	// Verify no "code" key in JSON (would be "code": if present)
	assert.NotContains(t, string(data), `"code"`)
}

func TestPrint_WithCommand(t *testing.T) {
	cmd := &cobra.Command{}
	AddFlags(cmd)

	// Capture stdout by using PrintTo instead
	var buf bytes.Buffer
	err := PrintTo(&buf, FormatText, VersionOutput{Version: "1.0.0"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "kapi 1.0.0")
}

func TestStatusOutput_WithErrors(t *testing.T) {
	status := StatusOutput{
		Project: ProjectInfo{
			Root:      "/project",
			ConfigDir: "/project/.kapi",
			Server:    "http://localhost",
		},
		Errors: []string{"sync failed", "connection timeout"},
	}

	var buf bytes.Buffer
	err := status.FormatText(&buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Errors:")
	assert.Contains(t, buf.String(), "sync failed")
	assert.Contains(t, buf.String(), "connection timeout")
}

func TestStatusOutput_PendingPullUnknown(t *testing.T) {
	status := StatusOutput{
		Project: ProjectInfo{
			Root:      "/project",
			ConfigDir: "/project/.kapi",
			Server:    "http://localhost",
		},
		PendingPull: -1, // Unknown count
	}

	var buf bytes.Buffer
	err := status.FormatText(&buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Pending pull: remote changes available")
}

func TestToolsListOutput_Empty(t *testing.T) {
	out := ToolsListOutput{Tools: []ToolInfo{}, Total: 0}
	var buf bytes.Buffer
	err := out.FormatText(&buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No tools available.")
}

func TestAuthStatusOutput_WithUserID(t *testing.T) {
	auth := AuthStatusOutput{
		LoggedIn: true,
		Server:   "http://localhost",
		User:     "test@example.com",
		UserID:   "user-123",
	}

	var buf bytes.Buffer
	err := auth.FormatText(&buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "test@example.com")
	assert.Contains(t, buf.String(), "ID: user-123")
}

func TestFormatsListOutput_JSON(t *testing.T) {
	out := FormatsListOutput{
		Formats: []FormatInfo{
			{
				Name:        "json",
				DisplayName: "JSON",
				HasReader:   true,
				HasWriter:   true,
				Source:      "built-in",
				Extensions:  []string{".json"},
				MimeTypes:   []string{"application/json"},
			},
		},
		Total: 1,
	}

	var buf bytes.Buffer
	err := PrintTo(&buf, FormatJSON, out)
	require.NoError(t, err)

	var parsed FormatsListOutput
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.Equal(t, 1, parsed.Total)
	assert.Equal(t, "json", parsed.Formats[0].Name)
	assert.True(t, parsed.Formats[0].HasReader)
}

func TestPluginsListOutput_JSON(t *testing.T) {
	out := PluginsListOutput{
		Plugins: []PluginInfo{
			{Name: "okapi-bridge", Version: "1.5.0", Status: "installed", Formats: 46},
		},
		Total: 1,
	}

	var buf bytes.Buffer
	err := PrintTo(&buf, FormatJSON, out)
	require.NoError(t, err)

	var parsed PluginsListOutput
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.Equal(t, 1, parsed.Total)
	assert.Equal(t, "okapi-bridge", parsed.Plugins[0].Name)
	assert.Equal(t, 46, parsed.Plugins[0].Formats)
}

func TestFlowsListOutput_JSON(t *testing.T) {
	out := FlowsListOutput{
		Flows: []FlowInfo{
			{Name: "translate", Description: "AI translation", Path: "/flows/translate.yaml", Steps: 2},
		},
		Total: 1,
	}

	var buf bytes.Buffer
	err := PrintTo(&buf, FormatJSON, out)
	require.NoError(t, err)

	var parsed FlowsListOutput
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.Equal(t, 1, parsed.Total)
	assert.Equal(t, "translate", parsed.Flows[0].Name)
	assert.Equal(t, 2, parsed.Flows[0].Steps)
}

func TestLsOutput_FormatText_Basic(t *testing.T) {
	out := LsOutput{
		Files: []LsEntry{
			{Path: "src/locales/en.json", Format: "json"},
			{Path: "src/locales/fr.json", Format: "json"},
			{Path: "docs/readme.md", Format: "markdown"},
		},
		Total: 3,
	}

	var buf bytes.Buffer
	err := out.FormatText(&buf)
	require.NoError(t, err)

	text := buf.String()
	assert.Contains(t, text, "src/locales/en.json")
	assert.Contains(t, text, "json")
	assert.Contains(t, text, "docs/readme.md")
	assert.Contains(t, text, "markdown")
	assert.Contains(t, text, "3 file(s)")
	// Should NOT contain column headers (no stats mode).
	assert.NotContains(t, text, "BLOCKS")
}

func TestLsOutput_FormatText_WithStats(t *testing.T) {
	out := LsOutput{
		Files: []LsEntry{
			{Path: "src/locales/en.json", Format: "json", Blocks: 42, Words: 1230},
			{Path: "docs/readme.md", Format: "markdown", Blocks: 8, Words: 540},
		},
		Total:    2,
		Blocks:   50,
		Words:    1770,
		HasStats: true,
	}

	var buf bytes.Buffer
	err := out.FormatText(&buf)
	require.NoError(t, err)

	text := buf.String()
	assert.Contains(t, text, "PATH")
	assert.Contains(t, text, "FORMAT")
	assert.Contains(t, text, "BLOCKS")
	assert.Contains(t, text, "WORDS")
	assert.NotContains(t, text, "DIRTY")
	assert.Contains(t, text, "src/locales/en.json")
	assert.Contains(t, text, "42")
	assert.Contains(t, text, "1230")
	assert.Contains(t, text, "2 file(s), 50 blocks, 1770 words")
}

func TestLsOutput_FormatText_WithDirty(t *testing.T) {
	out := LsOutput{
		Files: []LsEntry{
			{Path: "src/locales/en.json", Format: "json", Blocks: 42, Words: 1230, Dirty: 3},
			{Path: "docs/readme.md", Format: "markdown", Blocks: 8, Words: 540, Dirty: 1},
		},
		Total:    2,
		Blocks:   50,
		Words:    1770,
		Changed:  4,
		HasStats: true,
		HasDirty: true,
	}

	var buf bytes.Buffer
	err := out.FormatText(&buf)
	require.NoError(t, err)

	text := buf.String()
	assert.Contains(t, text, "DIRTY")
	assert.Contains(t, text, "3")
	assert.Contains(t, text, "4 changed")
}

func TestLsOutput_FormatText_Empty(t *testing.T) {
	out := LsOutput{
		Files: []LsEntry{},
		Total: 0,
	}

	var buf bytes.Buffer
	err := out.FormatText(&buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No tracked files.")
}

func TestLsOutput_JSON(t *testing.T) {
	out := LsOutput{
		Files: []LsEntry{
			{Path: "src/en.json", Format: "json", Blocks: 10, Words: 200, Dirty: 2},
		},
		Total:    1,
		Blocks:   10,
		Words:    200,
		Changed:  2,
		HasStats: true,
		HasDirty: true,
	}

	var buf bytes.Buffer
	err := PrintTo(&buf, FormatJSON, out)
	require.NoError(t, err)

	var parsed LsOutput
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.Equal(t, 1, parsed.Total)
	assert.Equal(t, "src/en.json", parsed.Files[0].Path)
	assert.Equal(t, 10, parsed.Files[0].Blocks)
	assert.Equal(t, 2, parsed.Files[0].Dirty)
}

func TestPresetsListOutput_FormatText(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		out := PresetsListOutput{Presets: []PresetEntry{}, Total: 0}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No presets available.")
	})

	t.Run("mixed presets", func(t *testing.T) {
		out := PresetsListOutput{
			Presets: []PresetEntry{
				{Name: "nextjs", Type: "framework", Description: "Next.js App Router", Source: "built-in"},
				{Name: "okf_html@wellFormed", Type: "format", Description: "Strict XHTML", Format: "okf_html", Source: "bridge", IsDefault: false},
				{Name: "okf_html@default", Type: "format", Description: "Default HTML", Format: "okf_html", Source: "bridge", IsDefault: true},
			},
			Total: 3,
		}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)

		text := buf.String()
		assert.Contains(t, text, "Framework Presets:")
		assert.Contains(t, text, "nextjs")
		assert.Contains(t, text, "Format Presets:")
		assert.Contains(t, text, "okf_html@wellFormed")
		assert.Contains(t, text, "(default)")
		assert.Contains(t, text, "Total: 3 preset(s)")
	})

	t.Run("framework only", func(t *testing.T) {
		out := PresetsListOutput{
			Presets: []PresetEntry{
				{Name: "nextjs", Type: "framework", Description: "Next.js", Source: "built-in"},
			},
			Total: 1,
		}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)

		text := buf.String()
		assert.Contains(t, text, "Framework Presets:")
		assert.NotContains(t, text, "Format Presets:")
	})
}

func TestPresetsListOutput_JSON(t *testing.T) {
	out := PresetsListOutput{
		Presets: []PresetEntry{
			{
				Name:        "nextjs",
				Type:        "framework",
				Description: "Next.js App Router",
				Source:      "built-in",
				Mappings:    []MappingEntry{{Local: "messages/*.json", Format: "json", TargetPath: "messages/{locale}.json"}},
				Exclude:     []string{"node_modules/**"},
			},
			{
				Name:        "okf_html@wellFormed",
				Type:        "format",
				Description: "Strict XHTML",
				Format:      "okf_html",
				Source:      "bridge",
				IsDefault:   false,
				Config:      map[string]any{"assumeWellformed": true},
			},
		},
		Total: 2,
	}

	var buf bytes.Buffer
	err := PrintTo(&buf, FormatJSON, out)
	require.NoError(t, err)

	var parsed PresetsListOutput
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.Equal(t, 2, parsed.Total)
	assert.Equal(t, "nextjs", parsed.Presets[0].Name)
	assert.Equal(t, "framework", parsed.Presets[0].Type)
	assert.Equal(t, 1, len(parsed.Presets[0].Mappings))
	assert.Equal(t, "messages/*.json", parsed.Presets[0].Mappings[0].Local)
	assert.Equal(t, []string{"node_modules/**"}, parsed.Presets[0].Exclude)

	assert.Equal(t, "okf_html@wellFormed", parsed.Presets[1].Name)
	assert.Equal(t, "format", parsed.Presets[1].Type)
	assert.Equal(t, "okf_html", parsed.Presets[1].Format)
	assert.Equal(t, true, parsed.Presets[1].Config["assumeWellformed"])
}

func TestPresetShowOutput_FormatText(t *testing.T) {
	t.Run("framework preset", func(t *testing.T) {
		out := PresetShowOutput{
			Name:        "nextjs",
			Type:        "framework",
			Description: "Next.js App Router",
			Source:      "built-in",
			Mappings:    []MappingEntry{{Local: "messages/*.json", Format: "json", TargetPath: "messages/{locale}.json"}},
			Exclude:     []string{"node_modules/**", ".next/**"},
		}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)

		text := buf.String()
		assert.Contains(t, text, "Framework Preset: nextjs")
		assert.Contains(t, text, "Description: Next.js App Router")
		assert.Contains(t, text, "Source: built-in")
		assert.Contains(t, text, "Mappings:")
		assert.Contains(t, text, "local: messages/*.json")
		assert.Contains(t, text, "Exclude: node_modules/**, .next/**")
	})

	t.Run("format preset", func(t *testing.T) {
		out := PresetShowOutput{
			Name:        "wellFormed",
			Type:        "format",
			Description: "Strict XHTML",
			Format:      "okf_html",
			Source:      "bridge",
			IsDefault:   true,
			Config:      map[string]any{"assumeWellformed": true},
		}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)

		text := buf.String()
		assert.Contains(t, text, "Format Preset: okf_html@wellFormed")
		assert.Contains(t, text, "Description: Strict XHTML")
		assert.Contains(t, text, "Default: yes")
		assert.Contains(t, text, "Configuration:")
		assert.Contains(t, text, "assumeWellformed: true")
	})
}

func TestPresetShowOutput_JSON(t *testing.T) {
	out := PresetShowOutput{
		Name:        "wellFormed",
		Type:        "format",
		Description: "Strict XHTML",
		Format:      "okf_html",
		Source:      "bridge",
		IsDefault:   true,
		Config:      map[string]any{"assumeWellformed": true},
	}

	var buf bytes.Buffer
	err := PrintTo(&buf, FormatJSON, out)
	require.NoError(t, err)

	var parsed PresetShowOutput
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.Equal(t, "wellFormed", parsed.Name)
	assert.Equal(t, "format", parsed.Type)
	assert.Equal(t, "okf_html", parsed.Format)
	assert.True(t, parsed.IsDefault)
	assert.Equal(t, true, parsed.Config["assumeWellformed"])
}

func TestPresetsValidateOutput_FormatText(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		out := PresetsValidateOutput{Valid: true}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "All presets and overrides are valid.")
	})

	t.Run("with errors", func(t *testing.T) {
		out := PresetsValidateOutput{
			Valid:  false,
			Errors: []string{"okf_html: unknown parameter: preservWhitespace", "json: expected boolean, got string"},
		}
		var buf bytes.Buffer
		err := out.FormatText(&buf)
		require.NoError(t, err)

		text := buf.String()
		assert.Contains(t, text, "Found 2 validation error(s):")
		assert.Contains(t, text, "preservWhitespace")
		assert.Contains(t, text, "expected boolean")
	})
}

func TestPresetsValidateOutput_JSON(t *testing.T) {
	out := PresetsValidateOutput{
		Valid:  false,
		Errors: []string{"bad param", "wrong type"},
	}

	var buf bytes.Buffer
	err := PrintTo(&buf, FormatJSON, out)
	require.NoError(t, err)

	var parsed PresetsValidateOutput
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.False(t, parsed.Valid)
	assert.Equal(t, 2, len(parsed.Errors))
	assert.Equal(t, "bad param", parsed.Errors[0])
}
