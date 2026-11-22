package project

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A token that stands for one name is filled from a file's name, which the
// repository chooses. A name that is not one safe segment is refused rather
// than let climb out of the directory the template names.
func TestTargetTemplateRefusesAnUnsafeSegment(t *testing.T) {
	for _, tc := range []struct {
		name, target, source, lang, token string
	}{
		{"a stem of ..", "out/{lang}/{name}/x.json", "src/...json", "fr", "{name}"},
		{"a stem of .. as {basename}", "out/{basename}/x.json", "src/...json", "fr", "{basename}"},
		{"a stem of .. as *", "out/*/x.json", "src/...json", "fr", "*"},
		{"a language that climbs", "out/{lang}/{name}.json", "src/a.json", "../../etc", "{lang}"},
		{"a language with a separator", "out/{lang}.json", "src/a.json", "fr/x", "{lang}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveTargetPath("src/*.json", "", tc.target, tc.source, tc.lang)
			var use *UnsafeSegmentError
			require.ErrorAs(t, err, &use)
			assert.Equal(t, tc.token, use.Token)
		})
	}

	t.Run("a safe name is expanded", func(t *testing.T) {
		got, err := ResolveTargetPath("src/*.json", "", "out/{lang}/{name}/x.json", "src/a.json", "fr")
		require.NoError(t, err)
		assert.Equal(t, "out/fr/a/x.json", filepath.ToSlash(got))
	})

	t.Run("a file with no extension leaves {ext} empty", func(t *testing.T) {
		got, err := ExpandTemplate("out/{name}{ext}", "src/Makefile")
		require.NoError(t, err)
		assert.Equal(t, "out/Makefile", got)
	})
}
