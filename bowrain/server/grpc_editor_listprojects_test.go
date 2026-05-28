package server

import (
	"context"
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression: the gRPC ListEditorProjects (used by the desktop app's dashboard)
// filtered projects by comparing the stored workspace UUID (project.WorkspaceID)
// against the request's workspace SLUG. A UUID never equals a slug, so every
// project was dropped and the desktop showed an empty dashboard even though the
// same projects listed fine over REST. The slug must be resolved to the
// workspace ID first, exactly as the REST list handler does via middleware.
func TestListEditorProjects_ResolvesWorkspaceSlugToID(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-list-editor-projects" // wire the AuthStore for slug resolution
	srv := NewServer(cfg)
	initTestStores(t, srv)
	require.NotNil(t, srv.AuthStore, "AuthStore must be wired for slug resolution")

	ctx := context.Background()

	ws := &platauth.Workspace{Name: "Acme CloudOps", Slug: "acme-list-test"}
	require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, ws))
	require.NotEmpty(t, ws.ID)
	require.NotEqual(t, ws.Slug, ws.ID, "slug and id must differ to exercise the bug")

	// Create a project the way the REST create path does: WorkspaceID = the
	// workspace UUID, plus the default collection + main stream.
	info, err := editorCreateProject(ctx, srv.ContentStore, ws.ID, "Company Website", "en", []string{"fr", "de"})
	require.NoError(t, err)

	g := NewEditorGRPCServer(srv)

	// Listing by SLUG must return the project stored under the workspace UUID.
	resp, err := g.ListEditorProjects(ctx, &pb.ListEditorProjectsRequest{WorkspaceSlug: ws.Slug})
	require.NoError(t, err)
	require.Len(t, resp.Projects, 1, "listing by slug must return the project stored under the workspace UUID")
	assert.Equal(t, info.ID, resp.Projects[0].Id)
	assert.Equal(t, "Company Website", resp.Projects[0].Name)

	// A different workspace's slug must not see this project.
	other := &platauth.Workspace{Name: "Other", Slug: "other-ws"}
	require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, other))
	resp2, err := g.ListEditorProjects(ctx, &pb.ListEditorProjectsRequest{WorkspaceSlug: other.Slug})
	require.NoError(t, err)
	assert.Empty(t, resp2.Projects, "a different workspace must not list this project")
}
