package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCache_StalenessLadder(t *testing.T) {
	dir := t.TempDir()
	c, err := openParseCache(filepath.Join(dir, "cache"))
	require.NoError(t, err)
	defer c.close()

	f := filepath.Join(dir, "x.json")
	require.NoError(t, os.WriteFile(f, []byte(`{"a":"Apple"}`), 0o644))
	st, _ := os.Stat(f)
	c.put(f, "k", st, []*model.Block{model.NewBlock("a", "Apple")})

	// Same file → hit.
	got, ok := c.get(f, "k", st)
	require.True(t, ok)
	require.Len(t, got, 1)
	assert.Equal(t, "Apple", got[0].SourceText())

	// Touch (new mtime, same bytes) → still a hit (re-hash path, no re-parse).
	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(f, future, future))
	st2, _ := os.Stat(f)
	_, ok = c.get(f, "k", st2)
	assert.True(t, ok, "touch with identical bytes stays a hit")

	// Changed bytes → miss.
	require.NoError(t, os.WriteFile(f, []byte(`{"a":"Banana","b":"Cherry"}`), 0o644))
	st3, _ := os.Stat(f)
	_, ok = c.get(f, "k", st3)
	assert.False(t, ok, "changed content is a miss")

	// Different config key → miss (the key includes the config).
	_, ok = c.get(f, "other-config", st)
	assert.False(t, ok)
}

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
	c, err := openParseCache(filepath.Join(root, ".kapi", "cache"))
	require.NoError(t, err)
	defer c.close()
	var n int
	require.NoError(t, c.db.QueryRow(`SELECT COUNT(*) FROM file_blocks`).Scan(&n))
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
