package server

import (
	"net/http"
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPhase4_CompactArchives proves change-log compaction moves trimmed rows to
// change_log_archive instead of deleting them.
func TestPhase4_CompactArchives(t *testing.T) {
	s, _ := newTestServer(t)
	pg := s.ContentStore.(*bstore.PostgresStore)
	cs := s.ContentStore
	fr := model.LocaleID("fr")
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-comp", Name: "Comp", DefaultSourceLanguage: "en"}))

	blk := &model.Block{ID: "bc", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "x"}}}}
	for _, v := range []string{"a", "b", "c"} {
		blk.SetTargetText(fr, v)
		require.NoError(t, cs.StoreBlocks(ctx, "p-comp", "main", []*model.Block{blk}))
	}

	n, err := pg.CompactChangeLog(ctx, "p-comp", "main", 0)
	require.NoError(t, err)
	require.Greater(t, n, int64(0), "some change_log rows should be compacted")

	var archived int64
	require.NoError(t, pg.SQLDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM change_log_archive WHERE project_id = $1`, "p-comp").Scan(&archived))
	assert.Equal(t, n, archived, "compacted rows must be archived, not deleted")
}

// TestPhase4_RoleEscalationBlocked proves an admin cannot mint a role granting
// permissions beyond their own (after a workspace override limits the admin role).
func TestPhase4_RoleEscalationBlocked(t *testing.T) {
	s, ownerToken := newTestServer(t)
	adminToken := addWorkspaceMember(t, s, "esc-admin", "esc@example.com", platauth.RoleAdmin)

	// Limit the admin role to view-only in this workspace.
	require.NoError(t, s.AuthStore.SetWorkspaceRoleOverride(t.Context(), "test-ws", platauth.RoleAdmin, platauth.PermViewContent))

	// Admin cannot create a role granting manage_project (beyond their own perms).
	assert.Equal(t, http.StatusForbidden,
		do(t, s, http.MethodPost, "/api/v1/test/roles", adminToken, `{"name":"sneaky","permissions":["view_content","manage_project"]}`))

	// Admin can create a role within their own perms.
	assert.Equal(t, http.StatusCreated,
		do(t, s, http.MethodPost, "/api/v1/test/roles", adminToken, `{"name":"viewer2","permissions":["view_content"]}`))

	// Owner (full perms) can create anything.
	assert.Equal(t, http.StatusCreated,
		do(t, s, http.MethodPost, "/api/v1/test/roles", ownerToken, `{"name":"poweruser","permissions":["manage_project"]}`))
}
