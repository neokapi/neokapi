package server

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEditorTermEnforce verifies the server-owned term-enforce action reports a
// violation when the preferred target term is absent, using the framework tool
// (no hand-reimplemented matching). It runs against an in-memory content store
// and terms, so it needs no PostgreSQL.
func TestEditorTermEnforce(t *testing.T) {
	ctx := t.Context()

	cs, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)

	proj := &platstore.Project{
		ID:                    "p1",
		Name:                  "Proj",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
	}
	require.NoError(t, cs.CreateProject(ctx, proj))
	require.NoError(t, cs.StoreItem(ctx, "p1", "main", &platstore.Item{Name: "hello.txt", Format: "txt", ItemType: "file"}))

	// Block whose fr target lacks the preferred term "logiciel".
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("the software is ready")
	b.SetTargetText("fr", "le programme est prêt")
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", "hello.txt", []*model.Block{b}))

	// In-memory terms seeded with software → logiciel (approved).
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:     "c1",
		Domain: "IT",
		Terms: []terms.Term{
			{Text: "software", Locale: "en", Status: model.TermApproved},
			{Text: "logiciel", Locale: "fr", Status: model.TermApproved},
		},
	}))

	wsStores := newWorkspaceStores()
	wsStores.tbFactory = func() terms.Store { return &testTermStore{tb} }

	results, err := editorTermEnforce(ctx, cs, wsStores, "acme", "p1", "main", "hello.txt", "fr")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.NotEmpty(t, results[0].BlockID)
	assert.Equal(t, "software", results[0].SourceTerm)
	assert.Equal(t, "c1", results[0].ConceptID)
	assert.Contains(t, results[0].Expected, "logiciel")
	assert.Equal(t, "en", results[0].SourceLocale)
	assert.Equal(t, "fr", results[0].TargetLocale)
}

// TestEditorTermEnforcePasses verifies no violation is reported when the target
// already uses the preferred term.
func TestEditorTermEnforcePasses(t *testing.T) {
	ctx := t.Context()

	cs, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)

	proj := &platstore.Project{
		ID:                    "p1",
		Name:                  "Proj",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
	}
	require.NoError(t, cs.CreateProject(ctx, proj))
	require.NoError(t, cs.StoreItem(ctx, "p1", "main", &platstore.Item{Name: "hello.txt", Format: "txt", ItemType: "file"}))

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("the software is ready")
	b.SetTargetText("fr", "le logiciel est prêt")
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", "hello.txt", []*model.Block{b}))

	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID: "c1", Domain: "IT",
		Terms: []terms.Term{
			{Text: "software", Locale: "en", Status: model.TermApproved},
			{Text: "logiciel", Locale: "fr", Status: model.TermApproved},
		},
	}))

	wsStores := newWorkspaceStores()
	wsStores.tbFactory = func() terms.Store { return &testTermStore{tb} }

	results, err := editorTermEnforce(ctx, cs, wsStores, "acme", "p1", "main", "hello.txt", "fr")
	require.NoError(t, err)
	assert.Empty(t, results)
}
