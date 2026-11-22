package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Opening a recipe with a YAML error shows the parser's error with its line,
// as the CLI does.
func TestOpenProjectReportsTheRecipeParseError(t *testing.T) {
	dir := t.TempDir()
	recipe := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: dup\ndefaults:\n  target_languages: [fr]\n  target_languages: [de]\n"), 0o644))

	_, err := NewApp().OpenProject(recipe)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 5")
	assert.Contains(t, err.Error(), "already defined")
	assert.NotContains(t, err.Error(), "not a kapi recipe")
}
