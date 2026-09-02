package server

import (
	"net/http"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPhase4_ABACStatusGating proves edits are gated by a block's workflow
// status: anyone with translate can edit a draft, but editing published content
// requires manage, and editing in-review content requires review.
func TestPhase4_ABACStatusGating(t *testing.T) {
	s, ownerToken := newTestServer(t)
	memberToken := addWorkspaceMember(t, s, "abac-mem", "abac@example.com", platauth.RoleMember)
	cs := s.ContentStore
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-abac", Name: "ABAC", DefaultSourceLanguage: "en", WorkspaceID: "test-ws"}))
	blk := &model.Block{ID: "ba", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "hi"}}}}
	require.NoError(t, cs.StoreBlocks(ctx, "p-abac", "main", []*model.Block{blk}))

	edit := func(token, text string) int {
		return do(t, s, http.MethodPut, "/api/v1/test/p-abac/blocks/main/ba", token, `{"target_locale":"fr","text":"`+text+`"}`)
	}
	setStatus := func(token, status string) int {
		return do(t, s, http.MethodPut, "/api/v1/test/p-abac/blocks/main/ba/status", token, `{"status":"`+status+`"}`)
	}

	// Open: a member (translate) can edit.
	require.Less(t, edit(memberToken, "v1"), 300)

	// Owner publishes the block.
	require.Equal(t, http.StatusOK, setStatus(ownerToken, "published"))

	// Member can no longer edit published content (needs manage_project)...
	assert.Equal(t, http.StatusForbidden, edit(memberToken, "v2"))
	// ...but the owner (manage) still can.
	assert.Less(t, edit(ownerToken, "v2-owner"), 300)

	// Restricted: a member without review cannot edit.
	require.Equal(t, http.StatusOK, setStatus(ownerToken, "restricted"))
	assert.Equal(t, http.StatusForbidden, edit(memberToken, "v3"))
	// The owner (review) can.
	assert.Less(t, edit(ownerToken, "v3-owner"), 300)

	// A member cannot change access state (needs review).
	assert.Equal(t, http.StatusForbidden, setStatus(memberToken, "open"))

	// The retired vocabulary is normalized, not rejected: "in_review" still
	// lands as restricted for a caller that has not caught up.
	require.Equal(t, http.StatusOK, setStatus(ownerToken, "in_review"))
}

// TestPhase4_SoDBlocksSelfApproval proves separation of duties (block mode)
// prevents the translator from approving (publishing) their own work, while a
// different reviewer can.
func TestPhase4_SoDBlocksSelfApproval(t *testing.T) {
	s, ownerToken := newTestServer(t)
	cs := s.ContentStore
	ctx := t.Context()
	require.NoError(t, s.AuthStore.SetSoDMode(ctx, "test-ws", platauth.SoDBlock))
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-sod", Name: "SoD", DefaultSourceLanguage: "en", WorkspaceID: "test-ws"}))
	blk := &model.Block{ID: "bs", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "hi"}}}}
	require.NoError(t, cs.StoreBlocks(ctx, "p-sod", "main", []*model.Block{blk}))

	// The owner translates the block (becomes its last editor).
	require.Less(t, do(t, s, http.MethodPut, "/api/v1/test/p-sod/blocks/main/bs", ownerToken, `{"target_locale":"fr","text":"v1"}`), 300)

	// The owner cannot approve (publish) their own translation under SoD block mode.
	assert.Equal(t, http.StatusForbidden,
		do(t, s, http.MethodPut, "/api/v1/test/p-sod/blocks/main/bs/status", ownerToken, `{"status":"published"}`))

	// A different reviewer can publish it.
	reviewerToken := addWorkspaceMember(t, s, "sod-rev", "sod-rev@example.com", platauth.RoleAdmin)
	assert.Equal(t, http.StatusOK,
		do(t, s, http.MethodPut, "/api/v1/test/p-sod/blocks/main/bs/status", reviewerToken, `{"status":"published"}`))
}

// TestPublishSoDReadsTheTargetAuthor proves the four-eyes gate on publishing
// asks who wrote the translation, per language, rather than reading the newest
// attributed row of the block's history. The decision ledger and a settled
// projection write into the same author column, so the block-global reading
// named the last decider (or "system") as the translator.
func TestPublishSoDReadsTheTargetAuthor(t *testing.T) {
	s, ownerToken := newTestServer(t)
	cs := s.ContentStore
	ctx := t.Context()
	require.NoError(t, s.AuthStore.SetSoDMode(ctx, "test-ws", platauth.SoDBlock))
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID: "p-pub", Name: "Publish SoD", DefaultSourceLanguage: "en", WorkspaceID: "test-ws",
	}))
	require.NoError(t, cs.StoreItem(ctx, "p-pub", "main", &platstore.Item{
		Name: "greetings.txt", Format: "txt", ItemType: "file",
	}))
	// Storing under an item mints the block a project-unique id and keeps the
	// caller's as its source id, so the address the routes take comes back from
	// the store rather than from the literal above.
	storeBlock := func(blk *model.Block) string {
		t.Helper()
		sourceID := blk.ID
		require.NoError(t, cs.StoreBlocksForItem(ctx, "p-pub", "main", "greetings.txt", []*model.Block{blk}))
		stored, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: "p-pub", Stream: "main", ItemName: "greetings.txt"})
		require.NoError(t, err)
		for _, sb := range stored {
			if sb.SourceID == sourceID {
				return sb.Block.ID
			}
		}
		t.Fatalf("block %s not stored", sourceID)
		return ""
	}
	newBlock := func(id string) string {
		t.Helper()
		return storeBlock(&model.Block{ID: id, Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "hi " + id}}}})
	}
	// The translator holds translate and no review, so every publish below is
	// the owner's or the reviewer's, and what varies is who wrote the wording.
	translatorToken := addWorkspaceMember(t, s, "pub-tr", "pub-tr@example.com", platauth.RoleMember)
	reviewerToken := addWorkspaceMember(t, s, "pub-rev", "pub-rev@example.com", platauth.RoleAdmin)

	write := func(token, blockID, locale, text string) {
		t.Helper()
		require.Less(t, do(t, s, http.MethodPut, "/api/v1/test/p-pub/blocks/main/"+blockID, token,
			`{"target_locale":"`+locale+`","text":"`+text+`"}`), 300)
	}
	publish := func(token, blockID, body string) int {
		return do(t, s, http.MethodPut, "/api/v1/test/p-pub/blocks/main/"+blockID+"/status", token, body)
	}

	t.Run("the author of the locale being published is refused", func(t *testing.T) {
		bid := newBlock("b-own")
		write(ownerToken, bid, "fr", "bonjour")
		assert.Equal(t, http.StatusForbidden, publish(ownerToken, bid, `{"status":"published","locale":"fr"}`))
	})

	t.Run("a later decision by somebody else keeps the author refused", func(t *testing.T) {
		bid := newBlock("b-dec")
		write(ownerToken, bid, "fr", "bonjour")
		// The reviewer approves the target, which files a decision row against
		// the block carrying the reviewer's identity. Reading the newest
		// attributed row names the reviewer and lets the owner publish wording
		// the owner wrote.
		require.Equal(t, http.StatusOK, do(t, s, http.MethodPut, "/api/v1/test/p-pub/blocks/main/"+bid+"/review",
			reviewerToken, `{"target_locale":"fr","reviewed":true}`))
		assert.Equal(t, http.StatusForbidden, publish(ownerToken, bid, `{"status":"published","locale":"fr"}`))
	})

	t.Run("recording a decision after somebody else's edit is not authorship", func(t *testing.T) {
		bid := newBlock("b-mine")
		write(translatorToken, bid, "fr", "bonjour")
		recordDecision(t, cs, "p-pub", bid, "fr", "test-user")
		assert.Equal(t, http.StatusOK, publish(ownerToken, bid, `{"status":"published","locale":"fr"}`))
	})

	t.Run("authoring one language does not hold up another", func(t *testing.T) {
		bid := newBlock("b-two")
		write(ownerToken, bid, "fr", "bonjour")
		write(translatorToken, bid, "de", "guten tag")
		assert.Equal(t, http.StatusForbidden, publish(ownerToken, bid, `{"status":"published","locale":"fr"}`))
		assert.Equal(t, http.StatusOK, publish(ownerToken, bid, `{"status":"published","locale":"de"}`))
	})

	t.Run("a target nobody wrote is publishable", func(t *testing.T) {
		blk := &model.Block{ID: "b-machine", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "hi machine"}}}}
		blk.SetTargetText("fr", "bonjour")
		bid := storeBlock(blk)
		assert.Equal(t, http.StatusOK, publish(ownerToken, bid, `{"status":"published","locale":"fr"}`))
	})

	t.Run("a block the project does not hold is still a 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, publish(ownerToken, "b-absent", `{"status":"published"}`))
	})

	t.Run("naming no locale judges every language the block holds", func(t *testing.T) {
		bid := newBlock("b-all")
		write(translatorToken, bid, "fr", "bonjour")
		write(ownerToken, bid, "de", "guten tag")
		assert.Equal(t, http.StatusForbidden, publish(ownerToken, bid, `{"status":"published"}`))
		assert.Equal(t, http.StatusOK, publish(reviewerToken, bid, `{"status":"published"}`))
	})
}

// recordDecision files a ledger decision on one target, naming decider as the
// person who made it. The ledger writes it into the same block_history column a
// translation is attributed in, which is what the publish gate must look past.
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
		Status:      string(model.TargetStatusReviewed),
		ReviewState: "approved",
		DecidedBy:   decider,
		DecidedAt:   now,
		Updated:     now,
	}})
	require.NoError(t, err)
}
