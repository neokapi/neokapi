package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doWithBody is like do but also returns the response body, for assertions on
// what a workspace-scoped listing does (and does not) contain.
func doWithBody(t *testing.T, s *Server, method, target, token, body string) (int, string) {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.GetEcho().ServeHTTP(rec, r)
	return rec.Code, rec.Body.String()
}

// TestProviderConfigListRequiresManageConnectors proves that listing AI provider
// configurations is gated by the same PermManageConnectors permission as
// save/delete/test — a plain member cannot enumerate provider configs, while a
// caller with manage rights is not blocked by the authorization check.
func TestProviderConfigListRequiresManageConnectors(t *testing.T) {
	s, ownerToken := newTestServer(t)
	memberToken := addWorkspaceMember(t, s, "prov-mem", "prov@example.com", platauth.RoleMember)

	// Member (no manage_connectors) is forbidden from listing.
	assert.Equal(t, http.StatusForbidden,
		do(t, s, http.MethodGet, "/api/v1/test/providers", memberToken, ""),
		"a member must not enumerate provider configs")

	// Owner (manage_connectors) passes the authz check (503 when the provider
	// store is unconfigured in tests is fine — it means authz did not block).
	assert.NotEqual(t, http.StatusForbidden,
		do(t, s, http.MethodGet, "/api/v1/test/providers", ownerToken, ""),
		"an owner with manage_connectors must not be forbidden from listing")
}

// TestProviderConfigWorkspacePartition proves the workspace-scoped provider
// store is partitioned by workspace at the API boundary: one tenant cannot
// enumerate or delete another tenant's provider configs, because every handler
// derives the workspace id from the auth context and passes it to the store.
func TestProviderConfigWorkspacePartition(t *testing.T) {
	s, ownerToken := newTestServer(t) // owns "test-ws" (slug "test")
	attackerToken := addWorkspaceWithOwner(t, s, "attacker-ws", "attacker", "attacker-user", "attacker@example.com")

	// Back the /:ws/providers handlers with a real workspace-scoped provider
	// store (Epic 004). A nil cipher exercises the plaintext pass-through path.
	pgdb := pgtest.NewTestDB(t)
	_, err := bstore.NewPostgresStoreFromDB(pgdb) // baseline migrations create provider_configs
	require.NoError(t, err)
	s.ProviderStore = bstore.NewProviderConfigStore(pgdb.DB, nil)

	ctx := t.Context()

	// test-ws owner creates a provider config.
	require.Equal(t, http.StatusCreated,
		do(t, s, http.MethodPost, "/api/v1/test/providers", ownerToken,
			`{"provider_type":"openai","model":"gpt-4o","name":"test-openai"}`),
		"workspace owner must be able to save a provider config")

	// The config is stamped with — and scoped to — its owning workspace.
	ownerConfigs, err := s.ProviderStore.List(ctx, "test-ws")
	require.NoError(t, err)
	require.Len(t, ownerConfigs, 1, "the owning workspace must have exactly its own config")
	victimID := ownerConfigs[0].ID
	require.NotEmpty(t, victimID, "saved config must have an id")

	// The attacker's own listing must not reveal the other tenant's config.
	code, body := doWithBody(t, s, http.MethodGet, "/api/v1/attacker/providers", attackerToken, "")
	require.Equal(t, http.StatusOK, code)
	assert.NotContains(t, body, victimID, "attacker's provider list must not include another tenant's config")
	assert.NotContains(t, body, "test-openai", "attacker's provider list must not leak another tenant's config name")

	// The attacker cannot delete the other tenant's config by id.
	assert.Equal(t, http.StatusNotFound,
		do(t, s, http.MethodDelete, "/api/v1/attacker/providers/"+victimID, attackerToken, ""),
		"cross-tenant provider delete must be denied (404)")

	// The config survives the cross-tenant delete attempt.
	_, err = s.ProviderStore.Get(ctx, "test-ws", victimID)
	require.NoError(t, err, "victim provider config must survive a cross-tenant delete attempt")

	// The rightful owner still sees and can delete its own config.
	code, body = doWithBody(t, s, http.MethodGet, "/api/v1/test/providers", ownerToken, "")
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, victimID, "the owning workspace must see its own config")
	assert.Equal(t, http.StatusNoContent,
		do(t, s, http.MethodDelete, "/api/v1/test/providers/"+victimID, ownerToken, ""),
		"the owning workspace must be able to delete its own config")
}
