package pluginhost_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Plugin discovery must use install-time caching, not per-run manifest parsing
// (AD-007: the dispatch cache "skips manifest parsing entirely" when no root
// changed). This guards against a regression that reintroduces a per-invocation
// parse — which would add startup latency proportional to the number of
// installed plugins.
//
// The check is deterministic, not a wall-clock timer: it primes the cache, then
// corrupts a manifest's CONTENT without touching any directory mtime (a content
// rewrite is not a dir-entry change, so the mtime freshness check still passes).
// A warm start that honors the cache returns the good cached plugin; a warm
// start that re-parses per run would hit the corrupt manifest and drop it.
func TestRuntime_WarmStartUsesCacheNoReparse(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // isolate CacheLocation()
	tmp := t.TempDir()
	writeManifest(t, tmp, "fakebridge", fakeBridgeManifest)
	manifestPath := filepath.Join(tmp, "fakebridge", "manifest.json")

	opts := isolatedOpts(tmp)

	// Cold start: discovers, parses, and writes the install-time cache.
	rt1 := pluginhost.NewRuntime(pluginhost.RuntimeOptions{
		Discover: opts, FormatReg: registry.NewFormatRegistry(), UseCache: true,
	})
	require.Len(t, rt1.Rescan().Plugins(), 1)
	rt1.Shutdown()

	// Corrupt the manifest content only — no dir entry added/removed/renamed,
	// so the discovery root's mtime is unchanged and the cache stays "fresh".
	require.NoError(t, os.WriteFile(manifestPath, []byte("{ not valid json"), 0o644))

	// Warm start (fresh Runtime → no in-memory state): must serve from the
	// cache and never read/parse the now-corrupt manifest.
	rt2 := pluginhost.NewRuntime(pluginhost.RuntimeOptions{
		Discover: opts, FormatReg: registry.NewFormatRegistry(), UseCache: true,
	})
	defer rt2.Shutdown()
	plugins := rt2.Rescan().Plugins()
	require.Len(t, plugins, 1, "warm start must load plugins from the cache, not re-parse manifests per run")
	assert.Equal(t, "fakebridge", plugins[0].Name())
}

// BenchmarkRuntime_WarmDiscover measures the warm-start path (cache present,
// roots unchanged). It should stay flat as plugins are added — a jump signals
// per-run manifest parsing has returned.
func BenchmarkRuntime_WarmDiscover(b *testing.B) {
	b.Setenv("XDG_CACHE_HOME", b.TempDir())
	tmp := b.TempDir()
	dir := filepath.Join(tmp, "fakebridge")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(fakeBridgeManifest), 0o644); err != nil {
		b.Fatal(err)
	}
	opts := isolatedOpts(tmp)

	// Prime the cache once.
	pluginhost.NewRuntime(pluginhost.RuntimeOptions{Discover: opts, UseCache: true}).Rescan()

	b.ResetTimer()
	for range b.N {
		rt := pluginhost.NewRuntime(pluginhost.RuntimeOptions{Discover: opts, UseCache: true})
		rt.Rescan()
		rt.Shutdown()
	}
}
