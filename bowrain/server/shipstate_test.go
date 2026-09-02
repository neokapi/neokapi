package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	voicepg "github.com/neokapi/neokapi/bowrain/voice"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// phRun returns the shared inline placeholder used by the ship-state fixtures:
// dropping it from a target trips the default code-difference check with
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
// store: approved counts ride GetBlockStats, the check pass runs only for
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
	require.NoError(t, applyShipStates(ctx, cs, nil, pid, "main", nil, stats))

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

	// On-brand rates (no voice store → checks-only basis everywhere derived).
	assertCompliant(t, fr, 2, 1.0, platstore.ComplianceBasisChecks)
	assertCompliant(t, de, 2, 1.0, platstore.ComplianceBasisChecks)
	assertCompliant(t, es, 1, 1.0, platstore.ComplianceBasisChecks)
	itStats := localeByCode(t, stats.LocaleStats, "it")
	assertCompliant(t, itStats, 1, 0.5, platstore.ComplianceBasisChecks)
	assertCompliant(t, itB, 0, 0.0, platstore.ComplianceBasisChecks)
	esB := localeByCode(t, colB.Locales, "es")
	assert.Nil(t, esB.ComplianceRate, "nothing translated in col-b/es → no rate")
	assert.Empty(t, esB.ComplianceBasis)
}

// assertCompliant checks the derived compliant triple on one locale scope.
func assertCompliant(t *testing.T, ls platstore.LocaleTranslationStats, blocks int, rate float64, basis platstore.ComplianceBasis) {
	t.Helper()
	assert.Equal(t, blocks, ls.CompliantBlocks, "%s: compliant blocks", ls.Locale)
	require.NotNil(t, ls.ComplianceRate, "%s: rate must be derived", ls.Locale)
	assert.InDelta(t, rate, *ls.ComplianceRate, 0.0001, "%s: compliance rate", ls.Locale)
	assert.Equal(t, basis, ls.ComplianceBasis, "%s: basis", ls.Locale)
}

// TestApplyShipStates_ComplianceVoiceScores covers the voice-informed half of the
// compliance rate: a persisted voice score below the profile's min bar demotes a
// checks-passing block, the latest score per block+locale wins, and the basis
// flips to voice+checks only in the scopes a score actually informed.
func TestApplyShipStates_ComplianceVoiceScores(t *testing.T) {
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	pid := seedShipStateProject(t, cs)

	voiceDB := pgtest.NewTestDB(t)
	bs, err := voicepg.NewPostgresVoiceStore(voiceDB)
	require.NoError(t, err)

	ctx := t.Context()
	profile := &coreprofile.VoiceProfile{ID: "p-ship", Scope: "ws-1", Name: "Ship Voice", MinScore: 80}
	require.NoError(t, bs.CreateProfile(ctx, profile))

	// The content store assigns block IDs on store; scores must reference the
	// STORED ids (exactly as the worker does — it scores blocks read back from
	// the store).
	storedID := func(item string) string {
		blocks, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: pid, Stream: "main", ItemName: item})
		require.NoError(t, err)
		require.Len(t, blocks, 1)
		return blocks[0].Block.ID
	}
	b1 := storedID("a.json")
	b2 := storedID("b.json")

	now := time.Now().UTC()
	score := func(block, loc string, val int, at time.Time) {
		require.NoError(t, bs.StoreScore(ctx, &coreprofile.StoredScore{
			ProjectID: pid, Stream: "main", BlockID: block, ProfileID: profile.ID,
			Locale: model.LocaleID(loc), Score: val, CheckedAt: at,
		}))
	}
	// de: b1 scores below the bar (checks pass, voice fails); b2 unscored.
	score(b1, "de", 60, now)
	// fr: an old failing score superseded by a fresh passing one — latest wins.
	score(b1, "fr", 40, now.Add(-time.Hour))
	score(b1, "fr", 95, now)
	score(b2, "fr", 100, now)

	proj, err := cs.GetProject(ctx, pid)
	require.NoError(t, err)
	stats, err := editorGetDashboardStats(ctx, cs, proj, "main")
	require.NoError(t, err)
	require.NoError(t, applyShipStates(ctx, cs, bs, pid, "main", nil, stats))

	fr := localeByCode(t, stats.LocaleStats, "fr")
	assertCompliant(t, fr, 2, 1.0, platstore.ComplianceBasisVoice)
	assert.Equal(t, platstore.ShipStateGoverned, fr.ShipState, "voice scores do not disturb ship states")

	de := localeByCode(t, stats.LocaleStats, "de")
	assertCompliant(t, de, 1, 0.5, platstore.ComplianceBasisVoice)
	assert.Equal(t, platstore.ShipStateAIShippable, de.ShipState, "a sub-bar voice score does not demote the ship state")
	assert.Equal(t, 0, de.FailingChecks)

	// Unscored locales keep the checks-only basis.
	es := localeByCode(t, stats.LocaleStats, "es")
	assertCompliant(t, es, 1, 1.0, platstore.ComplianceBasisChecks)

	// Per-collection basis: the de score lives on b1 (col-a); col-b/de has no
	// scored block and stays checks-only.
	colA := collByID(t, stats.CollectionStats, "col-a")
	assertCompliant(t, localeByCode(t, colA.Locales, "de"), 0, 0.0, platstore.ComplianceBasisVoice)
	colB := collByID(t, stats.CollectionStats, "col-b")
	assertCompliant(t, localeByCode(t, colB.Locales, "de"), 1, 1.0, platstore.ComplianceBasisChecks)
}

// TestApplyShipStates_StaleBasisWithholdsLocale is #1957's regression, over the
// two surfaces that read the derivation: the dashboard's per-locale (and
// per-collection) ship state, and the public ship manifest the picker consumes.
//
// A decision blesses a pairing. Rewriting the source under an approved
// translation leaves a target that renders a sentence the project no longer
// has, and no machine has looked at the new source either — so the locale is
// not shippable on anyone's review. Restoring the source converges back on the
// decision already recorded.
func TestApplyShipStates_StaleBasisWithholdsLocale(t *testing.T) {
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	ctx := t.Context()

	proj := &platstore.Project{
		Name:                  "stale-proj",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"nb"},
		WorkspaceID:           "ws-1",
	}
	require.NoError(t, cs.CreateProject(ctx, proj))
	require.NoError(t, cs.StoreItem(ctx, proj.ID, "main", &platstore.Item{
		Name: "en.json", Format: "json", ItemType: "file", CollectionID: "col-a",
	}))

	storeSource := func(text string, withTarget bool) {
		b := &model.Block{ID: "greeting", Translatable: true}
		b.SetSourceText(text)
		if withTarget {
			b.SetTargetText("nb", "Hei")
			b.Target("nb").Status = model.TargetStatusTranslated
		}
		require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", "en.json", []*model.Block{b}))
	}
	// A push carries source only; the target is already stored.
	storeSource("Hello", true)
	_, err = cs.UpsertUnitDecisions(ctx, proj.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:      string(model.TargetStatusReviewed),
		TargetHash:  state.TargetHash("Hei"),
		ContentHash: state.SourceHash("Hello"),
		ReviewState: "approved",
		DecidedBy:   "reviewer@example.com",
		Updated:     "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)

	derive := func() platstore.LocaleTranslationStats {
		t.Helper()
		p, err := cs.GetProject(ctx, proj.ID)
		require.NoError(t, err)
		stats, err := editorGetDashboardStats(ctx, cs, p, "main")
		require.NoError(t, err)
		require.NoError(t, applyShipStates(ctx, cs, nil, proj.ID, "main", nil, stats))
		// The manifest the public feed serves is the same derivation projected.
		feed := shipManifestFromStats(stats)
		nb := localeByCode(t, stats.LocaleStats, "nb")
		assert.Equal(t, nb.ShipState == platstore.ShipStateGoverned || nb.ShipState == platstore.ShipStateAIShippable,
			feed["nb"].Shippable, "the feed reads the same ship state the dashboard does")
		assert.Equal(t, nb.ShipState == platstore.ShipStateGoverned, feed["nb"].Verified)
		// The collection rollup carries the same verdict for its one item.
		coll := localeByCode(t, collByID(t, stats.CollectionStats, "col-a").Locales, "nb")
		assert.Equal(t, nb.ShipState, coll.ShipState, "the collection rollup agrees with the project")
		assert.Equal(t, nb.StaleBlocks, coll.StaleBlocks)
		return nb
	}

	approved := derive()
	require.Equal(t, platstore.ShipStateGoverned, approved.ShipState, "approved and current: governed")
	assert.Zero(t, approved.StaleBlocks)

	// The source moves under the approval.
	storeSource("Hello there", false)
	stale := derive()
	assert.Equal(t, platstore.ShipStatePending, stale.ShipState,
		"a translation of rewritten source is not shippable on anyone's review")
	assert.Equal(t, 1, stale.StaleBlocks)
	assert.Equal(t, 1, stale.TranslatedBlocks, "the target is still there — this is not a coverage shortfall")
	assert.Zero(t, stale.FailingChecks, "and it is not a failing check either")

	// The source comes back.
	storeSource("Hello", false)
	restored := derive()
	assert.Equal(t, platstore.ShipStateGoverned, restored.ShipState,
		"the recorded decision applies again — the locale recovers without a second review")
	assert.Zero(t, restored.StaleBlocks)
}

// TestApplyShipStates_MissingBasisShipsAndIsCounted: a decision recorded before
// the basis was tracked says nothing about the source it blessed. Reading that
// silence as drift would withhold every locale of every project holding older
// decisions, so the unit keeps its rung and ships as it did — and the
// assumption is counted rather than left silent.
func TestApplyShipStates_MissingBasisShipsAndIsCounted(t *testing.T) {
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	ctx := t.Context()

	proj := &platstore.Project{
		Name:                  "unknown-basis-proj",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"nb"},
		WorkspaceID:           "ws-1",
	}
	require.NoError(t, cs.CreateProject(ctx, proj))
	require.NoError(t, cs.StoreItem(ctx, proj.ID, "main", &platstore.Item{
		Name: "en.json", Format: "json", ItemType: "file", CollectionID: "col-a",
	}))
	b := &model.Block{ID: "greeting", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("nb", "Hei")
	b.StampTargetProvenance("nb", model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})
	require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", "en.json", []*model.Block{b}))

	// The record predates the basis: no ContentHash at all.
	_, err = cs.UpsertUnitDecisions(ctx, proj.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:      string(model.TargetStatusReviewed),
		TargetHash:  state.TargetHash("Hei"),
		ReviewState: "approved",
		Updated:     "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)

	p, err := cs.GetProject(ctx, proj.ID)
	require.NoError(t, err)
	stats, err := editorGetDashboardStats(ctx, cs, p, "main")
	require.NoError(t, err)
	require.NoError(t, applyShipStates(ctx, cs, nil, proj.ID, "main", nil, stats))

	nb := localeByCode(t, stats.LocaleStats, "nb")
	assert.Equal(t, platstore.ShipStateGoverned, nb.ShipState, "an unknown basis ships as before")
	assert.Zero(t, nb.StaleBlocks, "unknown is not stale")
	assert.Equal(t, 1, nb.BasisUnknownBlocks, "and it is counted, not silent")
	assert.True(t, shipManifestFromStats(stats)["nb"].Shippable)
}

// TestPublicShipManifestWithholdsStaleLocale drives the R8 feed endpoint end to
// end: the public manifest a picker reads must drop a locale whose source moved
// under its approval, exactly as the dashboard does.
func TestPublicShipManifestWithholdsStaleLocale(t *testing.T) {
	srv, _ := newTestServer(t)
	ctx := t.Context()

	proj := &platstore.Project{
		Name:                  "feed-proj",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"nb"},
		WorkspaceID:           "test-ws",
		Properties:            map[string]string{ShipFeedProperty: "true"},
	}
	require.NoError(t, srv.ContentStore.CreateProject(ctx, proj))
	require.NoError(t, srv.ContentStore.StoreItem(ctx, proj.ID, "main", &platstore.Item{
		Name: "en.json", Format: "json", ItemType: "file",
	}))
	storeSource := func(text string, withTarget bool) {
		b := &model.Block{ID: "greeting", Translatable: true}
		b.SetSourceText(text)
		if withTarget {
			b.SetTargetText("nb", "Hei")
			b.Target("nb").Status = model.TargetStatusTranslated
		}
		require.NoError(t, srv.ContentStore.StoreBlocksForItem(ctx, proj.ID, "main", "en.json", []*model.Block{b}))
		srv.invalidateDashboardCache(proj.WorkspaceID, proj.ID)
	}
	storeSource("Hello", true)

	ds, ok := srv.ContentStore.(platstore.DecisionStore)
	require.True(t, ok)
	_, err := ds.UpsertUnitDecisions(ctx, proj.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:      string(model.TargetStatusReviewed),
		TargetHash:  state.TargetHash("Hei"),
		ContentHash: state.SourceHash("Hello"),
		ReviewState: "approved",
		Updated:     "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	srv.invalidateDashboardCache(proj.WorkspaceID, proj.ID)

	feed := func() map[string]shipManifestEntry {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+proj.ID+"/ship.json", nil)
		rec := httptest.NewRecorder()
		srv.GetEcho().ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		var out map[string]shipManifestEntry
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		return out
	}

	assert.Equal(t, shipManifestEntry{Shippable: true, Verified: true}, feed()["nb"],
		"approved against the current source: the picker may offer it, badged verified")

	storeSource("Hello there", false)
	assert.Equal(t, shipManifestEntry{}, feed()["nb"],
		"the picker must not offer a locale rendering source the project rewrote")

	storeSource("Hello", false)
	assert.Equal(t, shipManifestEntry{Shippable: true, Verified: true}, feed()["nb"],
		"and it recovers when the source comes back, with no second review")
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
	assert.True(t, strings.Contains(body, `"compliance_basis":"checks"`), "wire carries compliance_basis: %s", body)

	stats := getDashboard(t, srv, token, pid, "")
	fr := localeByCode(t, stats.LocaleStats, "fr")
	assert.Equal(t, platstore.ShipStatePending, fr.ShipState, "1 of 3 blocks translated → pending")
	assert.Equal(t, 1, fr.ApprovedBlocks)
	assertCompliant(t, fr, 1, 1.0, platstore.ComplianceBasisChecks)
}
