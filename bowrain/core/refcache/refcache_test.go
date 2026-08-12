package refcache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref"
)

func testLayout(t *testing.T) coreproj.Layout {
	t.Helper()
	return coreproj.LayoutAt(filepath.Join(t.TempDir(), "project"))
}

func TestRoundTrip(t *testing.T) {
	layout := testLayout(t)

	cache := Load(layout, "https://example.test", "p1")
	cache.Observe("main", ref.Ref{Content: 999, Context: "ctx", Terms: "trm", Decisions: "dec"})
	cache.Consume("main", 42)
	cache.Touch(time.Now())
	require.NoError(t, cache.Save(layout))

	reloaded := Load(layout, "https://example.test", "p1")
	got := reloaded.Ref("main")
	assert.Equal(t, int64(42), got.Content, "the position is what this project consumed, never what the server published")
	assert.Equal(t, "ctx", got.Context)
	assert.Equal(t, "trm", got.Terms)
	assert.Equal(t, "dec", got.Decisions)
	assert.False(t, reloaded.ObservedAt.IsZero())
}

func TestDestinationKeyed(t *testing.T) {
	layout := testLayout(t)

	cache := Load(layout, "https://one.test", "p1")
	cache.Observe("main", ref.Ref{Context: "ctx"})
	cache.Consume("main", 7)
	require.NoError(t, cache.Save(layout))

	tests := []struct {
		name      string
		serverURL string
		projectID string
		wantEmpty bool
	}{
		{name: "same destination keeps the refs", serverURL: "https://one.test", projectID: "p1"},
		{name: "another server discards them", serverURL: "https://two.test", projectID: "p1", wantEmpty: true},
		{name: "another project discards them", serverURL: "https://one.test", projectID: "p2", wantEmpty: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Load(layout, tt.serverURL, tt.projectID).Ref("main")
			if tt.wantEmpty {
				assert.True(t, got.IsZero(), "a ref describing another destination must not survive")
				return
			}
			assert.Equal(t, int64(7), got.Content)
			assert.Equal(t, "ctx", got.Context)
		})
	}
}

func TestMissingAndCorruptFilesCostOneRoundTripNotAWrongAnswer(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		layout := testLayout(t)
		assert.True(t, Load(layout, "https://one.test", "p1").Ref("main").IsZero())
	})

	t.Run("corrupt", func(t *testing.T) {
		layout := testLayout(t)
		require.NoError(t, os.MkdirAll(layout.CacheDir(), 0o755))
		require.NoError(t, os.WriteFile(PathFor(layout), []byte("{not json"), 0o644))

		cache := Load(layout, "https://one.test", "p1")
		assert.True(t, cache.Ref("main").IsZero())
		// And it is usable, not poisoned: the next contact repopulates it.
		cache.Observe("main", ref.Ref{Terms: "t"})
		assert.Equal(t, "t", cache.Ref("main").Terms)
	})
}

func TestObserveLeavesThePositionAlone(t *testing.T) {
	cache := &Cache{}
	cache.Consume("main", 10)
	cache.Observe("main", ref.Ref{Content: 5000, Context: "c"})
	assert.Equal(t, int64(10), cache.Ref("main").Content)
	assert.Equal(t, "c", cache.Ref("main").Context)
}

func TestObserveSkipsEmptyIdentitiesButSetIdentityDoesNot(t *testing.T) {
	cache := &Cache{}
	cache.Observe("main", ref.Ref{Context: "c", Terms: "t", Decisions: "d"})

	// A degraded answer says nothing about the components it omits.
	cache.Observe("main", ref.Ref{Context: "c2"})
	assert.Equal(t, "c2", cache.Ref("main").Context)
	assert.Equal(t, "t", cache.Ref("main").Terms)
	assert.Equal(t, "d", cache.Ref("main").Decisions)

	// A component that genuinely holds nothing is recorded as holding nothing.
	cache.SetIdentity("main", ref.ComponentDecisions, "")
	assert.Empty(t, cache.Ref("main").Decisions)
}

func TestConsumeIsMonotonicAndResetIsTheWayBack(t *testing.T) {
	cache := &Cache{}
	cache.Consume("main", 30)
	cache.Consume("main", 12)
	assert.Equal(t, int64(30), cache.Ref("main").Content)

	// A write whose answer carries no position must not rewind the stream.
	cache.Consume("main", 0)
	assert.Equal(t, int64(30), cache.Ref("main").Content)

	cache.Reset("main")
	assert.Equal(t, int64(0), cache.Ref("main").Content)
}

func TestStreamsAreIndependentAndTheUnnamedStreamIsMain(t *testing.T) {
	cache := &Cache{}
	cache.Consume("", 5)
	cache.Observe("release", ref.Ref{Context: "rc"})

	assert.Equal(t, int64(5), cache.Ref("main").Content)
	assert.Equal(t, int64(5), cache.Ref("").Content)
	assert.Equal(t, int64(0), cache.Ref("release").Content)
	assert.Equal(t, "rc", cache.Ref("release").Context)
	assert.Empty(t, cache.Ref("main").Context)
}
