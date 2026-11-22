package host

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A run given a -p that names no project warns, naming the path, that it
// applies no recipe; the warning is not dropped with the error.
func TestResolveRunBindingsWarnsOfAProjectThatIsNotThere(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nowhere")
	cmd := bindingsCmd(t, missing)
	var stderr bytes.Buffer
	cmd.(*EnvCommand).SetErr(&stderr)

	(&App{}).resolveRunBindings("", cmd)
	assert.Contains(t, stderr.String(), "applies no recipe")
	assert.Contains(t, stderr.String(), missing)
}

// Context search given a -p that names no project reports the recipe as
// unreadable, naming the path.
func TestContextSearchReportsAProjectThatIsNotThere(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nowhere")
	src, cleanup := (&App{}).ContextSearchSourcesFor(bindingsCmd(t, missing), "", "")
	if cleanup != nil {
		defer cleanup()
	}
	require.Error(t, src.RecipeErr)
	assert.Contains(t, src.RecipeErr.Error(), missing)
}
