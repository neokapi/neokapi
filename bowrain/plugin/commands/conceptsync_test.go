package commands

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	bconn "github.com/neokapi/neokapi/bowrain/plugin/connector"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedOp captures one op appended to a change-set during a push.
type recordedOp struct {
	csID    string
	op      string
	payload map[string]any
}

// conceptSyncRecorder records the writes a concept push performs against the
// knowledge-graph surface, so a test can assert ordinary edits went up directly
// while governed edits travelled through a change-set.
type conceptSyncRecorder struct {
	mu         sync.Mutex
	updates    map[string]map[string]any // concept id → PUT body
	creates    []map[string]any
	changesets []string
	ops        []recordedOp
	submits    []string
}

func (r *conceptSyncRecorder) recordUpdate(cid string, body map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.updates == nil {
		r.updates = map[string]map[string]any{}
	}
	r.updates[cid] = body
}

// conceptSyncServer serves the read + write knowledge-graph surface a concept
// pull/push exercises, recording every write into rec. The two seeded concepts
// (c-greeting, c-cta) and the RELATED relation between them are the pull's
// source of truth; writes are recorded but not applied to the read model.
func conceptSyncServer(t *testing.T) (*httptest.Server, *conceptSyncRecorder) {
	t.Helper()
	rec := &conceptSyncRecorder{}

	concepts := []apiclient.ConceptInfo{
		{
			ID:         "c-greeting",
			Domain:     "ui",
			Definition: "A salutation.",
			Terms: []apiclient.TermInfo{
				{Text: "Hi", Locale: "en", Status: "approved"},
				{Text: "Hello", Locale: "en", Status: "approved"},
			},
			CreatedAt: "2026-01-01T10:00:00Z",
			UpdatedAt: "2026-01-02T10:00:00Z",
		},
		{
			ID:         "c-cta",
			Domain:     "ui",
			Definition: "Call to action.",
			Terms: []apiclient.TermInfo{
				{Text: "Get started", Locale: "en", Status: "approved"},
			},
			CreatedAt: "2026-01-01T10:00:00Z",
			UpdatedAt: "2026-01-02T10:00:00Z",
		},
	}
	relation := termbase.ConceptRelation{
		ID:           "r-1",
		SourceID:     "c-greeting",
		TargetID:     "c-cta",
		RelationType: graph.LabelRelated,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/{ws}/concepts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ConceptSearchResult{Concepts: concepts, TotalCount: len(concepts)})
	})
	mux.HandleFunc("GET /api/v1/{ws}/concepts/{cid}/relations", func(w http.ResponseWriter, r *http.Request) {
		cid := r.PathValue("cid")
		if cid == relation.SourceID || cid == relation.TargetID {
			_ = json.NewEncoder(w).Encode([]termbase.ConceptRelation{relation})
			return
		}
		_ = json.NewEncoder(w).Encode([]termbase.ConceptRelation{})
	})

	// Writes — recorded, not applied.
	mux.HandleFunc("PUT /api/v1/{ws}/concepts/{cid}", func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(r)
		rec.recordUpdate(r.PathValue("cid"), body)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/{ws}/concepts", func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(r)
		rec.mu.Lock()
		rec.creates = append(rec.creates, body)
		rec.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.ConceptInfo{ID: "c-created"})
	})
	mux.HandleFunc("POST /api/v1/{ws}/changesets", func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		id := "cs-1"
		rec.changesets = append(rec.changesets, id)
		rec.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.ChangeSet{ID: id, Name: "kapi push", Status: "draft"})
	})
	mux.HandleFunc("POST /api/v1/{ws}/changesets/{id}/ops", func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(r)
		op := recordedOp{csID: r.PathValue("id")}
		if s, ok := body["op"].(string); ok {
			op.op = s
		}
		if p, ok := body["payload"].(map[string]any); ok {
			op.payload = p
		}
		rec.mu.Lock()
		rec.ops = append(rec.ops, op)
		rec.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.ChangeSetOp{Seq: int64(len(rec.ops))})
	})
	mux.HandleFunc("POST /api/v1/{ws}/changesets/{id}/submit", func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.submits = append(rec.submits, r.PathValue("id"))
		rec.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(apiclient.ChangeSet{ID: r.PathValue("id"), Status: "in_review"})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, rec
}

func decodeBody(r *http.Request) map[string]any {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	return body
}

// pullInto runs a concept pull into a fresh temp termbase and returns the path
// and the recorded baseline.
func pullInto(t *testing.T, srv *httptest.Server) (string, *bproject.ConceptBaseline) {
	t.Helper()
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	tbPath := filepath.Join(t.TempDir(), "termbase.db")
	res, baseline, err := PullConcepts(context.Background(), client, tbPath, false)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, baseline)
	return tbPath, baseline
}

// editConcept opens the termbase, applies mutate to the named concept, and
// upserts it, simulating a local edit between pull and push.
func editConcept(t *testing.T, tbPath, conceptID string, mutate func(*termbase.Concept)) {
	t.Helper()
	tb, err := termbase.NewSQLiteTermBase(tbPath)
	require.NoError(t, err)
	defer tb.Close()
	c, ok, err := tb.GetConcept(context.Background(), conceptID)
	require.NoError(t, err)
	require.True(t, ok)
	mutate(&c)
	require.NoError(t, tb.AddConcept(context.Background(), c))
}

func TestPullConceptsWritesTermbaseAndBaseline(t *testing.T) {
	srv, _ := conceptSyncServer(t)
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	tbPath := filepath.Join(t.TempDir(), "termbase.db")

	res, baseline, err := PullConcepts(context.Background(), client, tbPath, false)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Concepts)
	assert.Equal(t, 3, res.Terms)
	assert.Equal(t, 1, res.Relations)

	// The baseline carries the pulled concepts + relation for a later push diff.
	require.NotNil(t, baseline)
	assert.Len(t, baseline.Concepts, 2)
	require.Contains(t, baseline.Concepts, "c-greeting")
	assert.Len(t, baseline.Relations, 1)

	// The concepts + relation are queryable in the local termbase for offline gating.
	tb, err := termbase.NewSQLiteTermBase(tbPath)
	require.NoError(t, err)
	defer tb.Close()
	concepts, err := tb.Concepts(context.Background())
	require.NoError(t, err)
	assert.Len(t, concepts, 2)
	rels, err := tb.RelationsOf(context.Background(), "c-greeting", nil)
	require.NoError(t, err)
	require.Len(t, rels, 1)
	assert.Equal(t, graph.LabelRelated, rels[0].RelationType)
}

func TestPullConceptsDryRunWritesNothing(t *testing.T) {
	srv, _ := conceptSyncServer(t)
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	tbPath := filepath.Join(t.TempDir(), "termbase.db")

	res, baseline, err := PullConcepts(context.Background(), client, tbPath, true)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Concepts)
	require.NotNil(t, baseline)

	// Dry-run must not create the termbase file.
	_, statErr := os.Stat(tbPath)
	assert.True(t, os.IsNotExist(statErr), "dry-run pull must not write the termbase")
}

func TestPushConceptsOrdinaryEditAppliesDirectly(t *testing.T) {
	srv, rec := conceptSyncServer(t)
	tbPath, baseline := pullInto(t, srv)

	// Ordinary edit: change a definition.
	editConcept(t, tbPath, "c-cta", func(c *termbase.Concept) {
		c.Definition = "Primary call to action."
	})

	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	res, err := PushConcepts(context.Background(), client, tbPath, baseline, false)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, 1, res.ConceptsApplied)
	assert.Equal(t, 0, res.ConceptsProposed)
	assert.Empty(t, res.ChangesetID)

	// The edit went up as a direct PUT; no change-set was opened.
	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Contains(t, rec.updates, "c-cta")
	assert.Equal(t, "Primary call to action.", rec.updates["c-cta"]["definition"])
	assert.NotContains(t, rec.updates, "c-greeting")
	assert.Empty(t, rec.changesets, "an ordinary edit must not open a change-set")
}

func TestPushConceptsGovernedEditProposesChangeSet(t *testing.T) {
	srv, rec := conceptSyncServer(t)
	tbPath, baseline := pullInto(t, srv)

	// Governed edit: ban a term (approved → forbidden).
	editConcept(t, tbPath, "c-greeting", func(c *termbase.Concept) {
		for i := range c.Terms {
			if c.Terms[i].Text == "Hi" {
				c.Terms[i].Status = model.TermForbidden
			}
		}
	})

	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	res, err := PushConcepts(context.Background(), client, tbPath, baseline, false)
	require.NoError(t, err)
	require.NotNil(t, res)

	// The governed transition went into a submitted change-set, not a direct write.
	assert.Equal(t, 1, res.ConceptsProposed)
	assert.Equal(t, 0, res.ConceptsApplied)
	assert.Equal(t, "cs-1", res.ChangesetID)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	assert.NotContains(t, rec.updates, "c-greeting", "a governed edit must not be direct-written")
	require.Len(t, rec.changesets, 1)
	require.Len(t, rec.ops, 1)
	require.Len(t, rec.submits, 1)

	op := rec.ops[0]
	assert.Equal(t, "term.status", op.op)
	assert.Equal(t, "c-greeting", op.payload["concept_id"])
	assert.Equal(t, "Hi", op.payload["text"])
	assert.Equal(t, "approved", op.payload["from"])
	assert.Equal(t, "forbidden", op.payload["to"])
}

func TestPushConceptsDryRunNeitherWritesNorProposes(t *testing.T) {
	srv, rec := conceptSyncServer(t)
	tbPath, baseline := pullInto(t, srv)

	editConcept(t, tbPath, "c-greeting", func(c *termbase.Concept) {
		for i := range c.Terms {
			if c.Terms[i].Text == "Hi" {
				c.Terms[i].Status = model.TermForbidden
			}
		}
	})

	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	res, err := PushConcepts(context.Background(), client, tbPath, baseline, true)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.True(t, res.DryRun)
	assert.Equal(t, 1, res.ConceptsProposed)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	assert.Empty(t, rec.updates, "dry-run must not write")
	assert.Empty(t, rec.changesets, "dry-run must not open a change-set")
	assert.Empty(t, rec.ops)
	assert.Empty(t, rec.submits)
}

func TestPushConceptsUnchangedIsNoop(t *testing.T) {
	srv, rec := conceptSyncServer(t)
	tbPath, baseline := pullInto(t, srv)

	// No local edit: the push must be a no-op.
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	res, err := PushConcepts(context.Background(), client, tbPath, baseline, false)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.changed())

	rec.mu.Lock()
	defer rec.mu.Unlock()
	assert.Empty(t, rec.updates)
	assert.Empty(t, rec.creates)
	assert.Empty(t, rec.changesets)
}

func TestPushConceptsNewConceptCreatesDirectly(t *testing.T) {
	srv, rec := conceptSyncServer(t)
	tbPath, baseline := pullInto(t, srv)

	// A brand-new local concept with only proposed terms → ordinary create.
	tb, err := termbase.NewSQLiteTermBase(tbPath)
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(context.Background(), termbase.Concept{
		ID:         "c-new",
		Domain:     "ui",
		Definition: "A freshly minted concept.",
		Terms:      []termbase.Term{{Text: "Widget", Locale: "en", Status: model.TermProposed}},
	}))
	require.NoError(t, tb.Close())

	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	res, err := PushConcepts(context.Background(), client, tbPath, baseline, false)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 1, res.ConceptsApplied)
	assert.Equal(t, 0, res.ConceptsProposed)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Len(t, rec.creates, 1)
	assert.Equal(t, "A freshly minted concept.", rec.creates[0]["definition"])
	assert.Empty(t, rec.changesets)
}

// TestConceptPull_BaselineSurvivesConnectorCloseThenPushReadsIt is the
// end-to-end regression for the pull-ordering bug: when a concept pull is folded
// into a project pull, the baseline it records must survive the sync connector's
// single deferred Close() and be readable by a later push. The bug was that the
// concept pull wrote the baseline to disk independently and the deferred
// conn.Close() then re-saved the connector's own cache (which never carried the
// baseline), erasing it — so every subsequent push found a nil baseline and was
// permanently inert. This test drives the real conceptPull → SetConceptBaseline
// → conn.Close() → conceptPush sequence against an httptest workspace server.
func TestConceptPull_BaselineSurvivesConnectorCloseThenPushReadsIt(t *testing.T) {
	t.Setenv("BOWRAIN_AUTH_TOKEN", "tok")

	srv, rec := conceptSyncServer(t)

	root := t.TempDir()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	recipe := &bproject.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults: coreproj.Defaults{SourceLanguage: "en"},
		},
		Server: &bproject.ServerSpec{
			URL:    srv.URL + "/acme/proj1",
			Stream: "main",
		},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)

	// Simulate a prior block pull that advanced + saved the stream cursor, so the
	// connector loads real block-sync state it will flush on its deferred Close().
	seed := bproject.LoadSyncCache(proj.Layout)
	seed.SetStreamCursor("main", 7)
	require.NoError(t, seed.Save(proj.Layout))

	conn, err := bconn.NewSourceConnector(proj, reg)
	require.NoError(t, err)

	// Reproduce runPull's exact ordering: the block pull saved the cursor (seeded
	// above), then the folded concept pull records its baseline on the connector,
	// and the single deferred conn.Close() flushes both together.
	func() {
		defer conn.Close()
		res, baseline, cerr := conceptPull(context.Background(), proj, false)
		require.NoError(t, cerr)
		require.NotNil(t, res)
		require.NotNil(t, baseline)
		assert.Equal(t, 2, res.Concepts)
		conn.SetConceptBaseline(baseline)
	}()

	// The baseline written by conceptPull must survive the connector's deferred
	// Close() and coexist with the block-sync cursor.
	reloaded := bproject.LoadSyncCache(proj.Layout)
	assert.Equal(t, int64(7), reloaded.GetStreamCursor("main"),
		"block-sync cursor must persist through the deferred conn.Close()")
	require.NotNil(t, reloaded.ConceptBaseline,
		"concept baseline must survive the deferred conn.Close(), not be erased by it")
	assert.Len(t, reloaded.ConceptBaseline.Concepts, 2)

	// A later push reads that persisted baseline: an ordinary local edit applies
	// directly, proving the baseline round-tripped to disk and is usable — i.e.
	// concept push is not inert after a pull.
	tbPath, err := projectTermbasePath(proj)
	require.NoError(t, err)
	editConcept(t, tbPath, "c-cta", func(c *termbase.Concept) {
		c.Definition = "Primary call to action."
	})

	pres, perr := conceptPush(context.Background(), proj, false)
	require.NoError(t, perr)
	require.NotNil(t, pres)
	assert.Equal(t, 1, pres.ConceptsApplied)
	assert.Equal(t, 0, pres.ConceptsProposed)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Contains(t, rec.updates, "c-cta")
	assert.Equal(t, "Primary call to action.", rec.updates["c-cta"]["definition"])
}

func TestPushConceptsNoBaselineSkips(t *testing.T) {
	srv, _ := conceptSyncServer(t)
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	tbPath := filepath.Join(t.TempDir(), "termbase.db")

	res, err := PushConcepts(context.Background(), client, tbPath, nil, false)
	require.NoError(t, err)
	assert.Nil(t, res, "push must skip when there is no pulled baseline")
}
