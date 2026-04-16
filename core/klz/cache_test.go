//go:build klzcache

package klz

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/klf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWarmCache_BuildsOnDiskEntry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("NEOKAPI_KLZ_CACHE", tmp)

	data := buildExampleArchive(t)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer r.Close()

	require.NoError(t, r.WarmCache(context.Background()))

	// Cache entry must exist under the sharded path.
	hash := r.ManifestHash()
	dbPath := filepath.Join(tmp, hash[:2], hash[2:], "db.sqlite")
	_, err = os.Stat(dbPath)
	require.NoError(t, err, "cache entry should exist at %s", dbPath)
}

func TestBlockByID_RoutesThroughCache(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("NEOKAPI_KLZ_CACHE", tmp)

	data := buildExampleArchive(t)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer r.Close()

	// First lookup warms the cache transparently.
	block, err := r.BlockByID(context.Background(), "tag-chip")
	require.NoError(t, err)
	require.NotNil(t, block)
	assert.Equal(t, "tag-chip", block.ID)

	// Cache must now be on disk.
	hash := r.ManifestHash()
	dbPath := filepath.Join(tmp, hash[:2], hash[2:], "db.sqlite")
	_, err = os.Stat(dbPath)
	require.NoError(t, err)

	// Second lookup hits the cache; result is still correct.
	block2, err := r.BlockByID(context.Background(), "files-heading")
	require.NoError(t, err)
	require.NotNil(t, block2)
	assert.Equal(t, "files-heading", block2.ID)
}

func TestSimilarSources(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("NEOKAPI_KLZ_CACHE", tmp)

	data := buildExampleArchive(t)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer r.Close()
	require.NoError(t, r.WarmCache(context.Background()))

	// The source texts contain "Files" and "cart", so a query for
	// "cart" should return the shopping-cart block.
	hits, err := r.SimilarSources(context.Background(), "items cart", "en", 5)
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	var found bool
	for _, h := range hits {
		if h.ID == "shopping-cart-plural" {
			found = true
		}
	}
	assert.True(t, found, "similar sources should include shopping-cart-plural")
}

func TestTMMatch(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("NEOKAPI_KLZ_CACHE", tmp)

	data := buildExampleArchive(t)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer r.Close()
	require.NoError(t, r.WarmCache(context.Background()))

	tm := r.TM()
	require.NotNil(t, tm)

	matches, err := tm.Match(context.Background(), "Files matched", "de", 5)
	require.NoError(t, err)
	// buildExampleArchive writes a German target overlay for
	// files-heading. The TM should surface it here.
	var found bool
	for _, m := range matches {
		if m.BlockID == "files-heading" && m.Locale == "de" {
			found = true
			assert.NotEmpty(t, m.TargetRuns)
		}
	}
	assert.True(t, found, "TM should find files-heading's German overlay")
}

func TestCacheGC_EvictsByMaxBytes(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("NEOKAPI_KLZ_CACHE", tmp)

	// Build two distinct archives so we get two cache entries.
	data1 := buildExampleArchive(t)
	r1, err := NewReader(bytes.NewReader(data1), int64(len(data1)))
	require.NoError(t, err)
	require.NoError(t, r1.WarmCache(context.Background()))
	_ = r1.Close()

	// Perturb archive 2 by adding an extra part.
	w := NewWriter(WriterOptions{
		Generator: r1.Manifest().Generator,
		Project:   r1.Manifest().Project,
		Created:   r1.Manifest().Created,
	})
	require.NoError(t, w.AddDocument("documents/examples.klf", exampleDocument(), nil))
	require.NoError(t, w.AddSkeleton("skeletons/other.skl", []byte("different skeleton"), nil))
	var buf bytes.Buffer
	_, err = w.Write(&buf)
	require.NoError(t, err)
	r2, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	require.NoError(t, r2.WarmCache(context.Background()))
	_ = r2.Close()

	entries, err := CacheEntries()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(entries), 2, "expected two cache entries")

	// Evict down to 1 byte → everything goes.
	report, err := CacheGC(context.Background(), 1, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, report.EvictedEntries, 1)

	after, err := CacheEntries()
	require.NoError(t, err)
	assert.Less(t, len(after), len(entries), "gc should have evicted entries")
}

func TestCacheSurvivesCorruption(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("NEOKAPI_KLZ_CACHE", tmp)

	data := buildExampleArchive(t)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer r.Close()
	require.NoError(t, r.WarmCache(context.Background()))

	// Corrupt the cache by overwriting db.sqlite with garbage.
	hash := r.ManifestHash()
	dbPath := filepath.Join(tmp, hash[:2], hash[2:], "db.sqlite")
	require.NoError(t, os.WriteFile(dbPath, []byte("not sqlite"), 0o644))

	// Lookups should still work — the cache is advisory, not
	// authoritative. Iteration-side fallback kicks in on any
	// cache error.
	block, err := r.BlockByID(context.Background(), "files-heading")
	require.NoError(t, err)
	require.NotNil(t, block)
	assert.Equal(t, "files-heading", block.ID)
}

// exampleDocument, buildExampleArchive, and other fixtures are
// defined in klz_test.go in this package.
var _ = klf.SchemaVersion
