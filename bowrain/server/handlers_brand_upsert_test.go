package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upsertBrandProfile posts payload to HandleUpsertBrandProfile in wsID as
// userID with full permissions, returning the recorder and decoded response.
func upsertBrandProfile(t *testing.T, srv *Server, wsID, userID, payload string) (*httptest.ResponseRecorder, BrandProfileUpsertResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.Set("workspace_id", wsID)
	c.Set("user_id", userID)
	require.NoError(t, srv.HandleUpsertBrandProfile(c))
	var out BrandProfileUpsertResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	return rec, out
}

// TestBrandUpsert_CreateThenIdempotentRepush proves the push upsert creates a
// workspace profile on first contact and that a re-push of identical content
// is a pure no-op: same version, no archived snapshot, action "unchanged".
func TestBrandUpsert_CreateThenIdempotentRepush(t *testing.T) {
	srv := setupBrandLoopServer(t)
	ctx := context.Background()
	const wsID = "ws-upsert-idem"

	payload := `{
		"name": "Acme Voice",
		"description": "How Acme sounds.",
		"tone": {"formality": "neutral", "personality": ["clear", "direct"]},
		"vocabulary": {"forbidden_terms": [{"term": "utilize", "replacement": "use"}]}
	}`

	rec, res := upsertBrandProfile(t, srv, wsID, "u-1", payload)
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "created", res.Action)
	require.NotNil(t, res.Profile)
	assert.Equal(t, 1, res.Profile.Version)
	assert.Equal(t, wsID, res.Profile.Scope)
	assert.Equal(t, "u-1", res.Profile.CreatedBy)

	rec2, res2 := upsertBrandProfile(t, srv, wsID, "u-1", payload)
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Equal(t, "unchanged", res2.Action)
	require.NotNil(t, res2.Profile)
	assert.Equal(t, res.Profile.ID, res2.Profile.ID, "re-push must match the existing profile by name")
	assert.Equal(t, 1, res2.Profile.Version, "an unchanged re-push must not bump the version")

	versions, err := srv.VoiceStore.ListProfileVersions(ctx, res.Profile.ID)
	require.NoError(t, err)
	assert.Empty(t, versions, "an unchanged re-push must not archive a snapshot")
}

// TestBrandUpsert_NewVersionPreservesServerEdits is the no-clobber path: a
// profile edited server-side (description edit + a rule promoted by the
// correction-learning loop) receives a pushed change as a NEW version — the
// edited state is archived in the version history (no field loss), and the
// promoted rule survives in the live vocabulary even though the pushed profile
// does not carry it.
func TestBrandUpsert_NewVersionPreservesServerEdits(t *testing.T) {
	srv := setupBrandLoopServer(t)
	ctx := context.Background()
	const wsID = "ws-upsert-noclobber"

	created := `{
		"name": "Acme Voice",
		"description": "Original description.",
		"tone": {"formality": "neutral"},
		"vocabulary": {"forbidden_terms": [{"term": "utilize", "replacement": "use"}]}
	}`
	rec, res := upsertBrandProfile(t, srv, wsID, "u-1", created)
	require.Equal(t, http.StatusCreated, rec.Code)
	profileID := res.Profile.ID

	// Server-side edit 1: a reviewer refines the description → v2 (v1 archived).
	live, err := srv.VoiceStore.GetProfile(ctx, profileID)
	require.NoError(t, err)
	live.Description = "Refined on the server."
	require.NoError(t, srv.VoiceStore.UpdateProfile(ctx, live))

	// Server-side edit 2: the correction loop promotes "synergy" → v3, with a
	// recorded promoted decision (mirroring HandlePromoteSuggestedRule).
	promotedProfile, changed, err := coreprofile.PromoteAndSave(ctx, srv.VoiceStore, profileID,
		coreprofile.SuggestedRule{Term: "synergy", Replacement: "teamwork", CorrectionCount: 3})
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, srv.VoiceStore.RecordRuleDecision(ctx, &coreprofile.RuleDecision{
		ProfileID: profileID, Term: "synergy", Replacement: "teamwork",
		Status: coreprofile.RuleDecisionPromoted, PromotedVersion: promotedProfile.Version,
	}))

	// The push carries a local change (new tone guideline) from a brand.yaml
	// that predates both server-side edits: no "synergy" rule, old description.
	pushed := `{
		"name": "Acme Voice",
		"description": "Original description.",
		"tone": {"formality": "neutral", "guidelines": "Lead with the benefit."},
		"vocabulary": {"forbidden_terms": [{"term": "utilize", "replacement": "use"}]}
	}`
	rec2, res2 := upsertBrandProfile(t, srv, wsID, "u-2", pushed)
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Equal(t, "updated", res2.Action)
	require.NotNil(t, res2.Profile)
	assert.Equal(t, 4, res2.Profile.Version, "the pushed change must land as a new version")

	// The pushed change is live…
	assert.Equal(t, "Lead with the benefit.", res2.Profile.Tone.Guidelines)
	// …and the promoted rule survived even though the push did not carry it.
	terms := make([]string, 0, len(res2.Profile.Vocabulary.ForbiddenTerms))
	for _, r := range res2.Profile.Vocabulary.ForbiddenTerms {
		terms = append(terms, r.Term)
	}
	assert.Contains(t, terms, "synergy", "a server-promoted rule must survive a push that predates it")
	assert.Contains(t, terms, "utilize")

	// The server-edited state is archived, not lost: v3's snapshot carries the
	// refined description.
	v3, err := srv.VoiceStore.GetProfileVersion(ctx, profileID, 3)
	require.NoError(t, err)
	assert.Equal(t, "Refined on the server.", v3.Snapshot.Description,
		"the pre-push server state must be recoverable from the version history")

	// Idempotence holds after preservation: the same stale brand.yaml re-pushed
	// compares equal to the merged live profile and no-ops.
	rec3, res3 := upsertBrandProfile(t, srv, wsID, "u-2", pushed)
	require.Equal(t, http.StatusOK, rec3.Code)
	assert.Equal(t, "unchanged", res3.Action)
	assert.Equal(t, 4, res3.Profile.Version)
}

// TestBrandUpsert_DemotedRuleStaysGone proves preservation respects a
// server-side demote: a promoted-then-demoted rule is absent from the live
// profile and a push must not resurrect it.
func TestBrandUpsert_DemotedRuleStaysGone(t *testing.T) {
	srv := setupBrandLoopServer(t)
	ctx := context.Background()
	const wsID = "ws-upsert-demote"

	rec, res := upsertBrandProfile(t, srv, wsID, "u-1", `{"name":"Acme Voice","tone":{"formality":"neutral"}}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	profileID := res.Profile.ID

	// Promote then demote "synergy" server-side; the decision row stays
	// promoted (it durably records the promotion) but the live rule is gone.
	promotedProfile, changed, err := coreprofile.PromoteAndSave(ctx, srv.VoiceStore, profileID,
		coreprofile.SuggestedRule{Term: "synergy", Replacement: "teamwork", CorrectionCount: 3})
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, srv.VoiceStore.RecordRuleDecision(ctx, &coreprofile.RuleDecision{
		ProfileID: profileID, Term: "synergy", Replacement: "teamwork",
		Status: coreprofile.RuleDecisionPromoted, PromotedVersion: promotedProfile.Version,
	}))
	_, demoted, err := coreprofile.DemoteAndSave(ctx, srv.VoiceStore, profileID, "synergy")
	require.NoError(t, err)
	require.True(t, demoted)

	rec2, res2 := upsertBrandProfile(t, srv, wsID, "u-1",
		`{"name":"Acme Voice","tone":{"formality":"formal"}}`)
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Equal(t, "updated", res2.Action)
	for _, r := range res2.Profile.Vocabulary.ForbiddenTerms {
		assert.False(t, strings.EqualFold("synergy", r.Term), "a demoted rule must not be resurrected by a push")
	}
}

// TestBrandUpsert_RequiresManageBrand proves the upsert is guarded by the same
// permission as the other brand-profile writes.
func TestBrandUpsert_RequiresManageBrand(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"X"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermViewContent)
	c.Set("workspace_id", "ws-any")
	c.Set("user_id", "u-any")
	err := srv.HandleUpsertBrandProfile(c)
	require.Error(t, err)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestBrandUpsert_NameRequired rejects a payload without a profile name — the
// name is the linkage key.
func TestBrandUpsert_NameRequired(t *testing.T) {
	srv := setupBrandLoopServer(t)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"description":"nameless"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.Set("workspace_id", "ws-nameless")
	c.Set("user_id", "u-1")
	require.NoError(t, srv.HandleUpsertBrandProfile(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
