package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/ref/refcache"
)

// TestGovernanceWatchReportsWhatMovedSinceTheLastRead pins the comparison the
// retrieval surface makes: the difference between two reads by ONE reader.
// A first read has nothing to be behind, a repeat of a change is not a second
// change, and the position is never governance.
func TestGovernanceWatchReportsWhatMovedSinceTheLastRead(t *testing.T) {
	tests := []struct {
		name  string
		reads []ref.Ref
		want  [][]ref.Component
	}{
		{
			name:  "the first read reports nothing",
			reads: []ref.Ref{{Context: "ctx", Terms: "trm"}},
			want:  [][]ref.Component{nil},
		},
		{
			name: "an unchanged graph reports nothing",
			reads: []ref.Ref{
				{Context: "ctx", Terms: "trm"},
				{Context: "ctx", Terms: "trm"},
			},
			want: [][]ref.Component{nil, nil},
		},
		{
			name: "a moved component is named",
			reads: []ref.Ref{
				{Context: "ctx", Terms: "trm"},
				{Context: "ctx-2", Terms: "trm"},
			},
			want: [][]ref.Component{nil, {ref.ComponentContext}},
		},
		{
			name: "a change is reported once, not on every answer after it",
			reads: []ref.Ref{
				{Context: "ctx"},
				{Context: "ctx-2"},
				{Context: "ctx-2"},
			},
			want: [][]ref.Component{nil, {ref.ComponentContext}, nil},
		},
		{
			name: "content traffic alone never reads as moved governance",
			reads: []ref.Ref{
				{Content: 1, Context: "ctx", Terms: "trm", Decisions: "dec"},
				{Content: 97, Context: "ctx", Terms: "trm", Decisions: "dec"},
			},
			want: [][]ref.Component{nil, nil},
		},
		{
			name: "several components move together",
			reads: []ref.Ref{
				{Context: "ctx", Terms: "trm", Decisions: "dec"},
				{Context: "ctx-2", Terms: "trm-2", Decisions: "dec"},
			},
			want: [][]ref.Component{nil, {ref.ComponentContext, ref.ComponentTerms}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &governanceWatch{}
			for i, r := range tt.reads {
				assert.Equal(t, tt.want[i], w.observe(r), "read %d", i)
			}
		})
	}
}

// TestGovernanceNotesReadTheCacheAndNotTheNetwork is the freshness contract:
// the note comes from the on-disk ref cache another process refreshes, so a
// retrieval pays a file read and never a round trip.
func TestGovernanceNotesReadTheCacheAndNotTheNetwork(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"),
		[]byte("version: v1\nname: freshness\n"), 0o644))
	t.Setenv("KAPI_PROJECT", filepath.Join(root, "kapi.yaml"))

	layout := project.LayoutAt(root)
	write := func(r ref.Ref) {
		cache := refcache.Load(layout, "https://venue.test", "p1")
		cache.Record("main", r)
		require.NoError(t, cache.Save(layout))
	}

	a := &App{}
	cmd := NewEnvCommand(context.Background(), "freshness-test")

	assert.Empty(t, a.governanceNotes(cmd), "no cache at all is not a staleness report")

	write(ref.Ref{Context: "ctx", Terms: "trm", Decisions: "dec"})
	assert.Empty(t, a.governanceNotes(cmd), "the first read establishes the baseline and reports nothing")

	write(ref.Ref{Context: "ctx", Terms: "trm-2", Decisions: "dec"})
	notes := a.governanceNotes(cmd)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "terms moved")
	assert.Contains(t, notes[0], "Re-read the project's context")

	assert.Empty(t, a.governanceNotes(cmd), "the same change is not reported twice")
}

// TestGovernanceNotesIgnoreAnUnobservedCache holds the note to the one thing it
// can honestly claim. A project that has never reached its venue holds no
// governance identities, and reporting that absence as movement would put a
// staleness note on every answer a disconnected project ever gave.
func TestGovernanceNotesIgnoreAnUnobservedCache(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"),
		[]byte("version: v1\nname: freshness\n"), 0o644))
	t.Setenv("KAPI_PROJECT", filepath.Join(root, "kapi.yaml"))

	layout := project.LayoutAt(root)
	cache := refcache.Load(layout, "https://venue.test", "p1")
	cache.Consume("main", 42) // a position, and no governance at all
	require.NoError(t, cache.Save(layout))

	a := &App{}
	cmd := NewEnvCommand(context.Background(), "freshness-test")
	assert.Empty(t, a.governanceNotes(cmd))
	assert.Empty(t, a.governanceNotes(cmd))
}

// TestGovernanceNotesReadACacheFromAnyDestination: the retrieval surface reads
// the graph, not the transport's configuration. It does not know which server
// or stream the recipe binds, and a cache it refused to read for that reason
// would make every project on a named stream permanently unable to report that
// its context had moved.
func TestGovernanceNotesReadACacheFromAnyDestination(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"),
		[]byte("version: v1\nname: freshness\n"), 0o644))
	t.Setenv("KAPI_PROJECT", filepath.Join(root, "kapi.yaml"))

	layout := project.LayoutAt(root)
	write := func(r ref.Ref) {
		cache := refcache.Load(layout, "https://venue.test", "p1")
		cache.Record("release-2026", r)
		require.NoError(t, cache.Save(layout))
	}

	a := &App{}
	cmd := NewEnvCommand(context.Background(), "freshness-test")

	write(ref.Ref{Context: "ctx"})
	require.Empty(t, a.governanceNotes(cmd))

	write(ref.Ref{Context: "ctx-2"})
	notes := a.governanceNotes(cmd)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "context moved")
}

// TestListComponentsReadsAsProse: the note is prose an assistant acts on.
func TestListComponentsReadsAsProse(t *testing.T) {
	tests := []struct {
		name string
		in   []ref.Component
		want string
	}{
		{name: "none", in: nil, want: ""},
		{name: "one", in: []ref.Component{ref.ComponentTerms}, want: "terms"},
		{
			name: "two",
			in:   []ref.Component{ref.ComponentContext, ref.ComponentTerms},
			want: "context and terms",
		},
		{
			name: "three",
			in:   ref.GovernanceComponents(),
			want: "context, terms and decisions",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, listComponents(tt.in))
		})
	}
}

// TestSearchContextLeadsWithFreshness: the note that says the rest of the
// answer may be about a graph that has moved comes first, because a caller that
// reads one note reads the first one.
func TestSearchContextLeadsWithFreshness(t *testing.T) {
	res, err := SearchContext(context.Background(), ContextSearchSources{
		Freshness: []string{"terms moved since this was last read here"},
	}, ContextSearchRequest{Query: "anything"})
	require.NoError(t, err)
	require.NotEmpty(t, res.Notes)
	assert.Equal(t, "terms moved since this was last read here", res.Notes[0])
}
