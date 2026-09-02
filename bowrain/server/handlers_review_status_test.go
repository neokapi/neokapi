package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover epic 006 task 3 (server side): HandleReviewBlock stores the
// review flag as the PER-LOCALE model.Target.Status on the block's target for
// the requested locale — the framework target ladder that convergence /
// coverage and ship gates consume — instead of the legacy block-global
// Properties["translation-status"]. They run against the real PostgreSQL
// ContentStore (testcontainers), so every assertion covers the full
// handler → StoreBlocks → translations-overlay round-trip.

// newReviewTestServer builds a minimal Server over a real Postgres content
// store — enough for the review + blocks handlers (no auth middleware; tests
// grant permissions directly on the echo context).
func newReviewTestServer(t *testing.T) (*Server, *bstore.PostgresStore) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return &Server{ContentStore: cs, wsStores: newWorkspaceStores()}, cs
}

// seedReviewProject creates a project (en → fr, de) with one item and the
// given blocks, and returns the project ID plus the stored blocks' internal
// IDs keyed by source text (StoreBlocksForItem remaps reader IDs).
func seedReviewProject(t *testing.T, cs *bstore.PostgresStore, blocks []*model.Block) (string, map[string]string) {
	t.Helper()
	ctx := t.Context()
	proj := &platstore.Project{
		Name:                  "review-proj",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr", "de"},
		WorkspaceID:           "ws-1",
		Properties:            map[string]string{},
	}
	require.NoError(t, cs.CreateProject(ctx, proj))
	require.NoError(t, cs.StoreItem(ctx, proj.ID, "main", &platstore.Item{
		Name: "greetings.txt", Format: "txt", ItemType: "file",
	}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", "greetings.txt", blocks))

	stored, err := cs.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: proj.ID, Stream: "main", ItemName: "greetings.txt",
	})
	require.NoError(t, err)
	ids := make(map[string]string, len(stored))
	for _, sb := range stored {
		ids[sb.Block.SourceText()] = sb.Block.ID
	}
	return proj.ID, ids
}

// callReviewBlockBodyAs invokes HandleReviewBlock the way the router would,
// with a raw JSON body and the given resolved permission set. It returns the
// recorder and the handler's error: a denial commits a 403 to the recorder AND
// returns errAccessDenied.
func callReviewBlockBodyAs(t *testing.T, srv *Server, pid, bid, body string, perms platauth.Permission) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	r := httptest.NewRequest(http.MethodPut,
		"/api/v1/acme/"+pid+"/blocks/main/"+bid+"/review", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id", "ref", "bid")
	c.SetParamValues("acme", pid, "main", bid)
	c.Set("project_permissions", perms)
	return rec, srv.HandleReviewBlock(c)
}

// callReviewBlockAs invokes HandleReviewBlock with the standard body shape.
func callReviewBlockAs(t *testing.T, srv *Server, pid, bid, locale string, reviewed bool, perms platauth.Permission) (*httptest.ResponseRecorder, error) {
	t.Helper()
	body := fmt.Sprintf(`{"target_locale":%q,"reviewed":%t,"item_name":"greetings.txt"}`, locale, reviewed)
	return callReviewBlockBodyAs(t, srv, pid, bid, body, perms)
}

// callReviewBlock invokes HandleReviewBlock with full permissions.
func callReviewBlock(t *testing.T, srv *Server, pid, bid, locale string, reviewed bool) *httptest.ResponseRecorder {
	t.Helper()
	rec, err := callReviewBlockAs(t, srv, pid, bid, locale, reviewed, platauth.PermAll)
	require.NoError(t, err)
	return rec
}

func getStoredBlock(t *testing.T, cs *bstore.PostgresStore, pid, bid string) *model.Block {
	t.Helper()
	sb, err := cs.GetBlock(t.Context(), pid, "main", bid)
	require.NoError(t, err)
	return sb.Block
}

// TestHandleReviewBlockPerLocale: reviewing fr sets fr's Target.Status to
// reviewed WITHOUT touching de, and never writes the legacy block-global
// property.
func TestHandleReviewBlockPerLocale(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.SetTargetText("de", "Hallo")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec := callReviewBlock(t, srv, pid, bid, "fr", true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	got := getStoredBlock(t, cs, pid, bid)
	require.NotNil(t, got.Target("fr"))
	assert.Equal(t, model.TargetStatusReviewed, got.Target("fr").Status, "fr must be reviewed")
	require.NotNil(t, got.Target("de"))
	assert.Equal(t, model.TargetStatusNew, got.Target("de").Status, "de must be untouched")
	assert.Equal(t, "Bonjour", got.TargetText("fr"), "review must not touch the translation text")
	_, hasLegacy := got.Properties[legacyTranslationStatusProperty]
	assert.False(t, hasLegacy, "the legacy block-global property must never be written anymore")
}

// TestHandleReviewBlockUnreview: un-reviewing fr demotes its status back to
// translated, still without touching de.
func TestHandleReviewBlockUnreview(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.SetTargetText("de", "Hallo")
	b.StampTargetProvenance("de", model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec := callReviewBlock(t, srv, pid, bid, "fr", true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = callReviewBlock(t, srv, pid, bid, "fr", false)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	got := getStoredBlock(t, cs, pid, bid)
	assert.Equal(t, model.TargetStatusTranslated, got.Target("fr").Status, "fr must be back at translated")
	assert.Equal(t, model.TargetStatusReviewed, got.Target("de").Status, "de's own reviewed status must survive fr's un-review")
}

// TestHandleReviewBlockNoTarget documents the no-target decision (epic 006
// task 3): approving a locale that has no non-empty translation is a 422 —
// "reviewed" is a rung on the target ladder and convergence.TargetState only
// counts a status when a non-empty target exists (an untranslated block falls
// back to source in the editor, which is not a reviewable translation).
// Un-reviewing a locale with no target is an idempotent no-op.
func TestHandleReviewBlockNoTarget(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("de", "Hallo") // fr has NO target
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec := callReviewBlock(t, srv, pid, bid, "fr", true)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "no fr translation to review")

	got := getStoredBlock(t, cs, pid, bid)
	assert.Nil(t, got.Target("fr"), "a rejected approval must not create a target")

	// Un-review with no target: idempotent no-op success.
	rec = callReviewBlock(t, srv, pid, bid, "fr", false)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got = getStoredBlock(t, cs, pid, bid)
	assert.Nil(t, got.Target("fr"))

	// An empty translation is not reviewable either (mirrors the host review
	// service's "has no translation to approve" rule).
	require.NoError(t, cs.StoreBlocks(t.Context(), pid, "main", func() []*model.Block {
		blk := getStoredBlock(t, cs, pid, bid)
		blk.SetTargetText("fr", "   ")
		return []*model.Block{blk}
	}()))
	rec = callReviewBlock(t, srv, pid, bid, "fr", true)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
}

// TestHandleReviewBlockLegacyPropertyLifecycle: a block reviewed under the OLD
// scheme (block-global property, no per-locale status) still reads as reviewed
// through the documented fallback — the blocks payload carries both the empty
// per-locale status and the legacy property, and readers resolve
// targets[locale].status first, then the property. Un-reviewing a locale with
// no target clears the stuck legacy flag.
func TestHandleReviewBlockLegacyPropertyLifecycle(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	// Legacy block: property set, target present but with no per-locale status.
	b := &model.Block{ID: "b1", Translatable: true, Properties: map[string]string{
		legacyTranslationStatusProperty: "reviewed",
	}}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	// Legacy block with NO target at all (was reviewable under the old scheme).
	b2 := &model.Block{ID: "b2", Translatable: true, Properties: map[string]string{
		legacyTranslationStatusProperty: "reviewed",
	}}
	b2.SetSourceText("Goodbye")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b, b2})

	// The blocks payload (what the editor consumes) must surface BOTH the
	// per-locale status ("" here) and the legacy property, so the frontend
	// fallback (status first, property second) resolves this block as reviewed.
	blocks, err := editorGetBlocks(t.Context(), cs, pid, "main", "greetings.txt", []string{"fr", "de"}, platstore.DefaultBlockLimit, 0)
	require.NoError(t, err)
	byID := map[string]BlockInfoResponse{}
	for _, bi := range blocks {
		byID[bi.ID] = bi
	}
	legacy := byID[ids["Hello"]]
	assert.Equal(t, "Bonjour", legacy.Targets["fr"].Text)
	assert.Empty(t, legacy.Targets["fr"].Status, "legacy block carries no per-locale status")
	assert.Equal(t, "reviewed", legacy.Properties[legacyTranslationStatusProperty],
		"legacy property must survive as the read fallback")

	// Reviewing fr under the new scheme writes the per-locale status; the
	// legacy property stays as-is (other locales may still read it) but the
	// per-locale status now wins for fr.
	rec := callReviewBlock(t, srv, pid, ids["Hello"], "fr", true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got := getStoredBlock(t, cs, pid, ids["Hello"])
	assert.Equal(t, model.TargetStatusReviewed, got.Target("fr").Status)

	// Un-reviewing a NO-target locale on a legacy block clears the block-global
	// flag — the only faithful reading of "un-review" under the old scheme —
	// so the block does not stay stuck at reviewed forever.
	rec = callReviewBlock(t, srv, pid, ids["Goodbye"], "fr", false)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got = getStoredBlock(t, cs, pid, ids["Goodbye"])
	_, hasLegacy := got.Properties[legacyTranslationStatusProperty]
	assert.False(t, hasLegacy, "legacy flag must be cleared when un-reviewing a no-target locale")
}

// TestHandleReviewBlockSignedOff: signed-off is the rung ABOVE reviewed on the
// target ladder. A plain PermTranslate caller reaches neither end of it: an
// approve needs PermReview, and an un-review of a signed-off target needs the
// same. A caller holding PermReview re-approves it as an idempotent no-op that
// keeps signed-off, and may demote it.
func TestHandleReviewBlockSignedOff(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.StampTargetProvenance("fr", model.TargetStatusSignedOff, model.Origin{Kind: model.OriginHuman})
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	translator := platauth.PermViewContent | platauth.PermTranslate

	// A translator cannot approve at all, signed-off or otherwise.
	rec, err := callReviewBlockAs(t, srv, pid, bid, "fr", true, translator)
	require.Error(t, err, "deny() returns errAccessDenied after committing the 403")
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())

	// A reviewer re-approving a signed-off target keeps signed-off (never demotes).
	rec, err = callReviewBlockAs(t, srv, pid, bid, "fr", true, translator|platauth.PermReview)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"status":"signed-off"`)
	assert.Equal(t, model.TargetStatusSignedOff, getStoredBlock(t, cs, pid, bid).Target("fr").Status,
		"re-approve must not demote signed-off to reviewed")

	// Un-review without PermReview: denied, status untouched.
	rec, err = callReviewBlockAs(t, srv, pid, bid, "fr", false, translator)
	require.Error(t, err, "deny() returns errAccessDenied after committing the 403")
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusSignedOff, getStoredBlock(t, cs, pid, bid).Target("fr").Status,
		"a PermTranslate caller must not undo a sign-off")

	// With the elevated review permission the demotion is allowed.
	rec, err = callReviewBlockAs(t, srv, pid, bid, "fr", false, translator|platauth.PermReview)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusTranslated, getStoredBlock(t, cs, pid, bid).Target("fr").Status)
}

// TestHandleReviewBlockRejectDemotesToDraft: a reviewer REJECTION
// (reviewed=false + status:"draft") demotes the locale's target to draft — the
// unit re-enters the work queue, matching host/convergereport.go's
// ReviewDecisionRejected → draft mapping — while a plain un-review still lands
// on translated, and other locales stay untouched.
func TestHandleReviewBlockRejectDemotesToDraft(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.SetTargetText("de", "Hallo")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec := callReviewBlock(t, srv, pid, bid, "fr", true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec, err := callReviewBlockBodyAs(t, srv, pid, bid,
		`{"target_locale":"fr","reviewed":false,"status":"draft","item_name":"greetings.txt"}`, platauth.PermAll)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"status":"draft"`)

	got := getStoredBlock(t, cs, pid, bid)
	assert.Equal(t, model.TargetStatusDraft, got.Target("fr").Status,
		"a rejection must re-enter the work queue at draft, not stay at translated")
	assert.Equal(t, "Bonjour", got.TargetText("fr"), "rejection must not touch the translation text")
	assert.Equal(t, model.TargetStatusNew, got.Target("de").Status, "de must be untouched")

	// An unknown demotion rung is a 400 …
	rec, err = callReviewBlockBodyAs(t, srv, pid, bid,
		`{"target_locale":"fr","reviewed":false,"status":"signed-off"}`, platauth.PermAll)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	// … and so is a status alongside an approval.
	rec, err = callReviewBlockBodyAs(t, srv, pid, bid,
		`{"target_locale":"fr","reviewed":true,"status":"draft"}`, platauth.PermAll)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusDraft, getStoredBlock(t, cs, pid, bid).Target("fr").Status,
		"rejected bodies must not change stored state")
}

// TestEditorEditDemotesStaleReview: editing a reviewed translation invalidates
// the stale approval — the review decision judged the OLD text, so the edited
// target drops back to translated (the host review model binds decisions to
// the content hash of the translation they judge for the same reason). Saving
// identical content is not an edit and keeps the status.
func TestEditorEditDemotesStaleReview(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	ctx := t.Context()

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.SetTargetText("de", "Hallo")
	b.StampTargetProvenance("de", model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec := callReviewBlock(t, srv, pid, bid, "fr", true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Saving the SAME text is not an edit: the review decision still applies.
	require.NoError(t, editorUpdateBlockTarget(ctx, cs, pid, "main", bid,
		UpdateBlockTargetRequest{TargetLocale: "fr", Text: "Bonjour"}))
	assert.Equal(t, model.TargetStatusReviewed, getStoredBlock(t, cs, pid, bid).Target("fr").Status,
		"re-saving identical text must keep the reviewed status")

	// Rewriting the fr text demotes fr to translated; de's review is untouched.
	require.NoError(t, editorUpdateBlockTarget(ctx, cs, pid, "main", bid,
		UpdateBlockTargetRequest{TargetLocale: "fr", Text: "Salut"}))
	got := getStoredBlock(t, cs, pid, bid)
	assert.Equal(t, model.TargetStatusTranslated, got.Target("fr").Status,
		"an edited translation must not keep counting as reviewed")
	assert.Equal(t, "Salut", got.TargetText("fr"))
	assert.Equal(t, model.TargetStatusReviewed, got.Target("de").Status,
		"editing fr must not touch de's review status")

	// Same rule on the Run-native update endpoint.
	rec = callReviewBlock(t, srv, pid, bid, "fr", true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, editorUpdateBlockTargetRuns(ctx, cs, pid, "main", bid,
		UpdateBlockTargetRunsRequest{TargetLocale: "fr", Runs: []model.Run{{Text: &model.TextRun{Text: "Salut !"}}}}))
	assert.Equal(t, model.TargetStatusTranslated, getStoredBlock(t, cs, pid, bid).Target("fr").Status,
		"a runs-level edit must also invalidate the stale review")
}

// TestHandleReviewBlockBadRequest: target_locale is required now that review
// status is per-locale.
func TestHandleReviewBlockBadRequest(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})

	rec := callReviewBlock(t, srv, pid, ids["Hello"], "", true)
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestHandleGetFileBlocksCarriesPerLocaleStatus: the blocks endpoint the editor
// consumes serializes each target as {text, status} keyed by plain locale, so
// the UI reads block.targets[locale].status (pinned contract, epic 006).
func TestHandleGetFileBlocksCarriesPerLocaleStatus(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.SetTargetText("de", "Hallo")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec := callReviewBlock(t, srv, pid, bid, "fr", true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Call the real handler end-to-end and decode the raw JSON payload.
	e := echo.New()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/acme/"+pid+"/blocks/main?item=greetings.txt", nil)
	w := httptest.NewRecorder()
	c := e.NewContext(r, w)
	c.SetParamNames("ws", "id", "ref")
	c.SetParamValues("acme", pid, "main")
	require.NoError(t, srv.HandleGetFileBlocks(c))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var payload []struct {
		ID      string `json:"id"`
		Targets map[string]struct {
			Text   string `json:"text"`
			Status string `json:"status"`
		} `json:"targets"`
		Properties map[string]string `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Len(t, payload, 1)
	assert.Equal(t, "Bonjour", payload[0].Targets["fr"].Text)
	assert.Equal(t, "reviewed", payload[0].Targets["fr"].Status)
	assert.Equal(t, "Hallo", payload[0].Targets["de"].Text)
	assert.Empty(t, payload[0].Targets["de"].Status)
	_, hasLegacy := payload[0].Properties[legacyTranslationStatusProperty]
	assert.False(t, hasLegacy)
}

// TestEditorReviewFeedsConvergenceCoverage is the convergence proof for epic
// 006 task 3: a block reviewed through the editor endpoint counts toward that
// locale ONLY in the convergence/coverage reviewed numbers.
//
// The chain, made explicit because the host module cannot import bowrain
// storage:
//
//  1. HandleReviewBlock sets Block.Targets[Variant(locale)].Status =
//     model.TargetStatusReviewed (this package).
//  2. The Postgres ContentStore persists the FULL model.Target JSON (runs +
//     status + origin) per variant (bowrain/store/overlay_sync.go,
//     SyncBlockOverlays) and GetBlocks round-trips it — asserted here against
//     the real database.
//  3. Host convergence consumes exactly that field: unitState is an alias for
//     convergence.TargetState (host/convergence_alias.go:26), which returns
//     the committed Target.Status when a non-empty target exists
//     (core/convergence/convergence.go), and host.ComputeShipCoverage feeds it
//     into convergence.CoverageTally (host/coverage.go). decisionStatus in
//     host/convergereport.go maps "approved" to the SAME
//     model.TargetStatusReviewed rung.
//
// This test drives steps 1–2 through the real handler + Postgres, then runs
// step 3's exact functions (convergence.TargetState + CoverageTally +
// gate.TargetLadder) over the round-tripped blocks and asserts the reviewed
// percentage moved for fr only.
func TestEditorReviewFeedsConvergenceCoverage(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Hello")
	b1.SetTargetText("fr", "Bonjour")
	b1.SetTargetText("de", "Hallo")
	b2 := &model.Block{ID: "b2", Translatable: true}
	b2.SetSourceText("Goodbye")
	b2.SetTargetText("fr", "Au revoir")
	b2.SetTargetText("de", "Tschüss")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b1, b2})

	// Approve ONE block for fr through the editor endpoint.
	rec := callReviewBlock(t, srv, pid, ids["Hello"], "fr", true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Round-trip every block from Postgres and tally coverage exactly the way
	// host.ComputeShipCoverage does: state := unitState(block, locale) (aliased
	// to convergence.TargetState), then tally.Add(scope, state).
	stored, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: pid, Stream: "main", ItemName: "greetings.txt",
	})
	require.NoError(t, err)
	require.Len(t, stored, 2)

	tally := convergence.NewCoverageTally()
	for _, sb := range stored {
		for _, locale := range []string{"fr", "de"} {
			state := convergence.TargetState(sb.Block, locale)
			tally.Add(convergence.Scope{Locale: locale}, state)
			// The reviewed block reads at the reviewed rung for fr only.
			if sb.Block.ID == ids["Hello"] && locale == "fr" {
				assert.Equal(t, string(model.TargetStatusReviewed), state)
			} else {
				assert.Equal(t, string(model.TargetStatusTranslated), state)
			}
		}
	}

	ladder := gate.TargetLadder()
	fr, ok := tally.Coverage(convergence.Scope{Locale: "fr"})
	require.True(t, ok)
	de, ok := tally.Coverage(convergence.Scope{Locale: "de"})
	require.True(t, ok)

	assert.InDelta(t, 50, fr.AtLeastPct(ladder, string(model.TargetStatusReviewed)), 1e-9,
		"fr: 1 of 2 blocks reviewed via the editor endpoint")
	assert.InDelta(t, 0, de.AtLeastPct(ladder, string(model.TargetStatusReviewed)), 1e-9,
		"de: reviewing fr must not move de's reviewed coverage")
	assert.InDelta(t, 100, fr.AtLeastPct(ladder, string(model.TargetStatusTranslated)), 1e-9)
	assert.InDelta(t, 100, de.AtLeastPct(ladder, string(model.TargetStatusTranslated)), 1e-9)
}
