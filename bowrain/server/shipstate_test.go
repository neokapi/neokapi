package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// phRun returns the shared inline placeholder used by the ship-state fixtures:
// dropping it from a target trips the default code-difference QA check with
// error severity (the same trigger convergence's countFailingBlocks tests use).
func phRun() model.Run {
	return model.Run{Ph: &model.PlaceholderRun{ID: "1", Disp: "{name}"}}
}

func textRun(s string) model.Run {
	return model.Run{Text: &model.TextRun{Text: s}}
}

// seedShipStateProject creates a project (en → fr, de, es, it) with two items
// in two collections:
//
//	a.json (col-a): b1, plain text.   fr reviewed; de/es/it translated.
//	b.json (col-b): b2, text + {name} placeholder. fr reviewed (placeholder
//	                kept); de translated (placeholder kept); it drops the
//	                placeholder (failing check); es untranslated.
//
// Expected per-locale states — global: fr governed, de ai_shippable,
// es pending (partial coverage), it pending (failing check). Collection col-a:
// everything fully covered and clean (fr governed, de/es/it ai_shippable);
// col-b: fr governed, de ai_shippable, es pending (no target), it pending
// (failing check attributed here).
func seedShipStateProject(t *testing.T, cs *bstore.PostgresStore) string {
	t.Helper()
	ctx := t.Context()
	proj := &platstore.Project{
		Name:                  "ship-proj",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr", "de", "es", "it"},
		WorkspaceID:           "ws-1",
		Properties:            map[string]string{},
	}
	require.NoError(t, cs.CreateProject(ctx, proj))

	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Hello world")
	b1.SetTargetText("fr", "Bonjour le monde")
	b1.StampTargetProvenance("fr", model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})
	b1.SetTargetText("de", "Hallo Welt")
	b1.SetTargetText("es", "Hola mundo")
	b1.SetTargetText("it", "Ciao mondo")

	b2 := &model.Block{
		ID:           "b2",
		Translatable: true,
		Source:       []model.Run{textRun("Hello "), phRun()},
	}
	b2.SetTargetRuns("fr", []model.Run{textRun("Bonjour "), phRun()})
	b2.StampTargetProvenance("fr", model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})
	b2.SetTargetRuns("de", []model.Run{textRun("Hallo "), phRun()})
	b2.SetTargetText("it", "Ciao") // drops the placeholder → error-severity finding

	require.NoError(t, cs.StoreItem(ctx, proj.ID, "main", &platstore.Item{
		Name: "a.json", Format: "json", ItemType: "file", CollectionID: "col-a",
	}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", "a.json", []*model.Block{b1}))
	require.NoError(t, cs.StoreItem(ctx, proj.ID, "main", &platstore.Item{
		Name: "b.json", Format: "json", ItemType: "file", CollectionID: "col-b",
	}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", "b.json", []*model.Block{b2}))
	return proj.ID
}

func localeByCode(t *testing.T, stats []platstore.LocaleTranslationStats, code string) platstore.LocaleTranslationStats {
	t.Helper()
	for _, ls := range stats {
		if ls.Locale == code {
			return ls
		}
	}
	t.Fatalf("locale %s not found", code)
	return platstore.LocaleTranslationStats{}
}

func collByID(t *testing.T, stats []platstore.CollectionTranslationStats, id string) platstore.CollectionTranslationStats {
	t.Helper()
	for _, c := range stats {
		if c.CollectionID == id {
			return c
		}
	}
	t.Fatalf("collection %s not found", id)
	return platstore.CollectionTranslationStats{}
}

// TestApplyShipStates covers the server-side derivation over a real content
// store: approved counts ride GetBlockStats, the QA pass runs only for
// coverage-complete locales, failing blocks are attributed to their
// collection, and every scope gets the derived ship state.
func TestApplyShipStates(t *testing.T) {
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	pid := seedShipStateProject(t, cs)

	ctx := t.Context()
	proj, err := cs.GetProject(ctx, pid)
	require.NoError(t, err)
	stats, err := editorGetDashboardStats(ctx, cs, proj, "main")
	require.NoError(t, err)
	require.NoError(t, applyShipStates(ctx, cs, pid, "main", stats))

	// Global per-locale states.
	fr := localeByCode(t, stats.LocaleStats, "fr")
	assert.Equal(t, platstore.ShipStateGoverned, fr.ShipState, "fr: full coverage, all approved, checks pass")
	assert.Equal(t, 2, fr.ApprovedBlocks)
	assert.Equal(t, 0, fr.FailingChecks)

	de := localeByCode(t, stats.LocaleStats, "de")
	assert.Equal(t, platstore.ShipStateAIShippable, de.ShipState, "de: full coverage, machine-reviewed only")
	assert.Equal(t, 0, de.ApprovedBlocks)
	assert.Equal(t, 0, de.FailingChecks)

	es := localeByCode(t, stats.LocaleStats, "es")
	assert.Equal(t, platstore.ShipStatePending, es.ShipState, "es: partial coverage")
	assert.Equal(t, 1, es.TranslatedBlocks)
	assert.Equal(t, 0, es.FailingChecks, "checks are not attributed below the coverage gate")

	it := localeByCode(t, stats.LocaleStats, "it")
	assert.Equal(t, platstore.ShipStatePending, it.ShipState, "it: failing check demotes full coverage")
	assert.Equal(t, 2, it.TranslatedBlocks)
	assert.Equal(t, 1, it.FailingChecks)

	// Collection rollups.
	colA := collByID(t, stats.CollectionStats, "col-a")
	assert.Equal(t, platstore.ShipStateGoverned, localeByCode(t, colA.Locales, "fr").ShipState)
	assert.Equal(t, platstore.ShipStateAIShippable, localeByCode(t, colA.Locales, "de").ShipState)
	assert.Equal(t, platstore.ShipStateAIShippable, localeByCode(t, colA.Locales, "es").ShipState,
		"es is fully covered within col-a even though the project is not")
	assert.Equal(t, platstore.ShipStateAIShippable, localeByCode(t, colA.Locales, "it").ShipState,
		"it's failing block lives in col-b, not col-a")
	assert.Equal(t, 0, localeByCode(t, colA.Locales, "it").FailingChecks)

	colB := collByID(t, stats.CollectionStats, "col-b")
	assert.Equal(t, platstore.ShipStateGoverned, localeByCode(t, colB.Locales, "fr").ShipState)
	assert.Equal(t, platstore.ShipStatePending, localeByCode(t, colB.Locales, "es").ShipState)
	itB := localeByCode(t, colB.Locales, "it")
	assert.Equal(t, platstore.ShipStatePending, itB.ShipState)
	assert.Equal(t, 1, itB.FailingChecks, "the failing block is attributed to its collection")
}

// TestTranslationDashboardShipStateWire asserts the dashboard endpoint carries
// the additive ship-state fields on the wire without disturbing the existing
// shape.
func TestTranslationDashboardShipStateWire(t *testing.T) {
	srv, token := newTestServer(t)
	pid := seedDashboardProject(t, srv, token)

	// Approve the one translated block's fr target so the item-level rollup is
	// derivable; the project stays pending on coverage (two items untranslated).
	ctx := t.Context()
	blocks, err := srv.ContentStore.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: pid, Stream: "main", ItemName: "c.json",
	})
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	blocks[0].StampTargetProvenance("fr", model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})
	require.NoError(t, srv.ContentStore.StoreBlocks(ctx, pid, "main", []*model.Block{blocks[0].Block}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/"+pid+"/dashboard/main", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	body := rec.Body.String()
	assert.True(t, strings.Contains(body, `"ship_state":"pending"`), "wire carries ship_state: %s", body)
	assert.True(t, strings.Contains(body, `"approved_blocks":1`), "wire carries approved_blocks: %s", body)
	assert.True(t, strings.Contains(body, `"failing_checks":0`), "wire carries failing_checks: %s", body)

	stats := getDashboard(t, srv, token, pid, "")
	fr := localeByCode(t, stats.LocaleStats, "fr")
	assert.Equal(t, platstore.ShipStatePending, fr.ShipState, "1 of 3 blocks translated → pending")
	assert.Equal(t, 1, fr.ApprovedBlocks)
}
