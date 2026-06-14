package backend

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKnowledgeNotConnected verifies the knowledge proxies surface
// errNotConnected when there is no active server connection, exactly like the
// governance proxies.
func TestKnowledgeNotConnected(t *testing.T) {
	app := newTestApp(t)

	_, err := app.ListConcepts("acme", ListConceptsArgs{})
	require.ErrorIs(t, err, errNotConnected)

	_, err = app.ListChangesets("acme", "")
	require.ErrorIs(t, err, errNotConnected)

	err = app.RemovePilot("acme", "cs-1", "proj-1", "main")
	require.ErrorIs(t, err, errNotConnected)
}

// TestKnowledgeConceptRoutes covers the concept + relation/observation/comment
// surface: method, path, query, and body all match the server REST contract.
func TestKnowledgeConceptRoutes(t *testing.T) {
	tests := []struct {
		name       string
		call       func(a *App) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{
			name: "list concepts with filters",
			call: func(a *App) error {
				_, err := a.ListConcepts("acme", ListConceptsArgs{
					Q: "brand", Status: "approved", Market: "dach", Offset: 20, Limit: 50,
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/concepts",
			wantQuery:  "limit=50&market=dach&offset=20&q=brand&status=approved",
		},
		{
			name: "list concepts unset limit omits paging",
			call: func(a *App) error {
				_, err := a.ListConcepts("acme", ListConceptsArgs{Q: "x"})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/concepts",
			wantQuery:  "q=x",
		},
		{
			name: "get concept",
			call: func(a *App) error {
				_, err := a.GetConcept("acme", "c-1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/concepts/c-1",
		},
		{
			name: "create concept",
			call: func(a *App) error {
				_, err := a.CreateConcept("acme", AddConceptRequest{Domain: "marketing", Definition: "d"})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/concepts",
			wantBody:   `"domain":"marketing"`,
		},
		{
			name: "concept story",
			call: func(a *App) error {
				_, err := a.GetConceptStory("acme", "c-1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/concepts/c-1/story",
		},
		{
			name: "list relations scoped",
			call: func(a *App) error {
				_, err := a.ListConceptRelations("acme", "c-1", "2026-01-02T03:04:05Z", "dach")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/concepts/c-1/relations",
			wantQuery:  "as_of=2026-01-02T03%3A04%3A05Z&market=dach",
		},
		{
			name: "add relation",
			call: func(a *App) error {
				_, err := a.AddConceptRelation("acme", "c-1", AddConceptRelationArgs{
					TargetID: "c-2", RelationType: "BROADER",
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/concepts/c-1/relations",
			wantBody:   `"target_id":"c-2"`,
		},
		{
			name: "delete relation",
			call: func(a *App) error {
				return a.DeleteConceptRelation("acme", "c-1", "r-9")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/acme/concepts/c-1/relations/r-9",
		},
		{
			name: "concept blast radius",
			call: func(a *App) error {
				_, err := a.GetConceptBlastRadius("acme", "c-1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/concepts/c-1/blast-radius",
		},
		{
			name: "list observations",
			call: func(a *App) error {
				_, err := a.ListObservations("acme", "c-1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/concepts/c-1/observations",
		},
		{
			name: "add observation",
			call: func(a *App) error {
				_, err := a.AddObservation("acme", "c-1", AddObservationArgs{
					Kind: "competitor", Quote: "q", Source: "s",
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/concepts/c-1/observations",
			wantBody:   `"kind":"competitor"`,
		},
		{
			name: "delete observation",
			call: func(a *App) error {
				return a.DeleteObservation("acme", "c-1", "o-3")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/acme/concepts/c-1/observations/o-3",
		},
		{
			name: "list comments",
			call: func(a *App) error {
				_, err := a.ListConceptComments("acme", "c-1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/concepts/c-1/comments",
		},
		{
			name: "add comment",
			call: func(a *App) error {
				_, err := a.AddConceptComment("acme", "c-1", AddCommentArgs{Body: "looks good"})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/concepts/c-1/comments",
			wantBody:   `"body":"looks good"`,
		},
		{
			name: "resolve comment",
			call: func(a *App) error {
				return a.ResolveConceptComment("acme", "c-1", "cm-2", true)
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/concepts/c-1/comments/cm-2/resolve",
			wantBody:   `"resolved":true`,
		},
		{
			name: "delete comment",
			call: func(a *App) error {
				return a.DeleteConceptComment("acme", "c-1", "cm-2")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/acme/concepts/c-1/comments/cm-2",
		},
	}

	runKnowledgeRouteTests(t, tests)
}

// TestKnowledgeMarketRoutes covers the market routes.
func TestKnowledgeMarketRoutes(t *testing.T) {
	tests := []struct {
		name       string
		call       func(a *App) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{
			name: "list markets",
			call: func(a *App) error {
				_, err := a.ListMarkets("acme")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/markets",
		},
		{
			name: "create market",
			call: func(a *App) error {
				_, err := a.CreateMarket("acme", MarketArgs{Name: "DACH", Locales: []string{"de-DE", "de-AT"}})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/markets",
			wantBody:   `"name":"DACH"`,
		},
		{
			name: "update market",
			call: func(a *App) error {
				_, err := a.UpdateMarket("acme", "m-1", MarketArgs{Name: "DACH+"})
				return err
			},
			wantMethod: http.MethodPut,
			wantPath:   "/api/v1/acme/markets/m-1",
			wantBody:   `"name":"DACH+"`,
		},
		{
			name: "delete market",
			call: func(a *App) error {
				return a.DeleteMarket("acme", "m-1")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/acme/markets/m-1",
		},
	}

	runKnowledgeRouteTests(t, tests)
}

// TestKnowledgeChangesetRoutes covers the full change-set lifecycle surface.
func TestKnowledgeChangesetRoutes(t *testing.T) {
	tests := []struct {
		name       string
		call       func(a *App) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{
			name: "list changesets by status",
			call: func(a *App) error {
				_, err := a.ListChangesets("acme", "in_review")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/changesets",
			wantQuery:  "status=in_review",
		},
		{
			name: "get changeset",
			call: func(a *App) error {
				_, err := a.GetChangeset("acme", "cs-1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/changesets/cs-1",
		},
		{
			name: "create changeset",
			call: func(a *App) error {
				_, err := a.CreateChangeset("acme", CreateChangesetArgs{Name: "Rename product"})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/changesets",
			wantBody:   `"name":"Rename product"`,
		},
		{
			name: "patch changeset",
			call: func(a *App) error {
				_, err := a.PatchChangeset("acme", "cs-1", UpdateChangesetArgs{Description: "updated"})
				return err
			},
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/acme/changesets/cs-1",
			wantBody:   `"description":"updated"`,
		},
		{
			name: "append op",
			call: func(a *App) error {
				_, err := a.AppendChangesetOp("acme", "cs-1", AddChangesetOpArgs{
					Op:      "relation.remove",
					Payload: map[string]any{"relation_id": "r-1"},
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/changesets/cs-1/ops",
			wantBody:   `"op":"relation.remove"`,
		},
		{
			name: "remove op",
			call: func(a *App) error {
				return a.RemoveChangesetOp("acme", "cs-1", 3)
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/acme/changesets/cs-1/ops/3",
		},
		{
			name: "submit",
			call: func(a *App) error {
				_, err := a.SubmitChangeset("acme", "cs-1")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/changesets/cs-1/submit",
		},
		{
			name: "approve with comment",
			call: func(a *App) error {
				_, err := a.ApproveChangeset("acme", "cs-1", ReviewArgs{Comment: "lgtm"})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/changesets/cs-1/approve",
			wantBody:   `"comment":"lgtm"`,
		},
		{
			name: "reject",
			call: func(a *App) error {
				_, err := a.RejectChangeset("acme", "cs-1", ReviewArgs{})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/changesets/cs-1/reject",
		},
		{
			name: "merge",
			call: func(a *App) error {
				_, err := a.MergeChangeset("acme", "cs-1")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/changesets/cs-1/merge",
		},
		{
			name: "abandon",
			call: func(a *App) error {
				_, err := a.AbandonChangeset("acme", "cs-1")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/changesets/cs-1/abandon",
		},
		{
			name: "changeset blast radius",
			call: func(a *App) error {
				_, err := a.GetChangesetBlastRadius("acme", "cs-1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/changesets/cs-1/blast-radius",
		},
		{
			name: "add pilot",
			call: func(a *App) error {
				_, err := a.AddPilot("acme", "cs-1", StartPilotArgs{ProjectID: "proj-7", Stream: "main"})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/changesets/cs-1/pilots",
			wantBody:   `"project_id":"proj-7"`,
		},
		{
			name: "remove pilot",
			call: func(a *App) error {
				return a.RemovePilot("acme", "cs-1", "proj-7", "main")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/acme/changesets/cs-1/pilots/proj-7/main",
		},
	}

	runKnowledgeRouteTests(t, tests)
}

// TestKnowledgeReturnsDecodedJSON verifies the raw-JSON return path decodes the
// server payload faithfully for the frontend to cast into a typed shape.
func TestKnowledgeReturnsDecodedJSON(t *testing.T) {
	app, _ := newGovTestApp(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"m-1","name":"DACH","locales":["de-DE","de-AT"]}]`))
	})

	raw, err := app.ListMarkets("acme")
	require.NoError(t, err)

	var markets []map[string]any
	require.NoError(t, json.Unmarshal(raw, &markets))
	require.Len(t, markets, 1)
	assert.Equal(t, "DACH", markets[0]["name"])
}

// runKnowledgeRouteTests drives a table of route assertions against a recording
// test server, mirroring TestBrandGovernanceRoutes in governance_test.go.
func runKnowledgeRouteTests(t *testing.T, tests []struct {
	name       string
	call       func(a *App) error
	wantMethod string
	wantPath   string
	wantQuery  string
	wantBody   string
},
) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, rec := newGovTestApp(t, func(w http.ResponseWriter, _ *http.Request) {
				// A benign JSON value all the raw decoders accept.
				_, _ = w.Write([]byte(`{}`))
			})
			require.NoError(t, tt.call(app))
			assert.Equal(t, tt.wantMethod, rec.method)
			assert.Equal(t, tt.wantPath, rec.path)
			if tt.wantQuery != "" {
				assert.Equal(t, tt.wantQuery, rec.query)
			}
			if tt.wantBody != "" {
				assert.Contains(t, rec.body, tt.wantBody)
			}
			assert.Equal(t, "Bearer tok-xyz", rec.auth)
		})
	}
}
