package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAuthStore answers only the two lookups WorkspaceAccessMiddleware makes.
// The interface is embedded rather than implemented: any other method reaching
// this store is a test that has drifted from what the middleware does, and a
// nil-interface panic says so louder than a zero value would.
type stubAuthStore struct {
	auth.AuthStore
	workspace    *platauth.Workspace
	workspaceErr error
	membership   *platauth.Membership
	membershipRr error
}

func (s *stubAuthStore) GetWorkspaceBySlug(context.Context, string) (*platauth.Workspace, error) {
	return s.workspace, s.workspaceErr
}

func (s *stubAuthStore) GetMembership(context.Context, string, string) (*platauth.Membership, error) {
	return s.membership, s.membershipRr
}

// TestWorkspaceAccessDistinguishesAbsentFromUnreachable holds the middleware to
// the difference between a workspace that is not there and one it could not
// look up. Collapsing them told a client whose database was briefly locked that
// its workspace did not exist — a permanent-sounding answer to a transient
// condition, and one that gives the client no reason to retry.
func TestWorkspaceAccessDistinguishesAbsentFromUnreachable(t *testing.T) {
	ws := &platauth.Workspace{ID: "ws-1", Slug: "acme"}

	tests := []struct {
		name       string
		store      *stubAuthStore
		wantStatus int
		wantCode   string
	}{
		{
			name:       "workspace genuinely absent is 404",
			store:      &stubAuthStore{workspaceErr: sql.ErrNoRows},
			wantStatus: http.StatusNotFound,
			wantCode:   "workspace not found",
		},
		{
			name:       "workspace lookup failed is 503, not 404",
			store:      &stubAuthStore{workspaceErr: errors.New("dial tcp: connection refused")},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   codeWorkspaceLookupFailed,
		},
		{
			name:       "wrapped no-rows is still 404",
			store:      &stubAuthStore{workspaceErr: fmtWrap(sql.ErrNoRows)},
			wantStatus: http.StatusNotFound,
			wantCode:   "workspace not found",
		},
		{
			name:       "genuine non-member is 403",
			store:      &stubAuthStore{workspace: ws, membershipRr: sql.ErrNoRows},
			wantStatus: http.StatusForbidden,
			wantCode:   "not a member of this workspace",
		},
		{
			name:       "membership lookup failed is 503, not 403",
			store:      &stubAuthStore{workspace: ws, membershipRr: errors.New("context deadline exceeded")},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   codeWorkspaceLookupFailed,
		},
		{
			name: "member passes through",
			store: &stubAuthStore{
				workspace:  ws,
				membership: &platauth.Membership{WorkspaceID: "ws-1", UserID: "u-1", Role: platauth.RoleOwner},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/acme/things", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("ws")
			c.SetParamValues("acme")
			c.Set("user_id", "u-1")

			h := WorkspaceAccessMiddleware(tt.store)(func(echo.Context) error {
				return c.NoContent(http.StatusOK)
			})
			require.NoError(t, h(c))
			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantCode != "" {
				assert.Contains(t, rec.Body.String(), tt.wantCode)
			}
		})
	}
}

// fmtWrap wraps err the way the store's scan helpers do, so the test proves the
// middleware unwraps rather than compares.
func fmtWrap(err error) error {
	return errors.Join(errors.New("scan workspace"), err)
}
