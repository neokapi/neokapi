//go:build integration

package server

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEditorLookupTermsForBlock_TargetTerms drives the term-matches lookup
// against the Postgres terms store the server opens for a workspace. A matched
// concept with a fr term answers that term as target_terms; a matched concept
// with none answers nil.
func TestEditorLookupTermsForBlock_TargetTerms(t *testing.T) {
	ctx := t.Context()
	db := pgtest.NewTestDB(t)

	cs, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID:                    "p1",
		Name:                  "Proj",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
	}))
	require.NoError(t, cs.StoreItem(ctx, "p1", "main", &platstore.Item{Name: "about-us.html", Format: "html", ItemType: "file"}))
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Turn on encryption from the dashboard")
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", "about-us.html", []*model.Block{b}))

	tb, err := sqlterms.NewPostgresStoreFromDB(db, "acme")
	require.NoError(t, err)
	for _, c := range []terms.Concept{
		{ID: "c-enc", Domain: "security", Terms: []terms.Term{
			{Text: "encryption", Locale: "en", Status: model.TermApproved},
			{Text: "chiffrement", Locale: "fr", Status: model.TermApproved},
		}},
		{ID: "c-dash", Domain: "ui", Terms: []terms.Term{
			{Text: "dashboard", Locale: "en", Status: model.TermApproved},
		}},
	} {
		require.NoError(t, tb.AddConcept(ctx, c))
	}

	wsStores := newWorkspaceStores()
	wsStores.pgDB = db

	blocks, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: "p1", Stream: "main", ItemName: "about-us.html"})
	require.NoError(t, err)
	require.Len(t, blocks, 1)

	got, err := editorLookupTermsForBlock(ctx, cs, wsStores, "acme", "p1", "main", blocks[0].Block.ID, "fr")
	require.NoError(t, err)
	require.Len(t, got, 2)

	bySource := map[string]BlockTermMatchResponse{}
	for _, m := range got {
		bySource[m.SourceTerm] = m
	}
	assert.Equal(t, []string{"chiffrement"}, bySource["encryption"].TargetTerms)
	assert.Nil(t, bySource["dashboard"].TargetTerms)
}
