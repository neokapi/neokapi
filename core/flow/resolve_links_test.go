package flow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A step-config path in a project flow is judged where it resolves. A link
// in the project that leads outside it is refused, and a link that stays
// inside resolves to its target, so the tool gets a path with no link on it.
func TestResolveToolConfig_PathsResolveThroughLinks(t *testing.T) {
	project := realTempDir(t)
	outside := realTempDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(outside, "store.db"), nil, 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(project, "escape")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "store.db"), filepath.Join(project, "store.db")))
	require.NoError(t, os.MkdirAll(filepath.Join(project, "data"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, "data", "real.db"), nil, 0o644))
	require.NoError(t, os.Symlink(filepath.Join("data", "real.db"), filepath.Join(project, "inside.db")))
	ctx := ResourceContext{ProjectDir: project}

	for _, tc := range []struct{ name, value string }{
		{"a file beneath a directory link out", "escape/store.db"},
		{"a new file beneath a directory link out", "escape/new.html"},
		{"a file link out", "store.db"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveToolConfig(map[string]any{"outputPath": tc.value}, nil, ctx)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "outputPath")
			assert.Contains(t, err.Error(), "leads outside the project")
		})
	}

	t.Run("a link inside resolves to its target", func(t *testing.T) {
		got, err := ResolveToolConfig(map[string]any{"outputPath": "inside.db"}, nil, ctx)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(project, "data", "real.db"), got["outputPath"])
	})
	t.Run("a new file inside resolves", func(t *testing.T) {
		got, err := ResolveToolConfig(map[string]any{"outputPath": "reports/r.html"}, nil, ctx)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(project, "reports", "r.html"), got["outputPath"])
	})
}

// A named resource is one of the person's own stores, by name only. It must
// exist as a regular file inside its kind's directory; a recipe creates
// neither the store nor the directory.
func TestResolveNamedResource_NamesAnExistingStore(t *testing.T) {
	config := realTempDir(t)
	t.Setenv("KAPI_CONFIG_DIR", config)
	outside := realTempDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.db"), nil, 0o644))
	writeStore(t, config, "terms", "product.db")
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.db"), filepath.Join(config, "terms", "linked.db")))
	require.NoError(t, os.MkdirAll(filepath.Join(config, "terms", "folder.db"), 0o755))

	got, err := resolveNamedResource("terms", "product")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(config, "terms", "product.db"), got)

	for _, tc := range []struct{ name, says string }{
		{"missing", "does not exist"},
		{"linked", "leads outside"},
		{"folder", "not a regular file"},
		{"..", "plain name"},
		{".", "plain name"},
		{"a/b", "plain name"},
		{`a\b`, "plain name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveNamedResource("terms", tc.name)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.says)
		})
	}

	t.Run("the directory of a kind is not created", func(t *testing.T) {
		_, err := resolveNamedResource("memory", "project-memory")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
		assert.NoDirExists(t, filepath.Join(config, "memory"))
	})
}
