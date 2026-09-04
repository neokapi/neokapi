package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `kapi voice pointer` writes the section into the project's assistant file
// for a project that already exists, the way init does for a fresh one.
func TestVoicePointerCmd(t *testing.T) {
	withVoice := "version: v1\nname: demo\ndefaults:\n  source_language: en\n  voice:\n    pack: friendly-dtc\n"
	withoutVoice := "version: v1\nname: demo\ndefaults:\n  source_language: en\n"

	tests := []struct {
		name     string
		recipe   string
		files    map[string]string
		args     []string
		wantErr  string
		wantFile string
		wantOut  []string
	}{
		{
			name:     "creates AGENTS.md and says how CLAUDE.md reads it",
			recipe:   withVoice,
			wantFile: "AGENTS.md",
			wantOut:  []string{"Wrote the voice pointer to", "(voice: Friendly DTC)", "@AGENTS.md"},
		},
		{
			name:     "writes into an existing CLAUDE.md",
			recipe:   withVoice,
			files:    map[string]string{"CLAUDE.md": "# Rules\n"},
			wantFile: "CLAUDE.md",
			wantOut:  []string{"Wrote the voice pointer into"},
		},
		{
			name:    "refuses a project with no voice",
			recipe:  withoutVoice,
			wantErr: "binds no voice profile",
		},
		{
			name:     "removes a stale section when the voice is gone",
			recipe:   withoutVoice,
			files:    map[string]string{"AGENTS.md": "# demo\n\n<!-- kapi:voice -->\nold\n<!-- /kapi:voice -->\n"},
			wantFile: "AGENTS.md",
			wantOut:  []string{"Removed the voice pointer from"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(tt.recipe), 0o644))
			for rel, body := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644))
			}

			a := &App{}
			cmd := NewVoiceCmd(a)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(append([]string{"pointer", "-p", root}, tt.args...))
			err := cmd.Execute()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				_, serr := os.Stat(filepath.Join(root, "AGENTS.md"))
				assert.True(t, os.IsNotExist(serr), "a refusal writes nothing")
				return
			}
			require.NoError(t, err)
			for _, w := range tt.wantOut {
				assert.Contains(t, out.String(), w)
			}
			body, rerr := os.ReadFile(filepath.Join(root, tt.wantFile))
			require.NoError(t, rerr)
			if tt.recipe == withVoice {
				assert.Contains(t, string(body), "voice, Friendly DTC, is held by kapi")
			} else {
				assert.NotContains(t, string(body), "kapi:voice")
			}
		})
	}
}

func TestVoicePointerCmd_JSON(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"),
		[]byte("version: v1\nname: demo\ndefaults:\n  source_language: en\n  voice:\n    pack: friendly-dtc\n"), 0o644))

	a := &App{}
	cmd := NewVoiceCmd(a)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"pointer", "-p", root, "--json"})
	require.NoError(t, cmd.Execute())

	var got struct {
		File    string `json:"file"`
		Created bool   `json:"created"`
		Action  string `json:"action"`
		Voice   string `json:"voice"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &got), out.String())
	assert.Equal(t, filepath.Join(root, "AGENTS.md"), got.File)
	assert.True(t, got.Created)
	assert.Equal(t, "created", got.Action)
	assert.Equal(t, "Friendly DTC", got.Voice)
}

func TestVoicePointerCmd_NeedsAProject(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	a := &App{}
	cmd := NewVoiceCmd(a)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"pointer"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no kapi project found")
}
