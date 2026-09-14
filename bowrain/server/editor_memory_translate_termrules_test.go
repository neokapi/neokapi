package server

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEditorMemoryTranslate_TermRules: the editor's content-memory translate
// holds matches to the workspace terms the way the convergence jobs do. A match
// that breaks a rule leaves its block untranslated, a match that keeps it fills,
// and a block whose source uses no ruled term fills. With no terms, every match
// fills.
func TestEditorMemoryTranslate_TermRules(t *testing.T) {
	pairs := map[string]string{
		"Open the dashboard":  "Ouvrir le panneau",
		"Close the dashboard": "Fermer le tableau de bord",
		"Hello":               "Bonjour",
	}

	run := func(t *testing.T, governed bool) map[string]string {
		t.Helper()
		ctx := t.Context()

		cs, err := bstore.NewSQLiteStore(":memory:")
		require.NoError(t, err)
		require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
			ID:                    "p1",
			Name:                  "Proj",
			DefaultSourceLanguage: "en",
			TargetLanguages:       []model.LocaleID{"fr"},
		}))
		require.NoError(t, cs.StoreItem(ctx, "p1", "main", &platstore.Item{Name: "hello.txt", Format: "txt", ItemType: "file"}))
		var blocks []*model.Block
		for i, source := range []string{"Open the dashboard", "Close the dashboard", "Hello"} {
			b := &model.Block{ID: "b" + string(rune('1'+i)), Translatable: true}
			b.SetSourceText(source)
			blocks = append(blocks, b)
		}
		require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", "hello.txt", blocks))

		tm := memory.NewInMemoryStore()
		for source, target := range pairs {
			require.NoError(t, tm.Add(ctx, memory.Entry{
				ID: "e-" + source,
				Variants: map[model.LocaleID][]model.Run{
					"en": {{Text: &model.TextRun{Text: source}}},
					"fr": {{Text: &model.TextRun{Text: target}}},
				},
				HintSrcLang: "en",
			}))
		}

		wsStores := newWorkspaceStores()
		wsStores.memoryFactory = func() memory.Store { return tm }
		tb := terms.NewInMemoryStore()
		if governed {
			require.NoError(t, tb.AddConcept(ctx, terms.Concept{
				ID: "c-dashboard",
				Terms: []terms.Term{
					{Text: "dashboard", Locale: "en", Status: model.TermPreferred},
					{Text: "tableau de bord", Locale: "fr", Status: model.TermPreferred},
				},
			}))
		}
		wsStores.termsFactory = func() terms.Store { return &testTermStore{tb} }

		_, err = editorMemoryTranslate(ctx, cs, wsStores, "acme", "p1", "main", "hello.txt", "fr")
		require.NoError(t, err)

		stored, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: "p1", Stream: "main", ItemName: "hello.txt"})
		require.NoError(t, err)
		targets := map[string]string{}
		for _, sb := range stored {
			targets[sb.Block.SourceText()] = sb.Block.TargetText("fr")
		}
		return targets
	}

	t.Run("governed", func(t *testing.T) {
		targets := run(t, true)
		assert.Empty(t, targets["Open the dashboard"], "the match that breaks the rule does not fill")
		assert.Equal(t, "Fermer le tableau de bord", targets["Close the dashboard"])
		assert.Equal(t, "Bonjour", targets["Hello"])
	})

	t.Run("no terms", func(t *testing.T) {
		targets := run(t, false)
		assert.Equal(t, "Ouvrir le panneau", targets["Open the dashboard"])
		assert.Equal(t, "Fermer le tableau de bord", targets["Close the dashboard"])
		assert.Equal(t, "Bonjour", targets["Hello"])
	})
}
