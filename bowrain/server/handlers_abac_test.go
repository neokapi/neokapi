package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// postChanges sends ops to a project's main stream through the router, as the
// token's holder in workspace "test", and returns the status and the result.
func postChanges(t *testing.T, s *Server, token, pid string, ops ...change.Op) (int, change.Result) {
	t.Helper()
	body, err := json.Marshal(change.Set{Ops: ops})
	require.NoError(t, err)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/test/projects/"+pid+"/streams/main/changes", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.GetEcho().ServeHTTP(rec, r)
	var res change.Result
	if rec.Code < 500 {
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
	}
	return rec.Code, res
}

// storeItemBlock stores blk under item in a project's main stream and returns
// the row id the store gave it.
func storeItemBlock(t *testing.T, cs platstore.ContentStore, pid, item string, blk *model.Block) string {
	t.Helper()
	ctx := t.Context()
	key := blk.ID
	require.NoError(t, cs.StoreItem(ctx, pid, "main", &platstore.Item{Name: item, Format: "txt", ItemType: "file"}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, pid, "main", item, []*model.Block{blk}))
	stored, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: pid, Stream: "main", ItemName: item})
	require.NoError(t, err)
	for _, sb := range stored {
		if sb.SourceID == key {
			return sb.Block.ID
		}
	}
	t.Fatalf("block %s not stored", key)
	return ""
}

// translateOp sets a block's translation, guarded by the revision the stream
// holds.
func translateOp(t *testing.T, cs platstore.ContentStore, pid, item, bid, locale, text string) change.Op {
	t.Helper()
	sb, err := cs.GetBlock(t.Context(), pid, "main", bid)
	require.NoError(t, err)
	rev := platstore.TargetRevision(sb, model.LocaleID(locale))
	return setText(at(item, bid, locale), rev, text)
}

// Edits are gated by a block's access state: anyone with translate edits an
// open block, a published one takes manage, and a restricted one takes review
// for the language.
func TestPhase4_ABACStatusGating(t *testing.T) {
	s, ownerToken := newTestServer(t)
	memberToken := addWorkspaceMember(t, s, "abac-mem", "abac@example.com", platauth.RoleMember)
	cs := s.ContentStore
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-abac", Name: "ABAC", DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr"}, WorkspaceID: "test-ws"}))
	bid := storeItemBlock(t, cs, "p-abac", "hi.txt", model.NewBlock("ba", "hi"))
	as, ok := cs.(platstore.BlockAccessStore)
	require.True(t, ok)

	edit := func(token, text string) int {
		code, _ := postChanges(t, s, token, "p-abac", translateOp(t, cs, "p-abac", "hi.txt", bid, "fr", text))
		return code
	}

	// Open: a member (translate) can edit.
	require.Equal(t, http.StatusOK, edit(memberToken, "v1"))

	// Published: a member can no longer edit; the owner (manage) can.
	require.NoError(t, as.SetBlockAccess(ctx, "p-abac", "main", bid, bstore.BlockAccessPublished, ""))
	assert.Equal(t, http.StatusForbidden, edit(memberToken, "v2"))
	assert.Equal(t, http.StatusOK, edit(ownerToken, "v2-owner"))

	// Restricted: a member without review cannot edit; the owner (review) can.
	require.NoError(t, as.SetBlockAccess(ctx, "p-abac", "main", bid, bstore.BlockAccessRestricted, ""))
	assert.Equal(t, http.StatusForbidden, edit(memberToken, "v3"))
	assert.Equal(t, http.StatusOK, edit(ownerToken, "v3-owner"))

	// The block's owner keeps working on content held for them.
	require.NoError(t, as.SetBlockAccess(ctx, "p-abac", "main", bid, bstore.BlockAccessRestricted, "abac-mem"))
	assert.Equal(t, http.StatusOK, edit(memberToken, "v4"))
}

// The separation-of-duties gate on establishing a translation asks who wrote
// the translation, per language, rather than reading the newest attributed row
// of the block's history. The decision ledger and a settled projection write
// into the same author column, so the block-global reading named the last
// decider (or "system") as the translator.
func TestEstablishSoDReadsTheTargetAuthor(t *testing.T) {
	s, ownerToken := newTestServer(t)
	cs := s.ContentStore
	ctx := t.Context()
	require.NoError(t, s.AuthStore.SetSoDMode(ctx, "test-ws", platauth.SoDBlock))
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID: "p-pub", Name: "Establish SoD", DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr", "de"}, WorkspaceID: "test-ws",
	}))
	newBlock := func(id string) string {
		t.Helper()
		return storeItemBlock(t, cs, "p-pub", "greetings.txt", model.NewBlock(id, "hi "+id))
	}
	translatorToken := addWorkspaceMember(t, s, "pub-tr", "pub-tr@example.com", platauth.RoleMember)
	reviewerToken := addWorkspaceMember(t, s, "pub-rev", "pub-rev@example.com", platauth.RoleAdmin)

	write := func(token, bid, locale, text string) {
		t.Helper()
		code, res := postChanges(t, s, token, "p-pub", translateOp(t, cs, "p-pub", "greetings.txt", bid, locale, text))
		require.Equal(t, http.StatusOK, code, "%+v", res)
	}
	establish := func(token, bid, locale string) int {
		t.Helper()
		sb, err := cs.GetBlock(ctx, "p-pub", "main", bid)
		require.NoError(t, err)
		code, _ := postChanges(t, s, token, "p-pub", decide(at("greetings.txt", bid, locale),
			platstore.TargetRevision(sb, model.LocaleID(locale)), change.OutcomeEstablish))
		return code
	}

	t.Run("the author of the locale being established is refused", func(t *testing.T) {
		bid := newBlock("b-own")
		write(ownerToken, bid, "fr", "bonjour")
		assert.Equal(t, http.StatusForbidden, establish(ownerToken, bid, "fr"))
	})

	t.Run("a later decision by somebody else keeps the author refused", func(t *testing.T) {
		bid := newBlock("b-dec")
		write(ownerToken, bid, "fr", "bonjour")
		require.Equal(t, http.StatusOK, establish(reviewerToken, bid, "fr"))
		write(reviewerToken, bid, "fr", "bonjour !")
		write(ownerToken, bid, "fr", "bonjour")
		assert.Equal(t, http.StatusForbidden, establish(ownerToken, bid, "fr"))
	})

	t.Run("recording a decision after somebody else's edit is not authorship", func(t *testing.T) {
		bid := newBlock("b-mine")
		write(translatorToken, bid, "fr", "bonjour")
		recordDecision(t, cs, "p-pub", bid, "fr", "test-user")
		assert.Equal(t, http.StatusOK, establish(ownerToken, bid, "fr"))
	})

	t.Run("authoring one language does not hold up another", func(t *testing.T) {
		bid := newBlock("b-two")
		write(ownerToken, bid, "fr", "bonjour")
		write(translatorToken, bid, "de", "guten tag")
		assert.Equal(t, http.StatusForbidden, establish(ownerToken, bid, "fr"))
		assert.Equal(t, http.StatusOK, establish(ownerToken, bid, "de"))
	})

	t.Run("a translation nobody wrote is establishable", func(t *testing.T) {
		blk := model.NewBlock("b-machine", "hi machine")
		blk.SetTargetText("fr", "bonjour")
		bid := storeItemBlock(t, cs, "p-pub", "greetings.txt", blk)
		assert.Equal(t, http.StatusOK, establish(ownerToken, bid, "fr"))
	})
}

// recordDecision files a ledger decision on one target, naming decider as the
// person who made it. The ledger writes it into the same block_history column a
// translation is attributed in, which is what the separation-of-duties gate
// must look past.
func recordDecision(t *testing.T, cs platstore.ContentStore, projectID, blockID, locale, decider string) {
	t.Helper()
	ds, ok := cs.(platstore.DecisionStore)
	require.True(t, ok, "the test content store must keep the decision ledger")
	sb, err := cs.GetBlock(t.Context(), projectID, "main", blockID)
	require.NoError(t, err)
	require.NotNil(t, sb)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = ds.UpsertUnitDecisions(t.Context(), projectID, "main", []venue.UnitDecision{{
		ItemName:    sb.ItemName,
		Unit:        sb.SourceID,
		Variant:     locale,
		Status:      string(model.TargetStatusEstablished),
		ReviewState: "approved",
		DecidedBy:   decider,
		DecidedAt:   now,
		Updated:     now,
	}})
	require.NoError(t, err)
}
