package editorclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// editorRecord captures one request the editor client made so tests can assert
// route, method, query, auth, and body.
type editorRecord struct {
	method string
	path   string
	query  string
	auth   string
	body   []byte
}

// editorServer routes by exact "METHOD PATH" to a canned (status, jsonBody)
// reply and records the last request. It returns a bare editor client.
func editorServer(t *testing.T, routes map[string]struct {
	status int
	body   string
}) (*EditorClient, *editorRecord) {
	t.Helper()
	got := &editorRecord{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.query = r.URL.RawQuery
		got.auth = r.Header.Get("Authorization")
		got.body, _ = io.ReadAll(r.Body)
		key := r.Method + " " + r.URL.Path
		reply, ok := routes[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"no route ` + key + `"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.status)
		if reply.body != "" {
			_, _ = w.Write([]byte(reply.body))
		}
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "tok"), got
}

type route = struct {
	status int
	body   string
}

func TestEditorListWorkspaces(t *testing.T) {
	c, got := editorServer(t, map[string]route{
		"GET /api/v1/workspaces": {200, `[{"id":"w1","name":"Acme","slug":"acme","description":"d","logo_url":"u","role":"owner"}]`},
	})
	ws, err := c.ListWorkspaces(context.Background())
	require.NoError(t, err)
	require.Len(t, ws, 1)
	assert.Equal(t, "acme", ws[0].Slug)
	assert.Equal(t, "owner", ws[0].Role)
	assert.Equal(t, "Bearer tok", got.auth)
}

func TestEditorProjects(t *testing.T) {
	c, got := editorServer(t, map[string]route{
		"GET /api/v1/acme/projects": {200, `[{"id":"p1","name":"Site","default_source_language":"en-US","target_languages":["fr","de"],"items":[{"id":"i1","name":"a.json","format":"json","type":"file","size":10,"block_count":3,"word_count":9}],"created_at":"t1","modified_at":"t2"}]`},
		"GET /api/v1/acme/p1":       {200, `{"id":"p1","name":"Site","default_source_language":"en-US","target_languages":["fr"],"items":[],"created_at":"t1","modified_at":"t2"}`},
	})
	projs, err := c.ListEditorProjects(context.Background(), "acme")
	require.NoError(t, err)
	require.Len(t, projs, 1)
	assert.Equal(t, "en-US", projs[0].DefaultSourceLanguage)
	require.Len(t, projs[0].Items, 1)
	assert.Equal(t, 3, projs[0].Items[0].BlockCount)

	p, err := c.GetEditorProject(context.Background(), "acme", "p1")
	require.NoError(t, err)
	assert.Equal(t, "Site", p.Name)
	assert.Equal(t, "GET", got.method)
}

func TestEditorBlocksRoundTrip(t *testing.T) {
	// A block with an inline placeholder run to exercise canonical model.Run
	// decoding, plus a target locale carrying the per-locale review status
	// (targets[locale].status — the payload the desktop reads review state from).
	body := `[{"id":"b1","source_runs":[{"text":"Hello "},{"ph":{"id":"1","type":"var","equiv":"NAME"}}],"targets":{"fr":{"text":"Bonjour NAME","status":"reviewed"},"de":{"text":"Hallo NAME"}},"targets_runs":{"fr":[{"text":"Bonjour "},{"ph":{"id":"1","type":"var","equiv":"NAME"}}]},"translatable":true,"properties":{"k":"v"}}]`
	c, got := editorServer(t, map[string]route{
		"GET /api/v1/acme/p1/blocks/main":         {200, body},
		"PUT /api/v1/acme/p1/blocks/main/b1/runs": {204, ""},
	})

	blocks, err := c.GetEditorBlocks(context.Background(), "acme", "p1", "a.json")
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, "item=a.json", got.query)
	require.Len(t, blocks[0].SourceRuns, 2)
	require.NotNil(t, blocks[0].SourceRuns[0].Text)
	assert.Equal(t, "Hello ", blocks[0].SourceRuns[0].Text.Text)
	require.NotNil(t, blocks[0].SourceRuns[1].Ph)
	assert.Equal(t, "NAME", blocks[0].SourceRuns[1].Ph.Equiv)
	require.Contains(t, blocks[0].TargetRuns, "fr")
	assert.True(t, blocks[0].Translatable)
	// Per-locale target text + review status decode (reviewing fr ≠ de).
	require.Contains(t, blocks[0].Targets, "fr")
	assert.Equal(t, "Bonjour NAME", blocks[0].Targets["fr"].Text)
	assert.Equal(t, "reviewed", blocks[0].Targets["fr"].Status)
	assert.Empty(t, blocks[0].Targets["de"].Status)

	// Update round-trips the canonical runs verbatim in the request body.
	runs := []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}
	require.NoError(t, c.UpdateBlockTargetRuns(context.Background(), "acme", "p1", "b1", "fr", runs))
	var sent struct {
		TargetLocale string      `json:"target_locale"`
		Runs         []model.Run `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(got.body, &sent))
	assert.Equal(t, "fr", sent.TargetLocale)
	require.Len(t, sent.Runs, 1)
	require.NotNil(t, sent.Runs[0].Text)
	assert.Equal(t, "Bonjour", sent.Runs[0].Text.Text)
}

func TestEditorBlockLookups(t *testing.T) {
	c, got := editorServer(t, map[string]route{
		"GET /api/v1/acme/p1/blocks/main/b1/tm-matches":   {200, `[{"source":"Hello","target":"Bonjour","score":0.9,"match_type":"fuzzy"}]`},
		"GET /api/v1/acme/p1/blocks/main/b1/term-matches": {200, `[{"source_term":"login","target_terms":["connexion"],"domain":"ui","status":"preferred","start":0,"end":5}]`},
	})
	tm, err := c.LookupMemoryForBlock(context.Background(), "acme", "p1", "b1", "fr")
	require.NoError(t, err)
	require.Len(t, tm, 1)
	assert.InEpsilon(t, 0.9, tm[0].Score, 0.001)
	assert.Equal(t, "target_locale=fr", got.query)

	terms, err := c.LookupTermsForBlock(context.Background(), "acme", "p1", "b1", "fr")
	require.NoError(t, err)
	require.Len(t, terms, 1)
	assert.Equal(t, []string{"connexion"}, terms[0].TargetTerms)
	assert.Equal(t, 5, terms[0].End)
}

func TestEditorMemory(t *testing.T) {
	c, got := editorServer(t, map[string]route{
		"GET /api/v1/acme/translation-memory":       {200, `{"entries":[{"id":"e1","source":"Hi","target":"Salut","source_language":"en","target_language":"fr","updated_at":"t"}],"total_count":1}`},
		"GET /api/v1/acme/translation-memory/count": {200, `{"count":42}`},
		"POST /api/v1/acme/translation-memory":      {201, `{"id":"e2","source":"Bye","target":"Au revoir","source_language":"en","target_language":"fr","updated_at":"t"}`},
		"PUT /api/v1/acme/translation-memory/e2":    {204, ""},
		"DELETE /api/v1/acme/translation-memory/e2": {204, ""},
	})
	res, err := c.GetMemoryEntries(context.Background(), "acme", "hi", "en", "fr", 0, 50)
	require.NoError(t, err)
	assert.Equal(t, 1, res.TotalCount)
	assert.Equal(t, "Salut", res.Entries[0].Target)
	assert.Equal(t, "en", res.Entries[0].SourceLanguage)
	assert.Contains(t, got.query, "q=hi")
	assert.Contains(t, got.query, "limit=50")

	n, err := c.GetMemoryCount(context.Background(), "acme")
	require.NoError(t, err)
	assert.Equal(t, 42, n)

	entry, err := c.AddMemoryEntry(context.Background(), "acme", "Bye", "Au revoir", "en", "fr")
	require.NoError(t, err)
	assert.Equal(t, "e2", entry.ID)
	var addBody map[string]string
	require.NoError(t, json.Unmarshal(got.body, &addBody))
	assert.Equal(t, "Bye", addBody["source"])
	assert.Equal(t, "fr", addBody["target_locale"])

	require.NoError(t, c.UpdateMemoryEntry(context.Background(), "acme", "e2", "Bye", "A+", "en", "fr"))
	require.NoError(t, c.DeleteMemoryEntry(context.Background(), "acme", "e2"))
	assert.Equal(t, "DELETE", got.method)
}

func TestEditorConcepts(t *testing.T) {
	c, got := editorServer(t, map[string]route{
		"GET /api/v1/acme/concepts":              {200, `{"concepts":[{"id":"c1","domain":"ui","definition":"d","terms":[{"text":"Login","locale":"en","status":"preferred"}],"created_at":"t","updated_at":"t"}],"total_count":1}`},
		"GET /api/v1/acme/concepts/count":        {200, `{"count":7}`},
		"POST /api/v1/acme/concepts":             {201, `{"id":"c2","domain":"ui","definition":"d","terms":[],"created_at":"t","updated_at":"t"}`},
		"PUT /api/v1/acme/concepts/c2":           {204, ""},
		"DELETE /api/v1/acme/concepts/c2":        {204, ""},
		"POST /api/v1/acme/concepts/import/csv":  {200, `{"imported":3}`},
		"POST /api/v1/acme/concepts/import/json": {200, `{"imported":5}`},
		"GET /api/v1/acme/concepts/export/json":  {200, `{"concepts":[]}`},
	})
	res, err := c.GetTerms(context.Background(), "acme", "log", "en", "fr", 0, 50)
	require.NoError(t, err)
	require.Len(t, res.Concepts, 1)
	assert.Equal(t, "Login", res.Concepts[0].Terms[0].Text)
	assert.Contains(t, got.query, "locale=en")

	n, err := c.GetTermCount(context.Background(), "acme")
	require.NoError(t, err)
	assert.Equal(t, 7, n)

	concept, err := c.EditorAddConcept(context.Background(), "acme", "ui", "d", []EditorTerm{{Text: "Login", Locale: "en", Status: "preferred"}})
	require.NoError(t, err)
	assert.Equal(t, "c2", concept.ID)

	require.NoError(t, c.EditorUpdateConcept(context.Background(), "acme", "c2", "ui", "d2", nil))
	require.NoError(t, c.EditorDeleteConcept(context.Background(), "acme", "c2"))

	csv, err := c.ImportTermsCSV(context.Background(), "acme", "a,b", "en", "fr", "ui", true)
	require.NoError(t, err)
	assert.Equal(t, 3, csv)

	js, err := c.ImportTermsJSON(context.Background(), "acme", `{"concepts":[]}`)
	require.NoError(t, err)
	assert.Equal(t, 5, js)

	exp, err := c.ExportTermsJSON(context.Background(), "acme", "glossary")
	require.NoError(t, err)
	assert.JSONEq(t, `{"concepts":[]}`, exp)
	assert.Contains(t, got.query, "name=glossary")
}

func TestEditorProviders(t *testing.T) {
	c, got := editorServer(t, map[string]route{
		"GET /api/v1/acme/providers":        {200, `[{"id":"pc1","name":"OpenAI","provider_type":"openai","model":"gpt-4o","base_url":""}]`},
		"POST /api/v1/acme/providers":       {201, `{"id":"pc2","name":"Anthropic","provider_type":"anthropic","model":"claude","base_url":""}`},
		"DELETE /api/v1/acme/providers/pc2": {204, ""},
		"POST /api/v1/acme/providers/test":  {204, ""},
	})
	cfgs, err := c.ListProviderConfigs(context.Background(), "acme")
	require.NoError(t, err)
	require.Len(t, cfgs, 1)
	assert.Equal(t, "openai", cfgs[0].ProviderType)

	saved, err := c.SaveProviderConfig(context.Background(), "acme", EditorSaveProviderRequest{Name: "Anthropic", ProviderType: "anthropic", Model: "claude", APIKey: "sk-x"})
	require.NoError(t, err)
	assert.Equal(t, "pc2", saved.ID)
	var sent map[string]string
	require.NoError(t, json.Unmarshal(got.body, &sent))
	assert.Equal(t, "sk-x", sent["api_key"])

	require.NoError(t, c.TestProviderConfig(context.Background(), "acme", EditorSaveProviderRequest{ProviderType: "anthropic"}))
	require.NoError(t, c.DeleteProviderConfig(context.Background(), "acme", "pc2"))
	assert.Equal(t, "DELETE", got.method)
}
