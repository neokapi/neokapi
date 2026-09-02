package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spCtx builds an authenticated echo context for the source-proposal handlers,
// setting the workspace/project params, the caller's project permissions, and the
// acting user. pid, when non-empty, wires the :pid path param (the decide route).
func spCtx(wsID, projID, pid string, perms platauth.Permission, userID, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/rc/"+projID+"/source-proposals", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	if pid != "" {
		c.SetParamNames("ws", "id", "pid")
		c.SetParamValues("rc", projID, pid)
	} else {
		c.SetParamNames("ws", "id")
		c.SetParamValues("rc", projID)
	}
	c.Set("project_permissions", perms)
	c.Set("workspace_id", wsID)
	c.Set("user_id", userID)
	return c, rec
}

// seedMultiLocaleProject seeds a governed project with fr + de target locales and
// the given blocks — the setup the source→all-locales fan-out proof needs. It
// returns the project id and the source-text → stored-block-id map (the store
// reassigns content-keyed block ids, so callers reference blocks by source text).
func seedMultiLocaleProject(t *testing.T, s *Server, wsID string, blocks []*model.Block) (string, map[string]string) {
	t.Helper()
	ctx := context.Background()
	proj := &platstore.Project{
		Name:                  "SP Proj",
		WorkspaceID:           wsID,
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr", "de"},
		Properties:            map[string]string{"source_gate": "none"},
	}
	require.NoError(t, s.ContentStore.CreateProject(ctx, proj))
	require.NoError(t, s.ContentStore.StoreItem(ctx, proj.ID, "main", &platstore.Item{
		Name: "ui.json", Format: "json", ItemType: "file",
	}))
	require.NoError(t, s.ContentStore.StoreBlocksForItem(ctx, proj.ID, "main", "ui.json", blocks))
	stored, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{ProjectID: proj.ID, Stream: "main", ItemName: "ui.json"})
	require.NoError(t, err)
	ids := map[string]string{}
	for _, sb := range stored {
		ids[sb.Block.SourceText()] = sb.Block.ID
	}
	return proj.ID, ids
}

// TestSourceProposal_ProposeApproveDemotesEveryLocale (RV-F) is the required
// end-to-end proof for the back-to-source lane: a reviewer working on fr proposes
// a source-text fix; a reviewer WITHOUT PermEditSource cannot approve it; a source
// owner (PermEditSource) approves it, which applies the change to the block's
// source and DEMOTES every target locale (fr AND de), the fan-out from one
// reviewer's find to all locales. Each translation stays where it is, carrying a
// basis that now names wording the block no longer holds, which is what the
// recycle pass re-drafts on. A convergence run is started (the re-draft nudge)
// and the proposal is closed.
func TestSourceProposal_ProposeApproveDemotesEveryLocale(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	ctx := context.Background()

	// A block whose source has a British spelling and whose fr + de targets are
	// both already APPROVED (reviewed) — translations of the pre-fix source.
	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Colour picker")
	b1.SetTargetText("fr", "Sélecteur de couleur")
	b1.Target("fr").Status = model.TargetStatusReviewed
	b1.SetTargetText("de", "Farbwähler")
	b1.Target("de").Status = model.TargetStatusReviewed
	projID, ids := seedMultiLocaleProject(t, s, wsID, []*model.Block{b1})
	blockID := ids["Colour picker"]
	require.NotEmpty(t, blockID)

	// The ledger holds the fr approval, recorded against the source as it reads
	// now. The source change is about to make that basis stale.
	ds, isLedger := s.ContentStore.(platstore.DecisionStore)
	require.True(t, isLedger)
	_, uerr := ds.UpsertUnitDecisions(ctx, projID, "main", []venue.UnitDecision{{
		ItemName:    "ui.json",
		Unit:        "b1",
		Variant:     "fr",
		Status:      string(model.TargetStatusReviewed),
		TargetHash:  state.TargetHash("Sélecteur de couleur"),
		ContentHash: state.SourceHash("Colour picker"),
		ReviewState: "approved",
		DecidedBy:   "reviewer-1",
		Updated:     "2026-01-01T00:00:00Z",
	}})
	require.NoError(t, uerr)

	// A reviewer on fr proposes fixing the source spelling. PermReview is enough to
	// PROPOSE — no PermEditSource needed yet.
	createBody := `{"block_id":"` + blockID + `","item_name":"ui.json","proposed_source":"Color picker","found_in_locale":"fr","rationale":"US spelling"}`
	cCreate, recCreate := spCtx(wsID, projID, "", platauth.PermReview|platauth.PermViewContent, "reviewer-1", createBody)
	require.NoError(t, s.HandleCreateSourceProposal(cCreate))
	require.Equal(t, http.StatusCreated, recCreate.Code)

	var created bstore.ProposedSourceChange
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &created))
	require.NotEmpty(t, created.ID)
	assert.Equal(t, bstore.SourceProposalOpen, created.Status)
	assert.Equal(t, "Colour picker", created.OriginalSource, "the proposal snapshots the original source")
	assert.Equal(t, "fr", created.FoundInLocale, "attribution: the locale the finder was reviewing")

	// The open proposal is visible on the source-owner review surface.
	openBefore, err := s.SourceProposalStore.ListOpen(ctx, projID)
	require.NoError(t, err)
	require.Len(t, openBefore, 1, "the open proposal surfaces for owners")

	// A reviewer WITHOUT PermEditSource cannot approve — the source-authoring gate.
	cDenied, recDenied := spCtx(wsID, projID, created.ID, platauth.PermReview|platauth.PermViewContent, "reviewer-1", `{"decision":"approve"}`)
	_ = s.HandleDecideSourceProposal(cDenied)
	assert.Equal(t, http.StatusForbidden, recDenied.Code, "approving a source change requires PermEditSource")

	// The source owner approves → applies + re-drafts every locale.
	cDecide, recDecide := spCtx(wsID, projID, created.ID, platauth.PermEditSource|platauth.PermViewContent, ownerID, `{"decision":"approve"}`)
	require.NoError(t, s.HandleDecideSourceProposal(cDecide))
	require.Equal(t, http.StatusOK, recDecide.Code)
	var decideResp map[string]any
	require.NoError(t, json.Unmarshal(recDecide.Body.Bytes(), &decideResp))
	assert.Equal(t, true, decideResp["applied"])
	assert.Equal(t, true, decideResp["run_started"], "approval starts a convergence run to re-draft")

	// The source is updated and EVERY target locale is demoted (the fan-out). The
	// translations stay: they are what the content memory recycles from and what a
	// reviewer compares the new draft against.
	sb, err := s.ContentStore.GetBlock(ctx, projID, "main", blockID)
	require.NoError(t, err)
	assert.Equal(t, "Color picker", sb.Block.SourceText(), "the approved source change is applied")
	require.NotNil(t, sb.Block.Target("fr"), "the fr translation is kept")
	assert.Equal(t, "Sélecteur de couleur", sb.Block.TargetText("fr"))
	assert.Equal(t, model.TargetStatusTranslated, sb.Block.Target("fr").Status,
		"the fr approval blessed wording the source no longer carries → back to the presence baseline")
	require.NotNil(t, sb.Block.Target("de"), "the de translation is kept too, so the fan-out reaches ALL locales")
	assert.Equal(t, "Farbwähler", sb.Block.TargetText("de"))
	assert.Equal(t, model.TargetStatusTranslated, sb.Block.Target("de").Status)
	assert.Equal(t, model.SourceStatusNew, sb.Block.SourceStatus, "the changed source re-settles on the next pass")

	// The ledger grades the kept translation against the source the project holds
	// now: the fr approval's basis names wording that is gone.
	tallies, terr := ds.TallyDecisionBasis(ctx, projID, "main")
	require.NoError(t, terr)
	stale := 0
	for _, tl := range tallies {
		if tl.Variant == "fr" {
			stale += tl.Stale
		}
	}
	assert.Equal(t, 1, stale, "the fr decision reads stale against the rewritten source")

	// A convergence run was started to drive the re-draft.
	runs, err := s.ConvergenceRunStore.ListRuns(ctx, projID, 20)
	require.NoError(t, err)
	assert.NotEmpty(t, runs, "approving a source change starts a convergence run (the re-draft nudge)")

	// The proposal is now approved and no longer open.
	got, err := s.SourceProposalStore.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.SourceProposalApproved, got.Status)
	assert.Equal(t, ownerID, got.DecidedBy)

	open, err := s.SourceProposalStore.ListOpen(ctx, projID)
	require.NoError(t, err)
	assert.Empty(t, open, "the approved proposal is no longer open")
}

// TestSourceProposal_Reject leaves the source and every target untouched.
func TestSourceProposal_Reject(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	ctx := context.Background()

	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Colour picker")
	b1.SetTargetText("fr", "Sélecteur de couleur")
	b1.Target("fr").Status = model.TargetStatusReviewed
	projID, ids := seedMultiLocaleProject(t, s, wsID, []*model.Block{b1})
	blockID := ids["Colour picker"]

	cCreate, recCreate := spCtx(wsID, projID, "", platauth.PermReview, "reviewer-1",
		`{"block_id":"`+blockID+`","item_name":"ui.json","proposed_source":"Color picker","found_in_locale":"fr"}`)
	require.NoError(t, s.HandleCreateSourceProposal(cCreate))
	var created bstore.ProposedSourceChange
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &created))

	cDecide, recDecide := spCtx(wsID, projID, created.ID, platauth.PermEditSource, ownerID,
		`{"decision":"reject","reason":"British spelling is intentional"}`)
	require.NoError(t, s.HandleDecideSourceProposal(cDecide))
	require.Equal(t, http.StatusOK, recDecide.Code)

	sb, err := s.ContentStore.GetBlock(ctx, projID, "main", blockID)
	require.NoError(t, err)
	assert.Equal(t, "Colour picker", sb.Block.SourceText(), "reject leaves the source untouched")
	require.NotNil(t, sb.Block.Target("fr"))
	assert.Equal(t, model.TargetStatusReviewed, sb.Block.Target("fr").Status, "reject leaves the target approved")

	got, err := s.SourceProposalStore.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.SourceProposalRejected, got.Status)

	runs, err := s.ConvergenceRunStore.ListRuns(ctx, projID, 20)
	require.NoError(t, err)
	assert.Empty(t, runs, "reject starts no convergence run")
}
