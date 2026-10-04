package state_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/storage"
)

// countingJournal applies what it is handed straight to the database, as the
// projector does after recording, and counts the adoptions it was asked to
// record.
type countingJournal struct {
	db        *storage.DB
	adoptions []state.Adoption
}

func (j *countingJournal) RecordEntries(ctx context.Context, entries []state.JournalEntry) error {
	return state.ApplyEntries(ctx, j.db, entries)
}

func (j *countingJournal) RecordAdoptions(ctx context.Context, adoptions []state.Adoption) error {
	for i := range adoptions {
		adoptions[i].At = time.Now()
	}
	j.adoptions = append(j.adoptions, adoptions...)
	return state.ApplyAdoptions(ctx, j.db, adoptions)
}

// sharedProject is one context database and a checkout handle per record
// directory, each recording through one journal, as a workspace serves the
// checkouts of one project.
type sharedProject struct {
	t       *testing.T
	db      *storage.DB
	root    string
	journal *countingJournal
}

func newSharedProject(t *testing.T) *sharedProject {
	t.Helper()
	root := t.TempDir()
	db, err := storage.OpenWith(filepath.Join(root, "context.db"), storage.ProjectOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return &sharedProject{t: t, db: db, root: root, journal: &countingJournal{db: db}}
}

func (p *sharedProject) checkout(name string) *state.WorkStore {
	p.t.Helper()
	w, err := state.OpenWorkFromDB(p.t.Context(), p.db, filepath.Join(p.root, name, ".kapi", "state"))
	require.NoError(p.t, err)
	w.SetJournal(p.journal)
	return w
}

func TestAFreshCheckoutTakesTheKeyTheProjectUses(t *testing.T) {
	ctx := t.Context()
	p := newSharedProject(t)
	first := p.checkout("first")

	keys, err := first.AdoptDocuments(ctx, []reconcile.DocUnit{{Path: "docs/intro.md", Content: []string{"h1", "h2", "h3"}}})
	require.NoError(t, err)
	key := keys["docs/intro.md"]
	// The first checkout follows a rename, and keeps the key.
	keys, err = first.AdoptDocuments(ctx, []reconcile.DocUnit{{Path: "guides/intro.md", Content: []string{"h1", "h2", "h3"}}})
	require.NoError(t, err)
	require.Equal(t, key, keys["guides/intro.md"])
	require.NotEqual(t, reconcile.DocumentKeyFor("guides/intro.md"), key)

	tests := []struct {
		name string
		read reconcile.DocUnit
	}{
		{name: "at the path another checkout read it", read: reconcile.DocUnit{Path: "guides/intro.md", Content: []string{"h1", "h2", "h3"}}},
		{name: "by what it holds, at a path nobody recorded", read: reconcile.DocUnit{Path: "manual/intro.md", Content: []string{"h1", "h2", "h4"}}},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fresh := p.checkout("fresh-" + string(rune('a'+i)))
			keys, err := fresh.AdoptDocuments(ctx, []reconcile.DocUnit{tc.read})
			require.NoError(t, err)
			assert.Equal(t, key, keys[tc.read.Path], "a checkout that has seen nothing takes the project's key, not one minted from the path")
		})
	}
}

// TestANewFileAtAMovedDocumentsOldPathTakesAKeyOfItsOwn: one checkout follows
// a document from docs/intro.md to guides/intro.md, so the key docs/intro.md
// minted belongs to it. A fresh checkout reading only a new file at
// docs/intro.md (another branch, a partial read) mints a key of its own, and
// the project's adoption of the moved document stands for every checkout
// after it.
func TestANewFileAtAMovedDocumentsOldPathTakesAKeyOfItsOwn(t *testing.T) {
	ctx := t.Context()
	p := newSharedProject(t)
	first := p.checkout("first")
	keys, err := first.AdoptDocuments(ctx, []reconcile.DocUnit{{Path: "docs/intro.md", Content: []string{"h1", "h2", "h3"}}})
	require.NoError(t, err)
	moved := keys["docs/intro.md"]
	keys, err = first.AdoptDocuments(ctx, []reconcile.DocUnit{{Path: "guides/intro.md", Content: []string{"h1", "h2", "h3"}}})
	require.NoError(t, err)
	require.Equal(t, moved, keys["guides/intro.md"])

	keys, err = p.checkout("fresh").AdoptDocuments(ctx, []reconcile.DocUnit{{Path: "docs/intro.md", Content: []string{"x1", "x2"}}})
	require.NoError(t, err)
	assert.NotEqual(t, moved, keys["docs/intro.md"], "a new document takes a key no other document holds")
	assert.True(t, reconcile.IsDocumentKey(keys["docs/intro.md"]))

	keys, err = p.checkout("third").AdoptDocuments(ctx, []reconcile.DocUnit{{Path: "guides/intro.md", Content: []string{"h1", "h2", "h3"}}})
	require.NoError(t, err)
	assert.Equal(t, moved, keys["guides/intro.md"], "the project's key for the moved document stands")
}

func TestADecisionAnswersInEveryCheckoutOfTheDocument(t *testing.T) {
	ctx := t.Context()
	p := newSharedProject(t)
	first := p.checkout("first")
	_, err := first.AdoptDocuments(ctx, []reconcile.DocUnit{{Path: "docs/intro.md", Content: []string{"h1"}}})
	require.NoError(t, err)
	keys, err := first.AdoptDocuments(ctx, []reconcile.DocUnit{{Path: "guides/intro.md", Content: []string{"h1"}}})
	require.NoError(t, err)
	decided := state.UnitState{
		Scope: keys["guides/intro.md"], Unit: "p#1", Variant: model.Variant("fr"),
		ContentHash: "h1", TargetHash: "t1", Status: model.TargetStatusEstablished,
		Decision: state.Decision{ReviewState: "approved", By: "reviewer"},
	}
	require.NoError(t, first.Put(ctx, decided))

	fresh := p.checkout("fresh")
	freshKeys, err := fresh.AdoptDocuments(ctx, []reconcile.DocUnit{{Path: "guides/intro.md", Content: []string{"h1"}}})
	require.NoError(t, err)
	got, found := fresh.Lookup(ctx, state.Key{Scope: freshKeys["guides/intro.md"], Unit: "p#1", Variant: model.Variant("fr")}, state.Reading{ContentHash: "h1", TargetHash: "t1"})
	require.True(t, found, "the other checkout's approval answers here, under the one key")
	assert.Equal(t, "reviewer", got.Decision.By)
}

// TestDocumentKeyAnswersAsTheDocumentsAndAdoptionsDo pins that the key of the
// document at one path is the one reading every document and adoption gives
// it: this checkout's own first, then the project's earliest adoption.
func TestDocumentKeyAnswersAsTheDocumentsAndAdoptionsDo(t *testing.T) {
	ctx := t.Context()
	p := newSharedProject(t)
	first := p.checkout("first")
	keys, err := first.AdoptDocuments(ctx, []reconcile.DocUnit{
		{Path: "docs/intro.md", Content: []string{"h1", "h2", "h3"}},
		{Path: "docs/setup.md", Content: []string{"s1", "s2"}},
	})
	require.NoError(t, err)
	// The first checkout follows a rename of the introduction.
	keys2, err := first.AdoptDocuments(ctx, []reconcile.DocUnit{
		{Path: "guides/intro.md", Content: []string{"h1", "h2", "h3"}},
		{Path: "docs/setup.md", Content: []string{"s1", "s2"}},
	})
	require.NoError(t, err)
	require.Equal(t, keys["docs/intro.md"], keys2["guides/intro.md"])
	fresh := p.checkout("fresh")

	tests := []struct {
		name  string
		w     *state.WorkStore
		path  string
		key   string
		found bool
	}{
		{name: "a document this checkout resolved", w: first, path: "guides/intro.md", key: keys["docs/intro.md"], found: true},
		{name: "a path only the project's adoptions name", w: fresh, path: "guides/intro.md", key: keys["docs/intro.md"], found: true},
		{name: "a path the key moved away from", w: fresh, path: "docs/intro.md"},
		{name: "a path nobody read", w: first, path: "docs/new.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, found, err := tt.w.DocumentKey(ctx, tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.key, key)

			// The same answer the whole listing gives.
			docs, err := tt.w.Documents(ctx)
			require.NoError(t, err)
			adopted, err := tt.w.AdoptedDocuments(ctx)
			require.NoError(t, err)
			byPath := map[string]string{}
			for _, d := range docs {
				byPath[d.Path] = d.Key
			}
			for _, d := range adopted {
				if _, known := byPath[d.Path]; !known {
					byPath[d.Path] = d.Key
				}
			}
			assert.Equal(t, byPath[tt.path], key)
		})
	}
}

func TestAdoptingWhatTheProjectHoldsRecordsNothing(t *testing.T) {
	ctx := t.Context()
	p := newSharedProject(t)
	read := []reconcile.DocUnit{
		{Path: "docs/a.md", Content: []string{"a1", "a2"}},
		{Path: "docs/b.md", Content: []string{"b1"}},
	}
	first := p.checkout("first")
	_, err := first.AdoptDocuments(ctx, read)
	require.NoError(t, err)
	require.Len(t, p.journal.adoptions, 2)
	a := p.journal.adoptions[0]
	assert.Equal(t, "docs/a.md", a.Path)
	assert.Equal(t, state.ContentDigest([]string{"a1", "a2"}), a.Digest)
	assert.Equal(t, []string{"a1", "a2"}, a.Content)

	_, err = first.AdoptDocuments(ctx, read)
	require.NoError(t, err)
	_, err = p.checkout("second").AdoptDocuments(ctx, read)
	require.NoError(t, err)
	assert.Len(t, p.journal.adoptions, 2, "a read the project already holds adopts nothing new")

	read[1].Content = []string{"b1", "b2"}
	_, err = first.AdoptDocuments(ctx, read)
	require.NoError(t, err)
	require.Len(t, p.journal.adoptions, 3, "a document read with other content is adopted again")
	assert.Equal(t, "docs/b.md", p.journal.adoptions[2].Path)

	adopted, err := first.AdoptedDocuments(ctx)
	require.NoError(t, err)
	require.Len(t, adopted, 2)
	assert.Equal(t, []string{"b1", "b2"}, adopted[1].Content, "a key holds its most recent adoption")
}

func TestApplyAdoptionsInAnyOrderLeavesTheSameRows(t *testing.T) {
	ctx := t.Context()
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	adoptions := []state.Adoption{
		{Key: "d-1", Path: "docs/a.md", Digest: "sha256:1", Content: []string{"a"}, At: at},
		{Key: "d-1", Path: "guides/a.md", Digest: "sha256:2", Content: []string{"a", "b"}, At: at.Add(time.Second)},
		{Key: "d-2", Path: "docs/b.md", Digest: "sha256:3", At: at.Add(2 * time.Second)},
	}
	read := func(order ...int) []reconcile.DocUnit {
		p := newSharedProject(t)
		w := p.checkout("reader")
		for _, i := range order {
			require.NoError(t, state.ApplyAdoptions(ctx, p.db, []state.Adoption{adoptions[i]}))
		}
		got, err := w.AdoptedDocuments(ctx)
		require.NoError(t, err)
		return got
	}
	forward := read(0, 1, 2)
	assert.Equal(t, forward, read(2, 1, 0))
	assert.Equal(t, forward, read(1, 0, 2))
	assert.Equal(t, []reconcile.DocUnit{
		{Key: "d-1", Path: "guides/a.md", Content: []string{"a", "b"}},
		{Key: "d-2", Path: "docs/b.md"},
	}, forward, "each key at its latest adoption, the earliest adopted first")
}
