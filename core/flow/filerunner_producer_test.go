package flow_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
)

// The runner keys a parse by what produced it: a run whose producer changed
// records the file again, and one whose producer cannot be named is never
// recorded.
func TestFileRunner_KeysAParseByItsProducer(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	inputPath := filepath.Join(t.TempDir(), "input.json")
	require.NoError(t, os.WriteFile(inputPath, []byte(`{"greeting":"Hello World"}`), 0o644))
	pseudo := func() tool.Tool {
		pt, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "qps"}, "qps")
		require.NoError(t, err)
		return pt
	}
	cache := newMemPartCache()
	run := func(producer string, cacheable bool) {
		s, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
		require.NoError(t, err)
		t.Cleanup(func() { s.Close() })
		r := flow.NewFileRunner(flow.FileRunnerConfig{
			FormatReg: reg, SourceLocale: "en-US", Store: s,
			PartCache: cache, PartCacheKey: "k",
			FormatProducer: func(string) (string, bool) { return producer, cacheable },
		})
		require.NoError(t, r.RunFileProcessOnly(context.Background(), "pseudo-translate", []tool.Tool{pseudo()}, inputPath, "qps"))
	}

	run("plugin@1", true)
	run("plugin@1", true)
	assert.Equal(t, 1, cache.records, "the producer unchanged, the second run is served from the cache")
	run("plugin@2", true)
	assert.Equal(t, 2, cache.records, "a new producer records the file again")
	run("", false)
	assert.Equal(t, 2, cache.records, "a producer that cannot be named records nothing")
}
