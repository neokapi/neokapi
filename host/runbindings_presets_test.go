package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A run whose recipe binds a voice this machine's store does not hold keeps
// the recipe's tool settings. A tool that takes no voice runs without a word
// about it; the first tool that would have taken the voice says, even when
// the run is quiet, and says it once.
func TestARunWithAnUnresolvedVoiceKeepsTheRecipeToolSettings(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
name: presets
defaults:
  source_language: en
  voice: not-imported
  locales:
    qps:
      tools:
        pseudo-translate:
          prefix: ""
          suffix: ""
`), 0o644))

	cmd := bindingsCmd(t, recipe)
	var stderr bytes.Buffer
	cmd.(*EnvCommand).SetErr(&stderr)
	a := &App{Quiet: true}
	b := a.resolveRunBindings("", cmd)
	require.NotNil(t, b, "the recipe's tool settings apply without the voice")
	assert.Nil(t, b.profile)

	got := a.applyBindingsFor(b, "pseudo-translate", nil, map[string]any{}, "qps")
	require.Contains(t, got, "prefix", "the recipe sets the markers")
	assert.Empty(t, got["prefix"])
	assert.Empty(t, got["suffix"])
	assert.Empty(t, stderr.String(), "a tool that takes no voice has nothing to say about one")

	a.applyBindingsFor(b, "translate", nil, map[string]any{}, "qps")
	a.applyBindingsFor(b, "translate", nil, map[string]any{}, "qps")
	assert.Contains(t, stderr.String(), "not-imported", "a quiet run still says what a voice-using tool went without")
	assert.Equal(t, 1, bytes.Count(stderr.Bytes(), []byte("Warning:")), "said once")
}
