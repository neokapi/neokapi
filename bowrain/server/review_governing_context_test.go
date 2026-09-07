package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// governingAuthStore answers the one question the review ledger asks of the
// auth store: the slug of the workspace holding a project's stores.
type governingAuthStore struct {
	auth.AuthStore
	slug string
}

func (a governingAuthStore) GetWorkspace(_ context.Context, id string) (*platauth.Workspace, error) {
	if a.slug == "" {
		return nil, errors.New("workspace not found: " + id)
	}
	return &platauth.Workspace{ID: id, Slug: a.slug}, nil
}

// GetUser is the other question: the readable name of the decider, which the
// ledger falls back to the user id for.
func (a governingAuthStore) GetUser(_ context.Context, id string) (*platauth.User, error) {
	return &platauth.User{ID: id, Email: id}, nil
}

// governingLedgerFixture is a server over the voice-context fixture: one
// project in a workspace whose default voice profile is bound and whose terms
// mandate a rendering, plus the decision ledger the SQLite store keeps.
func governingLedgerFixture(t *testing.T) (*Server, platstore.ContentStore, *platstore.Project) {
	t.Helper()
	cs, voiceCtx, proj := editorVoiceContextFixture(t)
	srv := &Server{
		ContentStore: cs,
		VoiceStore:   voiceCtx.Voice,
		AuthStore:    governingAuthStore{slug: "acme"},
		wsStores:     voiceCtx.Stores,
	}
	return srv, cs, proj
}

// ledgerFor opens a review ledger the way a request does, with a decider.
func ledgerFor(t *testing.T, srv *Server, projectID string) *reviewLedger {
	t.Helper()
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodPut, "/", nil), httptest.NewRecorder())
	c.Set("user_id", "reviewer@example.com")
	l := srv.newReviewLedger(t.Context(), c, projectID, "main")
	require.NotNil(t, l, "the content store keeps a decision ledger")
	return l
}

func storedBlockFor(t *testing.T, cs platstore.ContentStore, projectID string) *venue.StoredBlock {
	t.Helper()
	blocks, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projectID, Stream: "main", ItemName: "hello.txt"})
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	return blocks[0]
}

// TestPlatformApprovalRecordsTheGoverningContext proves an approval made on the
// platform carries what governed it, and that the fingerprint is the one a
// translation of the same unit under the same binding would be stamped with.
func TestPlatformApprovalRecordsTheGoverningContext(t *testing.T) {
	srv, cs, proj := governingLedgerFixture(t)
	ledger := ledgerFor(t, srv, proj.ID)
	sb := storedBlockFor(t, cs, proj.ID)

	governing := ledger.governingFingerprint(t.Context(), sb.ItemName, "fr")
	require.NotEmpty(t, governing, "a bound voice profile and terms govern this unit")

	// The same value the translate binding folds, which is what a producer
	// stamps on the target it writes.
	cfg := jobs.BuildTranslateConfig(t.Context(), jobs.TranslateBinding{
		Store:            cs,
		Voice:            srv.VoiceStore,
		WorkspaceDefault: srv.editorVoiceContext().WorkspaceDefault,
		Terms:            editorTerms(t.Context(), srv.editorVoiceContext(), "acme"),
		Project:          proj,
		WorkspaceID:      proj.WorkspaceID,
		ProjectID:        proj.ID,
		Stream:           "main",
		ItemName:         sb.ItemName,
		TargetLocale:     "fr",
	})
	_, _, want := coreprofile.GovernanceContext(cfg.Profile, cfg.TermRules)
	assert.Equal(t, want, governing, "a decision and a translation must name one context")

	ledger.write(t.Context(), []venue.UnitDecision{
		unitDecisionFor(sb, "fr", model.TargetStatusReviewed, true, ledger.decider, governing, nil),
	})

	row := srv.unitDecisionFor(t.Context(), proj.ID, "main", sb, "fr")
	require.NotNil(t, row, "the approval reached the ledger")
	assert.Equal(t, "approved", row.ReviewState)
	assert.Equal(t, governing, row.GoverningFingerprint)
	assert.Equal(t, "reviewer@example.com", row.DecidedBy)

	// A pull sends the ledger whole, so the fingerprint travels to the project
	// with the verdict it belongs to.
	ds, ok := cs.(platstore.DecisionStore)
	require.True(t, ok)
	pulled, err := ds.ListUnitDecisions(t.Context(), proj.ID, "main")
	require.NoError(t, err)
	require.Len(t, pulled, 1)
	assert.Equal(t, governing, pulled[0].GoverningFingerprint)

	// And the value survives the wire encoding the pull response uses.
	encoded, err := json.Marshal(pulled)
	require.NoError(t, err)
	var decoded []venue.UnitDecision
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Len(t, decoded, 1)
	assert.Equal(t, governing, decoded[0].GoverningFingerprint)
}

// TestGoverningFingerprintIsResolvedOncePerUnitAndLocale proves the bulk path
// pays for the resolution once per (item, locale) rather than once per block.
func TestGoverningFingerprintIsResolvedOncePerUnitAndLocale(t *testing.T) {
	srv, cs, proj := governingLedgerFixture(t)
	ledger := ledgerFor(t, srv, proj.ID)
	sb := storedBlockFor(t, cs, proj.ID)

	first := ledger.governingFingerprint(t.Context(), sb.ItemName, "fr")
	require.NotEmpty(t, first)
	require.Len(t, ledger.governing, 1)

	assert.Equal(t, first, ledger.governingFingerprint(t.Context(), sb.ItemName, "fr"))
	assert.Len(t, ledger.governing, 1, "the same unit and locale is resolved once")

	ledger.governingFingerprint(t.Context(), sb.ItemName, "de")
	assert.Len(t, ledger.governing, 2, "another locale is another context")
}

// TestGoverningFingerprintIsEmptyWithoutGovernance pins the ungoverned case: a
// workspace binding no voice profile and no terminology records no fingerprint,
// which reads as an ad-hoc decision rather than failing the review.
func TestGoverningFingerprintIsEmptyWithoutGovernance(t *testing.T) {
	_, cs, proj := governingLedgerFixture(t)
	srv := &Server{
		ContentStore: cs,
		AuthStore:    governingAuthStore{slug: "acme"},
		wsStores:     newWorkspaceStores(),
	}
	ledger := ledgerFor(t, srv, proj.ID)
	sb := storedBlockFor(t, cs, proj.ID)

	assert.Empty(t, ledger.governingFingerprint(t.Context(), sb.ItemName, "fr"))
}

// TestGoverningFingerprintSurvivesAnUnresolvableWorkspace proves the review is
// never failed by a scope that cannot be read.
func TestGoverningFingerprintSurvivesAnUnresolvableWorkspace(t *testing.T) {
	srv, cs, proj := governingLedgerFixture(t)
	srv.AuthStore = governingAuthStore{}
	ledger := ledgerFor(t, srv, proj.ID)
	sb := storedBlockFor(t, cs, proj.ID)

	assert.Empty(t, ledger.governingFingerprint(t.Context(), sb.ItemName, "fr"))
}

// TestReviewContextReadsTheRecordedGoverningContext proves the read side uses
// the recorded fingerprint: a target carrying no stamp of its own is judged
// against what the decision record says governed it.
func TestReviewContextReadsTheRecordedGoverningContext(t *testing.T) {
	srv, cs, proj := governingLedgerFixture(t)
	ledger := ledgerFor(t, srv, proj.ID)
	sb := storedBlockFor(t, cs, proj.ID)

	governing := ledger.governingFingerprint(t.Context(), sb.ItemName, "fr")
	require.NotEmpty(t, governing)
	ledger.write(t.Context(), []venue.UnitDecision{
		unitDecisionFor(sb, "fr", model.TargetStatusReviewed, true, ledger.decider, governing, nil),
	})

	row := srv.unitDecisionFor(t.Context(), proj.ID, "main", sb, "fr")
	require.NotNil(t, row)
	assert.Equal(t, governing, recordedGoverning(row))
	assert.Empty(t, recordedGoverning(nil), "no record is no context")
}
