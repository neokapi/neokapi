package sync

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/terms"
)

// newRefStore opens a real content store with one project on the main stream.
// Real, because every component is derived from stored rows: a fake would be
// answering with the value under test.
func newRefStore(t *testing.T) (*sqlitestore.SQLiteStore, string) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	p := &platstore.Project{Name: "refs", DefaultSourceLanguage: model.LocaleEnglish}
	require.NoError(t, cs.CreateProject(t.Context(), p))
	return cs, p.ID
}

func TestCurrentRef_RequiresAContentStore(t *testing.T) {
	_, err := CurrentRef(t.Context(), RefSource{}, "p", "main")
	require.Error(t, err)
}

// TestCurrentRef_ComponentsMoveIndependently is the property the whole design
// rests on: each component tracks exactly the state it names.
func TestCurrentRef_ComponentsMoveIndependently(t *testing.T) {
	cs, projectID := newRefStore(t)
	ctx := t.Context()
	src := RefSource{Content: cs}

	base, err := CurrentRef(ctx, src, projectID, "main")
	require.NoError(t, err)
	assert.Equal(t, int64(0), base.Content)
	assert.Empty(t, base.Context)
	assert.Empty(t, base.Decisions)

	require.NoError(t, cs.StoreBlocks(ctx, projectID, "main",
		[]*model.Block{model.NewBlock("greeting", "Hello")}))
	afterContent, err := CurrentRef(ctx, src, projectID, "main")
	require.NoError(t, err)
	assert.Greater(t, afterContent.Content, base.Content)
	assert.Equal(t, base.Context, afterContent.Context)
	assert.Equal(t, base.Decisions, afterContent.Decisions)

	require.NoError(t, cs.CreateCollection(ctx, &platstore.Collection{
		ProjectID: projectID, Name: "docs", Kind: platstore.CollectionConnected,
		Stream: "main", Owner: venue.ContextOwnerRecipe, ContextHash: "h1",
	}))
	afterContext, err := CurrentRef(ctx, src, projectID, "main")
	require.NoError(t, err)
	assert.NotEmpty(t, afterContext.Context)
	assert.Equal(t, afterContent.Content, afterContext.Content)
	assert.Equal(t, afterContent.Decisions, afterContext.Decisions)

	_, err = cs.UpsertUnitDecisions(ctx, projectID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "fr",
		Status: "reviewed", ReviewState: "approved", DecidedBy: "ana", Updated: "2026-08-01T00:00:00Z",
	}})
	require.NoError(t, err)
	afterDecision, err := CurrentRef(ctx, src, projectID, "main")
	require.NoError(t, err)
	assert.NotEmpty(t, afterDecision.Decisions)
	assert.Equal(t, afterContext.Context, afterDecision.Context)
}

// TestCurrentRef_IsPerStream: a ref describes one stream, so a branch's
// components are not the default stream's.
func TestCurrentRef_IsPerStream(t *testing.T) {
	cs, projectID := newRefStore(t)
	ctx := t.Context()
	src := RefSource{Content: cs}

	require.NoError(t, cs.CreateCollection(ctx, &platstore.Collection{
		ProjectID: projectID, Name: "docs", Kind: platstore.CollectionConnected,
		Stream: "main", Owner: venue.ContextOwnerRecipe, ContextHash: "h1",
	}))

	main, err := CurrentRef(ctx, src, projectID, "main")
	require.NoError(t, err)
	branch, err := CurrentRef(ctx, src, projectID, "release")
	require.NoError(t, err)

	assert.NotEmpty(t, main.Context)
	assert.Empty(t, branch.Context, "a branch that declares nothing claims nothing")
}

// TestCurrentRef_TermsComponentTracksTheWorkspaceVocabulary wires the one
// component that comes from outside the content store.
func TestCurrentRef_TermsComponentTracksTheWorkspaceVocabulary(t *testing.T) {
	cs, projectID := newRefStore(t)
	ctx := t.Context()

	tb := terms.NewInMemoryStore()
	src := RefSource{Content: cs, Terms: tb}

	empty, err := CurrentRef(ctx, src, projectID, "main")
	require.NoError(t, err)
	assert.Empty(t, empty.Terms, "an empty vocabulary makes no claim")

	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID: "c1", Domain: "product", Definition: "The loop.",
		Terms: []terms.Term{{Text: "convergence", Locale: model.LocaleEnglish, Status: model.TermPreferred}},
	}))
	seeded, err := CurrentRef(ctx, src, projectID, "main")
	require.NoError(t, err)
	require.NotEmpty(t, seeded.Terms)

	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID: "c1", Domain: "product", Definition: "The kapi loop.",
		Terms: []terms.Term{{Text: "convergence", Locale: model.LocaleEnglish, Status: model.TermPreferred}},
	}))
	edited, err := CurrentRef(ctx, src, projectID, "main")
	require.NoError(t, err)
	assert.NotEqual(t, seeded.Terms, edited.Terms)
	assert.Equal(t, seeded.Context, edited.Context, "terminology is not the declared context")

	// An unreachable vocabulary costs the answer to one question, not the ref.
	withoutTerms, err := CurrentRef(ctx, RefSource{Content: cs}, projectID, "main")
	require.NoError(t, err)
	assert.Empty(t, withoutTerms.Terms)
	assert.Equal(t, edited.Content, withoutTerms.Content)
}
