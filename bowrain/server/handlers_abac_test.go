package server

import (
	"net/http"
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
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

	// Draft: a member (translate) can edit.
	require.Less(t, edit(memberToken, "v1"), 300)

	// Owner publishes the block.
	require.Equal(t, http.StatusOK, setStatus(ownerToken, "published"))

	// Member can no longer edit published content (needs manage_project)...
	assert.Equal(t, http.StatusForbidden, edit(memberToken, "v2"))
	// ...but the owner (manage) still can.
	assert.Less(t, edit(ownerToken, "v2-owner"), 300)

	// In-review: a member without review cannot edit.
	require.Equal(t, http.StatusOK, setStatus(ownerToken, "in_review"))
	assert.Equal(t, http.StatusForbidden, edit(memberToken, "v3"))
	// The owner (review) can.
	assert.Less(t, edit(ownerToken, "v3-owner"), 300)

	// A member cannot change workflow status (needs review).
	assert.Equal(t, http.StatusForbidden, setStatus(memberToken, "draft"))
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
