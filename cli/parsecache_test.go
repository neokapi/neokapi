package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeCacheProject scaffolds a project WITH a .kapi/ state dir (so the parse
// cache's layout resolution succeeds) and a fully-translated nb target.
func writeCacheProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	recipe := `version: v1
name: cache
defaults:
  source_language: en
  target_languages: [nb]
content:
  - path: en.json
    target: "{lang}.json"
ship_gate: { translated: 100 }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "proj.kapi"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	return root
}

func countCacheRows(t *testing.T, root string) int {
	t.Helper()
	c, err := openDocCache(filepath.Join(root, ".kapi", "cache"))
	require.NoError(t, err)
	defer c.close()
	var n int
	require.NoError(t, c.db.QueryRow(`SELECT COUNT(*) FROM documents`).Scan(&n))
	return n
}

func TestStatus_PopulatesAndReusesParseCache(t *testing.T) {
	root := writeCacheProject(t)
	t.Chdir(root)

	// First status populates the cache (source + target file entries).
	first := runStatusJSON(t)
	assert.Positive(t, countCacheRows(t, root), "status populates the parse cache")

	// Second status is byte-identical (cache hits must not change the answer).
	second := runStatusJSON(t)
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	assert.JSONEq(t, string(a), string(b), "cached re-run gives the identical result")

	nb, ok := locale(second, "nb")
	require.True(t, ok)
	assert.Equal(t, 100, nb.Pct["translated"])
}

func TestStatus_ParseCacheIncrementalAndRebuildable(t *testing.T) {
	root := writeCacheProject(t)
	t.Chdir(root)

	_ = runStatusJSON(t) // warm the cache

	// Change the source: add a third key, leaving nb with only 2 of 3 → 67%.
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana","c":"Cherry"}`), 0o644))
	afterEdit := runStatusJSON(t)
	nb, ok := locale(afterEdit, "nb")
	require.True(t, ok)
	assert.Equal(t, 3, nb.Total, "the changed source re-parsed (cache invalidated)")
	assert.Equal(t, 67, nb.Pct["translated"], "2 of 3 translated after the edit")

	// Rebuild invariant: delete the cache, re-read → identical result.
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".kapi", "cache")))
	rebuilt := runStatusJSON(t)
	a, _ := json.Marshal(afterEdit)
	b, _ := json.Marshal(rebuilt)
	assert.JSONEq(t, string(a), string(b), "a rebuilt cache yields the identical result")
}
