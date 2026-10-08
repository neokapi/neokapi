package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Inside a project that binds no voice, `kapi voice show` answers that no tone
// or style guidance applies, and succeeds: an assistant told to read the voice
// before writing gets the state of the project, never a demand for flags it
// has no reason to pass.
func TestVoiceShowInAProjectWithNoVoiceSaysSo(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(`version: v1
name: no-voice
defaults:
  source_language: en
collections:
  - name: docs
    source_only: true
    content:
      - path: "docs/*.md"
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("# Guide\n"), 0o644))
	t.Chdir(root)

	for _, verb := range []string{"show"} {
		t.Run(verb, func(t *testing.T) {
			cmd := NewVoiceCmd(&App{})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{verb, filepath.Join("docs", "guide.md")})
			require.NoError(t, cmd.Execute())
			assert.Equal(t, "No voice profile is bound at this point, so no tone or style guidance applies.\n", out.String())
		})
	}
}

// Outside any project there is nothing to report on, so show still asks which
// profile to use.
func TestVoiceShowOutsideAProjectAsksForAProfile(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Chdir(t.TempDir())

	cmd := NewVoiceCmd(&App{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"show"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "specify a profile with --profile")
}
