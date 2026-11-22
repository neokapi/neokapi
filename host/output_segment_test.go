package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// A person's -o template is filled from each input's name, which the
// repository chooses. A name that is not one safe segment is refused before
// anything is resolved, so a file named "...json" cannot put its stem ".."
// in place of {name} and climb above the directory the person named.
func TestOutputTemplateRefusesAnUnsafeName(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "src", "...json")
	for _, tmpl := range []string{
		filepath.Join(dir, "out", "{name}", "x.json"),
		filepath.Join(dir, "out", "*", "x.json"),
	} {
		_, err := expandAdhocOutputTemplate(tmpl, input, filepath.Dir(input), "fr")
		var use *project.UnsafeSegmentError
		require.ErrorAs(t, err, &use, tmpl)
		assert.Contains(t, err.Error(), "not a safe path segment")

		_, err = expandOutputPath(tmpl, input, filepath.Dir(input), "fr")
		require.ErrorAs(t, err, &use, tmpl)
	}

	got, err := expandAdhocOutputTemplate(filepath.Join(dir, "out", "{name}", "x.json"), filepath.Join(dir, "src", "app.json"), "", "fr")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "out", "app", "x.json"), got)
}

// A package's source name is the repository's choice too: a merge layout
// that places {name} refuses one that is not a safe segment.
func TestMergeOutputPathRefusesAnUnsafeName(t *testing.T) {
	_, err := mergeOutputPath("out/{lang}/{name}/x.{ext}", "src/...json", "fr", false)
	require.ErrorContains(t, err, "merge: source name")
}
