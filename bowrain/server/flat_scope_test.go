package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/neokapi/neokapi/core/model"
)

// The flat sync routes narrow an API token to its scopes, as the workspace
// routes do: a token scoped to reading cannot start a push its owner could.
func TestFlatSyncRoutes_EnforceTokenScopes(t *testing.T) {
	srv, _ := newTestServer(t)
	ctx := t.Context()
	p := &platstore.Project{
		Name: "flat", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"},
		WorkspaceID: "test-ws", Properties: map[string]string{},
	}
	require.NoError(t, srv.ContentStore.CreateProject(ctx, p))

	mint := func(scope string) string {
		raw, err := json.Marshal([]string{scope})
		require.NoError(t, err)
		_, plaintext, err := srv.Services.Auth.CreateAPIToken(ctx, service.APITokenSpec{
			UserID: "test-user", WorkspaceID: "test-ws", Name: scope, ScopesJSON: string(raw),
		})
		require.NoError(t, err)
		return plaintext
	}

	tests := []struct {
		name    string
		scope   string
		refused bool
	}{
		{"a read-scoped token is refused a push", "read", true},
		{"a contribute-scoped token may push", "contribute", false},
		{"a full-access token may push", "*", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, srv, http.MethodPost, "/api/v1/projects/"+p.ID+"/sync/main/push/init", mint(tt.scope), map[string]any{})
			if tt.refused {
				assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
				return
			}
			assert.NotEqual(t, http.StatusForbidden, rec.Code, rec.Body.String())
			assert.NotEqual(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		})
	}

	t.Run("a read-scoped token still reads", func(t *testing.T) {
		rec := serve(t, srv, http.MethodGet, "/api/v1/projects/"+p.ID+"/sync/main/ref", mint("read"), nil)
		assert.NotEqual(t, http.StatusForbidden, rec.Code, rec.Body.String())
	})
}
