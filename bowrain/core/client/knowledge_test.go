package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturedRequest records what the test server saw so a test can assert the
// client built the right URL, query, method, and auth header.
type capturedRequest struct {
	method      string
	path        string
	escapedPath string
	query       url.Values
	auth        string
}

// knowledgeServer spins up a test server that records the request and replies
// with the given JSON body, and returns a workspace-scoped client pointed at it.
func knowledgeServer(t *testing.T, body string) (*BowrainClient, *capturedRequest) {
	t.Helper()
	got := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.escapedPath = r.URL.EscapedPath()
		got.query = r.URL.Query()
		got.auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok"), got
}

func TestListConcepts(t *testing.T) {
	body := `{
		"concepts": [
			{"id":"c1","domain":"ui","definition":"the primary call to action","terms":[
				{"text":"Get started","locale":"en-US","status":"preferred"},
				{"text":"Sign up","locale":"en-US","status":"forbidden","note":"use Get started"}
			],"created_at":"2026-06-01T10:00:00Z","updated_at":"2026-06-02T10:00:00Z"}
		],
		"total_count": 1
	}`
	c, got := knowledgeServer(t, body)

	res, err := c.ListConcepts(context.Background(), ListConceptsParams{
		Query:  "started",
		Status: "preferred",
		Domain: "ui",
		Market: "dach",
		Locale: "en-US",
		Source: "brand_vocabulary",
		Offset: 10,
		Limit:  25,
	})
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, got.method)
	assert.Equal(t, "/api/v1/acme/concepts", got.path)
	assert.Equal(t, "Bearer tok", got.auth)
	assert.Equal(t, "started", got.query.Get("q"))
	assert.Equal(t, "preferred", got.query.Get("status"))
	assert.Equal(t, "ui", got.query.Get("domain"))
	assert.Equal(t, "dach", got.query.Get("market"))
	assert.Equal(t, "en-US", got.query.Get("locale"))
	assert.Equal(t, "brand_vocabulary", got.query.Get("source"))
	assert.Equal(t, "10", got.query.Get("offset"))
	assert.Equal(t, "25", got.query.Get("limit"))

	require.Equal(t, 1, res.TotalCount)
	require.Len(t, res.Concepts, 1)
	assert.Equal(t, "c1", res.Concepts[0].ID)
	require.Len(t, res.Concepts[0].Terms, 2)
	assert.Equal(t, "Get started", res.Concepts[0].Terms[0].Text)
	assert.Equal(t, "forbidden", res.Concepts[0].Terms[1].Status)
}

func TestListConceptsOmitsZeroParams(t *testing.T) {
	c, got := knowledgeServer(t, `{"concepts":[],"total_count":0}`)

	_, err := c.ListConcepts(context.Background(), ListConceptsParams{})
	require.NoError(t, err)
	assert.Empty(t, got.query, "no query params should be sent for a zero-value request")
}

func TestGetConcept(t *testing.T) {
	body := `{"id":"c1","project_id":"","domain":"ui","definition":"def","terms":[
		{"text":"Get started","locale":"en-US","status":"preferred"}
	],"created_at":"2026-06-01T10:00:00Z","updated_at":"2026-06-02T10:00:00Z"}`
	c, got := knowledgeServer(t, body)

	concept, err := c.GetConcept(context.Background(), "c1")
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/acme/concepts/c1", got.path)
	assert.Equal(t, "c1", concept.ID)
	assert.Equal(t, "ui", concept.Domain)
	require.Len(t, concept.Terms, 1)
	assert.Equal(t, "preferred", concept.Terms[0].Status)
}

func TestGetConceptEscapesID(t *testing.T) {
	c, got := knowledgeServer(t, `{"id":"a/b","domain":"","definition":"","terms":[],"created_at":"","updated_at":""}`)
	_, err := c.GetConcept(context.Background(), "a/b")
	require.NoError(t, err)
	// The slash in the ID must be percent-escaped so it stays a single path
	// segment on the wire rather than splitting the route.
	assert.Equal(t, "/api/v1/acme/concepts/a%2Fb", got.escapedPath)
}

func TestGetConceptStory(t *testing.T) {
	body := `{"concept_id":"c1","entries":[
		{"kind":"revision","at":"2026-06-01T10:00:00Z","actor":"alice","summary":"created","ref":"1","data":{"rev":1}},
		{"kind":"comment","at":"2026-06-02T10:00:00Z","actor":"bob","summary":"looks good","ref":"cm1"}
	]}`
	c, got := knowledgeServer(t, body)

	story, err := c.GetConceptStory(context.Background(), "c1")
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/acme/concepts/c1/story", got.path)
	assert.Equal(t, "c1", story.ConceptID)
	require.Len(t, story.Entries, 2)
	assert.Equal(t, "revision", story.Entries[0].Kind)
	assert.Equal(t, "alice", story.Entries[0].Actor)
	assert.JSONEq(t, `{"rev":1}`, string(story.Entries[0].Data))
	assert.Equal(t, "comment", story.Entries[1].Kind)
}

func TestListConceptRelations(t *testing.T) {
	body := `[
		{"id":"r1","source_id":"c1","target_id":"c2","relation_type":"REPLACED_BY","note":"renamed",
		 "validity":{"tags":{"market":"dach"}},"created_at":"2026-06-01T10:00:00Z"},
		{"id":"r2","source_id":"c1","target_id":"c3","relation_type":"RELATED","created_at":"2026-06-01T10:00:00Z"}
	]`
	c, got := knowledgeServer(t, body)

	rels, err := c.ListConceptRelations(context.Background(), "c1", "2026-06-01T00:00:00Z", "dach")
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/acme/concepts/c1/relations", got.path)
	assert.Equal(t, "2026-06-01T00:00:00Z", got.query.Get("as_of"))
	assert.Equal(t, "dach", got.query.Get("market"))

	require.Len(t, rels, 2)
	assert.Equal(t, "r1", rels[0].ID)
	assert.Equal(t, graph.LabelReplacedBy, rels[0].RelationType)
	require.NotNil(t, rels[0].Validity)
	assert.Equal(t, "dach", rels[0].Validity.Tags["market"])
	assert.Equal(t, "c3", rels[1].TargetID)
	assert.Nil(t, rels[1].Validity)
}

func TestGetGraph(t *testing.T) {
	body := `{
		"nodes":[{"id":"c1","label":"Get started","domain":"ui","status":"preferred","source":"brand_vocabulary","term_count":2}],
		"edges":[{"id":"r1","source":"c1","target":"c2","type":"RELATED","note":"see also"}]
	}`
	c, got := knowledgeServer(t, body)

	g, err := c.GetGraph(context.Background(), GraphParams{
		Focus:  "c1",
		Depth:  3,
		Domain: "ui",
		Status: "preferred",
		AsOf:   "2026-06-01T00:00:00Z",
		Market: "dach",
	})
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/acme/graph", got.path)
	assert.Equal(t, "c1", got.query.Get("focus"))
	assert.Equal(t, "3", got.query.Get("depth"))
	assert.Equal(t, "ui", got.query.Get("domain"))
	assert.Equal(t, "preferred", got.query.Get("status"))
	assert.Equal(t, "2026-06-01T00:00:00Z", got.query.Get("as_of"))
	assert.Equal(t, "dach", got.query.Get("market"))

	require.Len(t, g.Nodes, 1)
	assert.Equal(t, "Get started", g.Nodes[0].Label)
	assert.Equal(t, 2, g.Nodes[0].TermCount)
	require.Len(t, g.Edges, 1)
	assert.Equal(t, "RELATED", g.Edges[0].Type)
}

func TestListChangesets(t *testing.T) {
	body := `[
		{"id":"cs1","workspace_id":"ws1","name":"Rename CTA","status":"in_review","created_by":"alice",
		 "created_at":"2026-06-01T10:00:00Z","updated_at":"2026-06-01T11:00:00Z"}
	]`
	c, got := knowledgeServer(t, body)

	sets, err := c.ListChangesets(context.Background(), "in_review")
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/acme/changesets", got.path)
	assert.Equal(t, "in_review", got.query.Get("status"))
	require.Len(t, sets, 1)
	assert.Equal(t, "Rename CTA", sets[0].Name)
	assert.Equal(t, "in_review", sets[0].Status)
}

func TestListChangesetsNoStatus(t *testing.T) {
	c, got := knowledgeServer(t, `[]`)
	sets, err := c.ListChangesets(context.Background(), "")
	require.NoError(t, err)
	assert.Empty(t, got.query)
	assert.Empty(t, sets)
}

func TestGetChangeset(t *testing.T) {
	// The server flattens the embedded change-set onto the top level alongside
	// governed/ops/reviews/pilots; ChangeSetDetail must decode both.
	body := `{
		"id":"cs1","workspace_id":"ws1","name":"Rename CTA","status":"in_review","created_by":"alice",
		"created_at":"2026-06-01T10:00:00Z","updated_at":"2026-06-01T11:00:00Z",
		"governed":true,
		"ops":[{"workspace_id":"ws1","changeset_id":"cs1","seq":1,"op":"term.status","payload":{"x":1},"base_rev":2,"created_by":"alice","created_at":"2026-06-01T10:05:00Z"}],
		"reviews":[{"workspace_id":"ws1","changeset_id":"cs1","reviewer":"bob","verdict":"approve","created_at":"2026-06-01T11:30:00Z"}],
		"pilots":[{"workspace_id":"ws1","changeset_id":"cs1","project_id":"p1","stream":"pilot","created_by":"alice","created_at":"2026-06-01T10:10:00Z"}]
	}`
	c, got := knowledgeServer(t, body)

	cs, err := c.GetChangeset(context.Background(), "cs1")
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/acme/changesets/cs1", got.path)

	assert.Equal(t, "cs1", cs.ID) // promoted from the embedded ChangeSet
	assert.Equal(t, "Rename CTA", cs.Name)
	assert.True(t, cs.Governed)
	require.Len(t, cs.Ops, 1)
	assert.Equal(t, "term.status", cs.Ops[0].Op)
	assert.JSONEq(t, `{"x":1}`, string(cs.Ops[0].Payload))
	require.Len(t, cs.Reviews, 1)
	assert.Equal(t, "approve", cs.Reviews[0].Verdict)
	require.Len(t, cs.Pilots, 1)
	assert.Equal(t, "pilot", cs.Pilots[0].Stream)
}

func TestGetChangesetBlastRadius(t *testing.T) {
	body := `{
		"total_blocks":100,"affected_blocks":12,"new_violations":8,"resolved":4,"words":340,
		"projects":[{"project_id":"p1","project_name":"Marketing","affected_blocks":12,"new_violations":8,"resolved":4,"words":340,
			"collections":[{"collection_id":"col1","collection_name":"Homepage","affected_blocks":12,"new_violations":8,"resolved":4,"words":340,
				"locales":[{"stream":"main","locale":"de-DE","affected_blocks":12,"new_violations":8,"resolved":4,"words":340}]}]}],
		"samples":[{"project_id":"p1","stream":"main","collection_id":"col1","collection_name":"Homepage","locale":"de-DE","item_name":"home.json","block_id":"b1","text":"Jetzt registrieren","new_violations":1}]
	}`
	c, got := knowledgeServer(t, body)

	impact, err := c.GetChangesetBlastRadius(context.Background(), "cs1")
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/acme/changesets/cs1/blast-radius", got.path)

	assert.Equal(t, 100, impact.TotalBlocks)
	assert.Equal(t, 12, impact.AffectedBlocks)
	assert.Equal(t, 8, impact.NewViolations)
	require.Len(t, impact.Projects, 1)
	assert.Equal(t, "Marketing", impact.Projects[0].ProjectName)
	require.Len(t, impact.Projects[0].Collections, 1)
	require.Len(t, impact.Projects[0].Collections[0].Locales, 1)
	assert.Equal(t, "de-DE", impact.Projects[0].Collections[0].Locales[0].Locale)
	require.Len(t, impact.Samples, 1)
	assert.Equal(t, "Jetzt registrieren", impact.Samples[0].Text)
	assert.Equal(t, 1, impact.Samples[0].NewViolations)
}

func TestKnowledgeRequiresWorkspace(t *testing.T) {
	// A non-workspace client (claim token) must refuse the workspace-scoped
	// knowledge surface rather than build a malformed URL.
	c := NewClaimTokenClient("http://example.invalid", "proj1", "claim")
	_, err := c.ListConcepts(context.Background(), ListConceptsParams{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace")
}

func TestKnowledgePropagatesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	t.Cleanup(srv.Close)
	c := NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")

	_, err := c.GetConcept(context.Background(), "c1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}
