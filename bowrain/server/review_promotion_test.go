package server

import (
	"encoding/json"
	"net/http"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewBlock_PromotesApprovedWordingIntoMemory pins the editor half of
// "the corpus follows decisions": establishing a translation through a
// stream's changes route must land the (source, target) pair in the workspace
// content memory, and rejecting it must evict it. The full chain runs —
// router → change service → ledger → PromoteDecisionsToMemory — because each
// hop has already failed silently once (the wrapped-store capability, then a
// skip with no log).
func TestReviewBlock_PromotesApprovedWordingIntoMemory(t *testing.T) {
	s, ownerToken := newTestServer(t)
	cs := s.ContentStore
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID: "p-promote", Name: "Promote", DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"nb"}, WorkspaceID: "test-ws",
	}))

	blk := &model.Block{ID: "greeting", Translatable: true}
	blk.SetSourceText("Hello")
	blk.SetTargetText("nb", "Hei")
	require.NoError(t, cs.StoreItem(ctx, "p-promote", "main", &platstore.Item{Name: "en.json", Format: "json"}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p-promote", "main", "en.json", []*model.Block{blk}))

	// Resolve the stored row id the editor addresses blocks by.
	rows, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: "p-promote", Stream: "main", ItemName: "en.json", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	bid := rows[0].ID

	// The workspace SLUG routes the memory ("test"), not the workspace id.
	tm, err := s.wsStores.getMemory("test")
	require.NoError(t, err)
	before, err := tm.Count(ctx)
	require.NoError(t, err)

	// The decision is sent to the stream's changes route through the router,
	// on the revision of the translation the reviewer read.
	decideVia := func(outcome change.Outcome) int {
		t.Helper()
		sb, err := cs.GetBlock(ctx, "p-promote", "main", bid)
		require.NoError(t, err)
		body, err := json.Marshal(change.Set{Ops: []change.Op{
			decide(at("en.json", bid, "nb"), platstore.TargetRevision(sb, "nb"), outcome)}})
		require.NoError(t, err)
		return do(t, s, http.MethodPost, "/api/v1/test/projects/p-promote/streams/main/changes", ownerToken, string(body))
	}
	code := decideVia(change.OutcomeEstablish)
	require.Equal(t, http.StatusOK, code)

	after, err := tm.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, before+1, after, "an approval must promote its wording into the workspace memory")

	// The ledger carries the decision the promotion followed.
	ds, ok := s.ContentStore.(platstore.DecisionStore)
	require.True(t, ok, "the (possibly wrapped) content store must forward the decision ledger")
	decisions, err := ds.ListUnitDecisions(ctx, "p-promote", "main")
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "approved", decisions[0].ReviewState)

	// Rejection evicts.
	code = decideVia(change.OutcomeReject)
	require.Equal(t, http.StatusOK, code)
	final, err := tm.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, final, "a rejection must evict the wording it condemns")
}
