package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApproveContextScan(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody ContextScanApproval
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"profile":{"id":"p1","name":"Acme Voice"},"profile_action":"created",
			"concepts_created":2,"concepts_existing":0,"concept_ids":["c1","c2"]}`))
	}))
	t.Cleanup(srv.Close)
	c := NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")

	res, err := c.ApproveContextScan(context.Background(), "scan-1", ContextScanApproval{
		Profile: BrandProfileUpsert{Name: "Acme Voice"},
		Locale:  "en-US",
		Terms:   []ContextScanApprovedTerm{{Term: "sign in", Domain: "auth"}, {Term: "cart"}},
	})
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1/acme/brand-scans/scan-1/approve", gotPath)
	assert.Equal(t, "Acme Voice", gotBody.Profile.Name)
	assert.Equal(t, "en-US", gotBody.Locale)
	require.Len(t, gotBody.Terms, 2)
	assert.Equal(t, "sign in", gotBody.Terms[0].Term)

	assert.Equal(t, "created", res.ProfileAction)
	assert.Equal(t, 2, res.ConceptsCreated)
	assert.Equal(t, []string{"c1", "c2"}, res.ConceptIDs)
}

func TestApproveContextScanForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	c := NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")

	_, err := c.ApproveContextScan(context.Background(), "scan-1", ContextScanApproval{})
	require.ErrorIs(t, err, ErrForbidden)
}

func TestApproveContextScanRequiresWorkspace(t *testing.T) {
	c := NewProjectBearerClient("http://example.invalid", "proj1", "tok")
	_, err := c.ApproveContextScan(context.Background(), "scan-1", ContextScanApproval{})
	require.Error(t, err)
}
