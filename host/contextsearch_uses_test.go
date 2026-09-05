package host_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// usesConcepts is the vocabulary the usage tests count: one concept with a
// preferred and a deprecated English spelling and a Norwegian one.
func usesConcepts() []terms.Concept {
	return []terms.Concept{{
		ID:         "c-widget",
		Domain:     "product",
		Definition: "The unit users place on a dashboard.",
		Terms: []terms.Term{
			{Text: "gadget", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "widget", Locale: model.LocaleEnglish, Status: model.TermDeprecated},
			{Text: "dings", Locale: "nb", Status: model.TermApproved},
		},
	}}
}

// usesBlock is one block of the fixture's extracted content.
type usesBlock struct {
	hash, id, file, source string
	targets                map[string][]model.Run
}

func usesBlocks() []usesBlock {
	return []usesBlock{
		{hash: "h1", id: "guide.a", file: "docs/guide.md", source: "Drag a widget onto the board."},
		{hash: "h2", id: "guide.b", file: "docs/guide.md", source: "Every widget has a gadget inside."},
		{hash: "h3", id: "guide.c", file: "docs/guide.md", source: "Nothing of interest.",
			targets: map[string][]model.Run{"nb": {model.TextR("En dings her.")}}},
	}
}

// graphProject is a real project store holding the fixture's terms and blocks,
// with the context graph materialized over them: the stores a context search
// reads, exactly as `kapi up` leaves them.
type graphProject struct {
	app  *host.App
	root string
	proj *project.KapiProject
	src  host.ContextSearchSources
}

func newGraphProject(t *testing.T, proj *project.KapiProject, concepts []terms.Concept, blocks []usesBlock) *graphProject {
	t.Helper()
	ctx := t.Context()
	a := &host.App{}
	t.Cleanup(a.Shutdown)
	root := t.TempDir()
	require.NoError(t, project.EnsureLayout(project.LayoutAt(root)))
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	for _, c := range concepts {
		require.NoError(t, db.Terms().AddConcept(ctx, c))
	}
	g := &graphProject{app: a, root: root, proj: proj}
	g.putBlocks(t, blocks)
	g.materialize(t)
	graph, err := a.ProjectGraph(ctx, root)
	require.NoError(t, err)
	g.src = host.ContextSearchSources{
		Terms:      db.Terms(),
		Blocks:     db.BlocksAutocommit(),
		Graph:      graph,
		GraphScope: host.ProjectScope(proj),
	}
	return g
}

func (g *graphProject) putBlocks(t *testing.T, blocks []usesBlock) {
	t.Helper()
	ctx := t.Context()
	db, err := g.app.ProjectDB(ctx, g.root)
	require.NoError(t, err)
	sess, err := db.Blocks().Begin(ctx)
	require.NoError(t, err)
	for _, bc := range blocks {
		b := &blockstore.Block{Hash: bc.hash, ID: bc.id, Translatable: true,
			Source: []model.Run{model.TextR(bc.source)}, Targets: bc.targets}
		b.Properties.File = bc.file
		require.NoError(t, sess.PutBlock("docs", b))
	}
	require.NoError(t, sess.Commit())
}

func (g *graphProject) materialize(t *testing.T) {
	t.Helper()
	_, err := g.app.MaterializeContextGraph(t.Context(), g.root, g.proj)
	require.NoError(t, err)
}

func hitsByTerm(res *host.ContextSearchResult) map[string]host.ContextTermHit {
	out := map[string]host.ContextTermHit{}
	for _, h := range res.Terms {
		out[h.Term] = h
	}
	return out
}

// A discouraged word with nothing using it is a settled question; the same word
// in three places is work. The count is what tells them apart, so it belongs in
// the same answer as the verdict, and it is read from the graph extraction
// wrote: the same edges the platform's concept page reads.
func TestSearchContextCountsTermUsesFromTheGraph(t *testing.T) {
	p := newGraphProject(t, &project.KapiProject{Name: "uses"}, usesConcepts(), usesBlocks())
	res, err := host.SearchContext(t.Context(), p.src, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)

	byTerm := hitsByTerm(res)
	widget := byTerm["widget"]
	assert.True(t, widget.Discouraged)
	assert.Equal(t, 2, widget.Uses, "two uses of the retired word")
	assert.Equal(t, 2, widget.UseBlocks, "in two blocks")
	require.Len(t, widget.TopUses, 2)
	assert.Equal(t, "docs/guide.md", widget.TopUses[0].Document)
	assert.Equal(t, "guide.a", widget.TopUses[0].BlockID)
	assert.Contains(t, widget.TopUses[0].Snippet, "widget", "the passage is read from the block the edge names")

	gadget := byTerm["gadget"]
	assert.Equal(t, 1, gadget.Uses, "the preferred word is counted too")

	// A term in another language is counted where that language is written.
	dings := byTerm["dings"]
	assert.Equal(t, 1, dings.Uses)
	require.Len(t, dings.TopUses, 1)
	assert.Equal(t, "nb", dings.TopUses[0].Locale)
	assert.Contains(t, dings.TopUses[0].Snippet, "dings")

	assert.NotContains(t, notesJoined(res), "term usage", "a counted answer carries no usage caveat")
}

// The count is the graph's, not the block cache's. A block written after the
// last extraction is not counted until the graph is rebuilt, which is what "as
// of the last extraction" means on every face; and once it is rebuilt, the
// count moves.
func TestSearchContextCountsAsOfTheLastExtraction(t *testing.T) {
	p := newGraphProject(t, &project.KapiProject{Name: "uses"}, usesConcepts(), usesBlocks())
	p.putBlocks(t, []usesBlock{{hash: "h4", id: "guide.d", file: "docs/guide.md", source: "One more widget."}})

	before, err := host.SearchContext(t.Context(), p.src, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)
	assert.Equal(t, 2, hitsByTerm(before)["widget"].Uses, "the cache moved, the graph did not")

	p.materialize(t)
	after, err := host.SearchContext(t.Context(), p.src, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)
	assert.Equal(t, 3, hitsByTerm(after)["widget"].Uses, "the rebuilt graph counts the new block")
	assert.Equal(t, 3, hitsByTerm(after)["widget"].UseBlocks)
}

// Repeated uses of one term in one block fold onto one edge carrying the count,
// and the answer reads the count rather than the number of edges.
func TestSearchContextSumsRepeatedUsesWithinABlock(t *testing.T) {
	p := newGraphProject(t, &project.KapiProject{Name: "uses"}, usesConcepts(), []usesBlock{
		{hash: "h1", id: "a", file: "docs/a.md", source: "A widget, another widget, a third widget."},
	})
	res, err := host.SearchContext(t.Context(), p.src, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)
	widget := hitsByTerm(res)["widget"]
	assert.Equal(t, 3, widget.Uses)
	assert.Equal(t, 1, widget.UseBlocks)
	require.Len(t, widget.TopUses, 1, "one block is one place")
}

// The count stands without the block cache; only the passage needs it. A caller
// that bound the graph and not the blocks gets the number and no snippet.
func TestSearchContextCountsWithoutABlockCache(t *testing.T) {
	p := newGraphProject(t, &project.KapiProject{Name: "uses"}, usesConcepts(), usesBlocks())
	src := p.src
	src.Blocks = nil
	res, err := host.SearchContext(t.Context(), src, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)
	widget := hitsByTerm(res)["widget"]
	assert.Equal(t, 2, widget.Uses)
	require.Len(t, widget.TopUses, 2)
	assert.Empty(t, widget.TopUses[0].Snippet)
	assert.Equal(t, "docs/guide.md", widget.TopUses[0].Document, "where it is used is on the edge")
}

// A project with no graph bound must not look like a project where the word is
// unused. The two need different next steps, so the search says which.
func TestSearchContextWithoutGraphSaysSo(t *testing.T) {
	p := newGraphProject(t, &project.KapiProject{Name: "uses"}, usesConcepts(), usesBlocks())
	res, err := host.SearchContext(t.Context(), host.ContextSearchSources{
		Terms:  p.src.Terms,
		Blocks: p.src.Blocks,
	}, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)

	require.NotEmpty(t, res.Terms)
	for _, h := range res.Terms {
		assert.Zero(t, h.Uses, "no join is run over the block cache in the graph's place")
	}
	assert.Contains(t, notesJoined(res), "no context graph is bound, so term usage was not counted")
}

// Nothing extracted is a different state from nothing found, and the note
// says so rather than reporting zero uses of a word nobody has looked for.
func TestSearchContextUnextractedSaysSo(t *testing.T) {
	p := newGraphProject(t, &project.KapiProject{Name: "uses"}, usesConcepts(), usesBlocks())
	src := p.src
	src.Unextracted = true
	res, err := host.SearchContext(t.Context(), src, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)
	assert.Contains(t, notesJoined(res), "has not been extracted yet, so term usage was not counted")
}

// A graph that could not be opened is broken, not absent.
func TestSearchContextGraphErrorSaysSo(t *testing.T) {
	p := newGraphProject(t, &project.KapiProject{Name: "uses"}, usesConcepts(), usesBlocks())
	res, err := host.SearchContext(t.Context(), host.ContextSearchSources{
		Terms:    p.src.Terms,
		GraphErr: errors.New("disk on fire"),
	}, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)
	notes := notesJoined(res)
	assert.Contains(t, notes, "the context graph could not be opened, so term usage was not counted: disk on fire")
	assert.NotContains(t, notes, "no context graph is bound", "broken and absent are not both reported")
}

// A standalone vocabulary is counted against the project's edges, which name the
// project's own concepts, so the answer says what the count covers rather than
// letting a foreign store read as unused.
func TestSearchContextQualifiesAStandaloneTermsStore(t *testing.T) {
	p := newGraphProject(t, &project.KapiProject{Name: "uses"}, usesConcepts(), usesBlocks())
	src := p.src
	src.TermsStandalone = true
	res, err := host.SearchContext(t.Context(), src, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)
	assert.Equal(t, 2, hitsByTerm(res)["widget"].Uses, "a matching concept id is counted")
	assert.Contains(t, notesJoined(res), "a standalone terms store is counted only where its concept ids match")

	uses, err := host.FindContextUses(t.Context(), src, "widget", 0)
	require.NoError(t, err)
	assert.Equal(t, 2, uses.Total)
	require.Len(t, uses.Notes, 1)
	assert.Contains(t, uses.Notes[0], "a standalone terms store")
}

// The note is about term usage, so it has no business appearing when the query
// matched no term at all.
func TestSearchContextWithoutTermHitsSaysNothingAboutUses(t *testing.T) {
	res, err := host.SearchContext(t.Context(), host.ContextSearchSources{
		Terms: inMemoryTerms(t, usesConcepts()),
	}, host.ContextSearchRequest{Query: "nothing at all like a term"})
	require.NoError(t, err)

	assert.Empty(t, res.Terms)
	assert.NotContains(t, notesJoined(res), "term usage")
}

// The rendered line says what the number means, because a reader comparing it
// against a file just edited has to know which of the two moved.
func TestContextSearchResultRendersUsesAsOfTheLastExtraction(t *testing.T) {
	p := newGraphProject(t, &project.KapiProject{Name: "uses"}, usesConcepts(), usesBlocks())
	res, err := host.SearchContext(t.Context(), p.src, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)

	var b testWriter
	require.NoError(t, res.FormatText(&b))
	out := b.String()
	assert.Contains(t, out, "used 2 time(s) in 2 block(s), as of the last extraction")
	assert.Contains(t, out, "docs/guide.md guide.a")
}

func inMemoryTerms(t *testing.T, concepts []terms.Concept) terms.Terminology {
	t.Helper()
	tb := terms.NewInMemoryStore()
	for _, c := range concepts {
		require.NoError(t, tb.AddConcept(context.Background(), c))
	}
	return tb
}

func notesJoined(res *host.ContextSearchResult) string {
	var b strings.Builder
	for _, n := range res.Notes {
		b.WriteString(n + "\n")
	}
	return b.String()
}

type testWriter struct{ b []byte }

func (w *testWriter) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
func (w *testWriter) String() string              { return string(w.b) }
