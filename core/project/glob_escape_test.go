//go:build unix

package project

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// escapeTree makes a project root with one real catalog, a file link and a
// directory link that both lead out of the root, and one link that stays in.
func escapeTree(t *testing.T) (root, outside string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "project")
	outside = filepath.Join(base, "outside")
	createFile(t, root, "src/en.json", "{}")
	createFile(t, root, "shared/common.json", "{}")
	createFile(t, outside, "secret.json", `{"secret":"outside"}`)
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.json"), filepath.Join(root, "src", "fr.json")))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "src", "vendor")))
	require.NoError(t, os.Symlink(filepath.Join("..", "shared", "common.json"), filepath.Join(root, "src", "common.json")))
	return root, outside
}

// A link that leads out of the root is never listed, whether it names a file
// or a directory, and whether or not a pattern names it before a wildcard. A
// link that stays inside the root is listed.
func TestExpandGlobListsNoLinkOutOfTheRoot(t *testing.T) {
	root, _ := escapeTree(t)

	for _, pattern := range []string{"src/*.json", "src/**/*.json", "src/vendor/*.json", "src/fr.json"} {
		got, err := ExpandGlob(root, pattern)
		require.NoError(t, err, pattern)
		for _, p := range got {
			assert.NotEqual(t, "src/fr.json", p, "%s lists a link out of the root", pattern)
			assert.NotContains(t, p, "vendor", "%s reads through a directory link out of the root", pattern)
		}
	}
	got, err := ExpandGlob(root, "src/*.json")
	require.NoError(t, err)
	sort.Strings(got)
	assert.Equal(t, []string{"src/common.json", "src/en.json"}, got)
}

// With FilesOnly, a link out of the root is reported to OnUnreadable, so it is
// named rather than silently dropped.
func TestExpandGlobReportsALinkOutOfTheRoot(t *testing.T) {
	root, _ := escapeTree(t)
	var unreadable []string
	got, err := ExpandGlobs(root, []string{"src/*.json"}, GlobOptions{
		FilesOnly:    true,
		OnUnreadable: func(rel string, _ error) { unreadable = append(unreadable, rel) },
	})
	require.NoError(t, err)
	sort.Strings(got[0])
	assert.Equal(t, []string{"src/common.json", "src/en.json"}, got[0])
	assert.Equal(t, []string{"src/fr.json"}, unreadable)
}
