package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover a review decision on the server: decide, sent to a
// stream's changes route, stores the decision as the per-locale
// model.Edition.Status on the block's translation for the decided locale — the
// framework target ladder that convergence, coverage and the ship gates
// consume. They run against the real PostgreSQL ContentStore
// (testcontainers), so every assertion covers the full route → change
// service → stream home → translations-table round trip.

// newReviewTestServer builds a minimal Server over a real Postgres content
// store — enough for the changes route and the blocks handlers (no auth
// middleware; tests grant permissions directly on the echo context).
func newReviewTestServer(t *testing.T) (*Server, *bstore.PostgresStore) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	srv := &Server{ContentStore: cs, wsStores: newWorkspaceStores()}
	srv.wsStores.memoryFactory = func() memory.Store { return &testMemoryStore{memory.NewInMemoryStore()} }
	srv.wsStores.termsFactory = func() terms.Store { return &testTermStore{terms.NewInMemoryStore()} }
	return srv, cs
}

// seedReviewProject creates a project (en → fr, de) with one item and the
// given blocks, and returns the project ID plus the stored blocks' internal
// IDs keyed by source text (StoreBlocksForItem remaps reader IDs).
func seedReviewProject(t *testing.T, cs *bstore.PostgresStore, blocks []*model.Block) (string, map[string]string) {
	t.Helper()
	return seedReviewProjectWith(t, cs, map[string]string{}, blocks)
}

// seedReviewProjectWith is seedReviewProject with the project's properties.
func seedReviewProjectWith(t *testing.T, cs *bstore.PostgresStore, props map[string]string, blocks []*model.Block) (string, map[string]string) {
	t.Helper()
	ctx := t.Context()
	proj := &platstore.Project{
		Name:                  "review-proj",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr", "de"},
		WorkspaceID:           "ws-1",
		Properties:            props,
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

// decideAs sends one decision on a block's translation, guarded by the
// revision the stream holds, as who.
func decideAs(t *testing.T, srv *Server, cs *bstore.PostgresStore, pid, bid, locale string, outcome change.Outcome, who changeCaller) (*httptest.ResponseRecorder, change.Result) {
	t.Helper()
	rev := targetRev(t, cs, pid, bid, locale)
	return sendChanges(t, srv, pid, who, change.Set{Ops: []change.Op{decide(at("greetings.txt", bid, locale), rev, outcome)}})
}

// decideOn sends one decision with every permission.
func decideOn(t *testing.T, srv *Server, cs *bstore.PostgresStore, pid, bid, locale string, outcome change.Outcome) (*httptest.ResponseRecorder, change.Result) {
	t.Helper()
	return decideAs(t, srv, cs, pid, bid, locale, outcome, fullCaller)
}

func getStoredBlock(t *testing.T, cs *bstore.PostgresStore, pid, bid string) *model.Block {
	t.Helper()
	sb, err := cs.GetBlock(t.Context(), pid, "main", bid)
	require.NoError(t, err)
	return sb.Block
}

// draftMarks is every draft mark the project's ledger holds on main.
func draftMarks(t *testing.T, cs *bstore.PostgresStore, pid string) []platstore.DraftBasis {
	t.Helper()
	marks, err := cs.ListDraftBases(t.Context(), pid, "main")
	require.NoError(t, err)
	return marks
}

// Establishing fr sets fr's Edition.Status to established without touching de,
// and never writes the legacy block-global property.
func TestDecide_EstablishSetsOneLocale(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.SetTargetText("de", "Hallo")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec, res := decideOn(t, srv, cs, pid, bid, "fr", change.OutcomeEstablish)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, change.SetApplied, res.Status)
	assert.Equal(t, change.OpApplied, res.Ops[0].Status)

	got := getStoredBlock(t, cs, pid, bid)
	require.True(t, holdsTarget(got, "fr"))
	assert.Equal(t, model.TargetStatusEstablished, targetStatusOf(t, got, "fr"), "fr must be established")
	require.True(t, holdsTarget(got, "de"))
	assert.Equal(t, model.TargetStatusNew, targetStatusOf(t, got, "de"), "de must be untouched")
	assert.Equal(t, "Bonjour", got.TargetText("fr"), "a decision must not touch the translation text")
	_, hasLegacy := got.Properties[legacyTranslationStatusProperty]
	assert.False(t, hasLegacy, "the legacy block-global property must never be written")
}

// Withdrawing fr's approval moves it back to translated, still without touching
// de.
func TestDecide_WithdrawMovesBackToTranslated(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.SetTargetText("de", "Hallo")
	b.StampTargetProvenance("de", model.TargetStatusEstablished, model.Origin{Kind: model.OriginHuman})
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec, _ := decideOn(t, srv, cs, pid, bid, "fr", change.OutcomeEstablish)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec, _ = decideOn(t, srv, cs, pid, bid, "fr", change.OutcomeWithdraw)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	got := getStoredBlock(t, cs, pid, bid)
	assert.Equal(t, model.TargetStatusTranslated, targetStatusOf(t, got, "fr"), "fr must be back at translated")
	assert.Equal(t, model.TargetStatusEstablished, targetStatusOf(t, got, "de"), "de's own established status must survive fr's withdrawal")
}

// Establishing a locale that has no non-empty translation is refused with 422:
// established is a rung on the target ladder, and convergence only counts a
// status when a non-empty translation exists. Withdrawing a locale with no
// translation is an idempotent no-op.
func TestDecide_ALocaleWithNoTranslationIsNotEstablished(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("de", "Hallo") // fr has NO target
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec, res := decideOn(t, srv, cs, pid, bid, "fr", change.OutcomeEstablish)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	assert.Contains(t, res.Ops[0].Error.Message, "no fr translation to establish")
	assert.False(t, holdsTarget(getStoredBlock(t, cs, pid, bid), "fr"), "a refused approval must not create a target")

	rec, res = decideOn(t, srv, cs, pid, bid, "fr", change.OutcomeWithdraw)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, change.OpUnchanged, res.Ops[0].Status)
	assert.False(t, holdsTarget(getStoredBlock(t, cs, pid, bid), "fr"))

	// An empty translation is not reviewable either.
	require.NoError(t, cs.StoreBlocks(t.Context(), pid, "main", func() []*model.Block {
		blk := getStoredBlock(t, cs, pid, bid)
		blk.SetTargetText("fr", "   ")
		return []*model.Block{blk}
	}()))
	rec, _ = decideOn(t, srv, cs, pid, bid, "fr", change.OutcomeEstablish)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
}

// A block reviewed under the old scheme (block-global property, no per-locale
// status) reads as established through the documented fallback. Withdrawing a
// locale with no translation clears the stuck legacy flag.
func TestDecide_AWithdrawalClearsTheLegacyFlag(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true, Properties: map[string]string{
		legacyTranslationStatusProperty: "established",
	}}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b2 := &model.Block{ID: "b2", Translatable: true, Properties: map[string]string{
		legacyTranslationStatusProperty: "established",
	}}
	b2.SetSourceText("Goodbye")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b, b2})

	blocks, err := editorGetBlocks(t.Context(), cs, pid, "main", "greetings.txt", []string{"fr", "de"}, platstore.DefaultBlockLimit, 0)
	require.NoError(t, err)
	byID := map[string]BlockInfoResponse{}
	for _, bi := range blocks {
		byID[bi.ID] = bi
	}
	legacy := byID[ids["Hello"]]
	assert.Equal(t, "Bonjour", legacy.Targets["fr"].Text)
	assert.Empty(t, legacy.Targets["fr"].Status, "legacy block carries no per-locale status")
	assert.Equal(t, "established", legacy.Properties[legacyTranslationStatusProperty],
		"legacy property must survive as the read fallback")

	rec, _ := decideOn(t, srv, cs, pid, ids["Hello"], "fr", change.OutcomeEstablish)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusEstablished, targetStatusOf(t, getStoredBlock(t, cs, pid, ids["Hello"]), "fr"))

	rec, _ = decideOn(t, srv, cs, pid, ids["Goodbye"], "fr", change.OutcomeWithdraw)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	_, hasLegacy := getStoredBlock(t, cs, pid, ids["Goodbye"]).Properties[legacyTranslationStatusProperty]
	assert.False(t, hasLegacy, "legacy flag must be cleared when withdrawing a no-target locale")
}

// Established is the top rung of the target ladder. A translator reaches
// neither end of it: establishing needs review for the language, and so does
// moving an established translation. A reviewer re-establishes it as an
// idempotent no-op and may move it.
func TestDecide_EstablishedTakesTheReviewPermission(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.StampTargetProvenance("fr", model.TargetStatusEstablished, model.Origin{Kind: model.OriginHuman})
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	reviewer := translateCaller
	reviewer.perms |= platauth.PermReview

	rec, res := decideAs(t, srv, cs, pid, bid, "fr", change.OutcomeEstablish, translateCaller)
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Equal(t, change.CodeNotPermitted, res.Ops[0].Error.Code)

	rec, res = decideAs(t, srv, cs, pid, bid, "fr", change.OutcomeEstablish, reviewer)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, change.OpUnchanged, res.Ops[0].Status, "re-establishing changes nothing")
	assert.Equal(t, model.TargetStatusEstablished, targetStatusOf(t, getStoredBlock(t, cs, pid, bid), "fr"))

	rec, res = decideAs(t, srv, cs, pid, bid, "fr", change.OutcomeWithdraw, translateCaller)
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Equal(t, change.CodeNotPermitted, res.Ops[0].Error.Code)
	assert.Equal(t, model.TargetStatusEstablished, targetStatusOf(t, getStoredBlock(t, cs, pid, bid), "fr"),
		"a translator must not undo an approval")

	rec, _ = decideAs(t, srv, cs, pid, bid, "fr", change.OutcomeWithdraw, reviewer)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusTranslated, targetStatusOf(t, getStoredBlock(t, cs, pid, bid), "fr"))
}

// A rejection moves the translation to draft, so the unit re-enters the work
// queue, and clears the platform's draft mark so it is drafted again; other
// locales stay untouched.
func TestDecide_RejectMovesToDraft(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.SetTargetText("de", "Hallo")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec, _ := decideOn(t, srv, cs, pid, bid, "fr", change.OutcomeEstablish)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	ctx := t.Context()
	var approval venue.UnitDecision
	rows, err := cs.ListUnitDecisions(ctx, pid, "main")
	require.NoError(t, err)
	for _, d := range rows {
		if d.Variant == "fr" {
			approval = d
		}
	}
	require.NotEmpty(t, approval.Unit, "the approval is recorded in the ledger")
	require.NoError(t, cs.RecordDraftBases(ctx, pid, "main", []platstore.DraftBasis{{
		ItemName: approval.ItemName, Unit: approval.Unit, Variant: approval.Variant, Basis: approval.Basis,
	}}))
	require.Len(t, draftMarks(t, cs, pid), 1)

	rec, _ = decideOn(t, srv, cs, pid, bid, "fr", change.OutcomeReject)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Empty(t, draftMarks(t, cs, pid), "the rejection clears the draft mark, so the unit is drafted again")

	got := getStoredBlock(t, cs, pid, bid)
	assert.Equal(t, model.TargetStatusDraft, targetStatusOf(t, got, "fr"), "a rejection re-enters the work queue at draft")
	assert.Equal(t, "Bonjour", got.TargetText("fr"), "a rejection must not touch the translation text")
	assert.Equal(t, model.TargetStatusNew, targetStatusOf(t, got, "de"), "de must be untouched")

	// A pre-review is an agent's, and a stream decides on translations.
	rec, res := decideOn(t, srv, cs, pid, bid, "fr", change.OutcomeAdvise)
	assert.Equal(t, change.CodeNotPermitted.HTTPStatus(), rec.Code, rec.Body.String())
	assert.Equal(t, change.CodeNotPermitted, res.Ops[0].Error.Code)
	rec, res = sendChanges(t, srv, pid, fullCaller, change.Set{Ops: []change.Op{
		decide(at("greetings.txt", bid, ""), sourceRev(t, cs, pid, bid), change.OutcomeEstablish)}})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	assert.Equal(t, model.TargetStatusDraft, targetStatusOf(t, getStoredBlock(t, cs, pid, bid), "fr"),
		"refused decisions do not change stored state")
}

// Editing an established translation invalidates the approval: the decision
// judged the old wording, so a person's edit drops it back to translated.
// Saving identical content is not an edit and keeps the status.
func TestChanges_AnEditMovesAnEstablishedTranslationBack(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.SetTargetText("de", "Hallo")
	b.StampTargetProvenance("de", model.TargetStatusEstablished, model.Origin{Kind: model.OriginHuman})
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec, _ := decideOn(t, srv, cs, pid, bid, "fr", change.OutcomeEstablish)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec, res := sendChanges(t, srv, pid, fullCaller, change.Set{Ops: []change.Op{
		setText(at("greetings.txt", bid, "fr"), targetRev(t, cs, pid, bid, "fr"), "Bonjour")}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, change.OpUnchanged, res.Ops[0].Status)
	assert.Equal(t, model.TargetStatusEstablished, targetStatusOf(t, getStoredBlock(t, cs, pid, bid), "fr"),
		"re-saving identical text keeps the established status")

	rec, _ = sendChanges(t, srv, pid, fullCaller, change.Set{Ops: []change.Op{
		setText(at("greetings.txt", bid, "fr"), targetRev(t, cs, pid, bid, "fr"), "Salut")}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got := getStoredBlock(t, cs, pid, bid)
	assert.Equal(t, model.TargetStatusTranslated, targetStatusOf(t, got, "fr"), "an edited translation does not stay established")
	fr, ok := got.TargetEdition("fr")
	require.True(t, ok)
	assert.Equal(t, model.OriginHuman, fr.Origin.Kind)
	assert.Equal(t, "Salut", got.TargetText("fr"))
	assert.Equal(t, model.TargetStatusEstablished, targetStatusOf(t, got, "de"), "editing fr does not touch de's status")

	// The same holds for content sent as runs.
	rec, _ = decideOn(t, srv, cs, pid, bid, "fr", change.OutcomeEstablish)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec, _ = sendChanges(t, srv, pid, fullCaller, change.Set{Ops: []change.Op{{
		Kind: change.KindSetContent, At: at("greetings.txt", bid, "fr"), IfMatch: targetRev(t, cs, pid, bid, "fr"),
		Body: &change.SetContent{Runs: []model.Run{{Text: &model.TextRun{Text: "Salut !"}}}},
	}}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusTranslated, targetStatusOf(t, getStoredBlock(t, cs, pid, bid), "fr"))
}

// The blocks endpoint the editor consumes serializes each target as {text,
// status} keyed by plain locale, so the UI reads block.targets[locale].status.
func TestHandleGetFileBlocksCarriesPerLocaleStatus(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.SetTargetText("de", "Hallo")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec, _ := decideOn(t, srv, cs, pid, bid, "fr", change.OutcomeEstablish)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

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
	assert.Equal(t, "established", payload[0].Targets["fr"].Status)
	assert.Equal(t, "Hallo", payload[0].Targets["de"].Text)
	assert.Empty(t, payload[0].Targets["de"].Status)
	_, hasLegacy := payload[0].Properties[legacyTranslationStatusProperty]
	assert.False(t, hasLegacy)
}

// A block established through the changes route counts toward that locale only
// in the convergence and coverage established numbers: the route stores the
// per-locale Edition.Status, Postgres round-trips it, and convergence.TargetState
// and CoverageTally read exactly that field.
func TestDecide_FeedsConvergenceCoverage(t *testing.T) {
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

	rec, _ := decideOn(t, srv, cs, pid, ids["Hello"], "fr", change.OutcomeEstablish)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

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
			if sb.Block.ID == ids["Hello"] && locale == "fr" {
				assert.Equal(t, string(model.TargetStatusEstablished), state)
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

	assert.InDelta(t, 50, fr.AtLeastPct(ladder, string(model.TargetStatusEstablished)), 1e-9,
		"fr: 1 of 2 blocks established")
	assert.InDelta(t, 0, de.AtLeastPct(ladder, string(model.TargetStatusEstablished)), 1e-9,
		"de: establishing fr does not move de's coverage")
	assert.InDelta(t, 100, fr.AtLeastPct(ladder, string(model.TargetStatusTranslated)), 1e-9)
	assert.InDelta(t, 100, de.AtLeastPct(ladder, string(model.TargetStatusTranslated)), 1e-9)
}
