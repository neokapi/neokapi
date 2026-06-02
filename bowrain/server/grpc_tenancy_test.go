package server

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// tenancyAuthStore is a minimal AuthStore that only answers GetMembership. It
// embeds the interface so it satisfies auth.AuthStore; the tenancy tests below
// exercise only GetMembership (any other method would panic on the nil embed,
// which is the point — the NeokapiService scoping path must touch nothing else).
type tenancyAuthStore struct {
	auth.AuthStore
	members map[string]bool // "<workspaceID>/<userID>" -> member?
}

func (s *tenancyAuthStore) GetMembership(_ context.Context, workspaceID, userID string) (*platauth.Membership, error) {
	if s.members[workspaceID+"/"+userID] {
		return &platauth.Membership{WorkspaceID: workspaceID, UserID: userID, Role: platauth.RoleMember}, nil
	}
	return nil, errors.New("not a member")
}

func ctxWithGRPCUser(userID string) context.Context {
	return context.WithValue(context.Background(), grpcUserKey{}, &platauth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: userID},
	})
}

func mustCreateProject(t *testing.T, srv *Server, name, workspaceID string) *store.Project {
	t.Helper()
	p := &store.Project{Name: name, WorkspaceID: workspaceID, DefaultSourceLanguage: model.LocaleID("en")}
	require.NoError(t, srv.Services.Project.CreateProject(context.Background(), p))
	require.NotEmpty(t, p.ID)
	return p
}

// TestGRPCProjectTenancyScoping verifies that NeokapiService project RPCs are
// scoped to the caller's workspace memberships once an AuthStore is present, so
// one tenant cannot enumerate, read, or mutate another tenant's projects.
func TestGRPCProjectTenancyScoping(t *testing.T) {
	srv := NewServer(DefaultConfig())
	initTestStores(t, srv)
	srv.AuthStore = &tenancyAuthStore{members: map[string]bool{"ws-a/user-a": true}}

	g := NewGRPCServer(srv)
	projA := mustCreateProject(t, srv, "Project A", "ws-a")
	projB := mustCreateProject(t, srv, "Project B", "ws-b")

	userA := ctxWithGRPCUser("user-a")

	// ListProjects returns only the caller's workspace projects.
	list, err := g.ListProjects(userA, &pb.ListProjectsRequest{})
	require.NoError(t, err)
	require.Len(t, list.Projects, 1)
	assert.Equal(t, projA.ID, list.Projects[0].Id)

	// GetProject on the caller's own workspace succeeds.
	got, err := g.GetProject(userA, &pb.GetProjectRequest{ProjectId: projA.ID})
	require.NoError(t, err)
	assert.Equal(t, projA.ID, got.Id)

	// GetProject on another tenant's project is NotFound (not PermissionDenied),
	// so it cannot be used as a cross-tenant existence oracle.
	_, err = g.GetProject(userA, &pb.GetProjectRequest{ProjectId: projB.ID})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))

	// A cross-tenant mutation path (CreateVersion) is blocked too.
	_, err = g.CreateVersion(userA, &pb.CreateVersionRequest{ProjectId: projB.ID, Label: "x"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))

	// A caller with no membership at all sees nothing and cannot read.
	userNobody := ctxWithGRPCUser("user-z")
	list, err = g.ListProjects(userNobody, &pb.ListProjectsRequest{})
	require.NoError(t, err)
	assert.Empty(t, list.Projects)
	_, err = g.GetProject(userNobody, &pb.GetProjectRequest{ProjectId: projA.ID})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// TestGRPCProjectTenancyStandalone verifies the standalone/single-user bypass:
// with no AuthStore configured, every project is visible (no workspace scoping).
func TestGRPCProjectTenancyStandalone(t *testing.T) {
	srv := NewServer(DefaultConfig())
	initTestStores(t, srv)
	require.Nil(t, srv.AuthStore)

	g := NewGRPCServer(srv)
	mustCreateProject(t, srv, "Project A", "ws-a")
	mustCreateProject(t, srv, "Project B", "ws-b")

	list, err := g.ListProjects(context.Background(), &pb.ListProjectsRequest{})
	require.NoError(t, err)
	assert.Len(t, list.Projects, 2)
}
