package commands

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/ref/refcache"
	"github.com/neokapi/neokapi/core/registry"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	bconn "github.com/neokapi/neokapi/host/venue/source"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChangesetURLMatchesTheWebRoute pins the review link the CLI hands out
// against the route the web app actually serves.
//
// It built /<ws>/changesets/<id>, which has never been a route: the app serves
// /<ws>/context/changes/<id>. Every push that proposed governed terminology
// therefore ended by printing a 404 — the change-set was real and a reviewer
// was waiting on it, and the only thing pointing at it was wrong, so following
// the link was indistinguishable from nothing having happened.
//
// The assertion reads the route table rather than restating a string, because a
// link that is merely self-consistent is exactly what shipped last time.
func TestChangesetURLMatchesTheWebRoute(t *testing.T) {
	proj := &bproject.Project{
		Recipe: &bproject.Recipe{
			Server: &bproject.ServerSpec{URL: "https://bowrain.cloud/acme/proj123/"},
		},
	}

	got := changesetURL(proj, "cs-42")
	assert.Equal(t, "https://bowrain.cloud/acme/context/changes/cs-42", got)

	// The web app's route table is the authority for that path. Read it: a
	// rename on the frontend must break this test, not the founder's click.
	routes := filepath.Join("..", "..", "packages", "app", "src", "routes", "index.tsx")
	src, err := os.ReadFile(routes)
	require.NoError(t, err,
		"the web route table is the authority for the CLI's review link; if it moved, point this test at it")

	// Reduce to a bool BEFORE asserting. assert.Contains prints its haystack,
	// and the haystack here is the whole route table — 33KB of unrelated TSX
	// inlined as one escaped string, with the sentence explaining the breakage
	// last. A cross-layer test earns its keep only if whoever trips it can see
	// in one line what they broke and why it mattered; buried under forty pages
	// of imports it reads as noise from a package they have never opened.
	assert.True(t, strings.Contains(string(src), `path: "changes/$id"`),
		`%s no longer declares path: "changes/$id" — kapi links /<ws>/context/changes/<id> `+
			`after a push proposes governed terminology, so that link now 404s for everyone it reaches`,
		routes)
	assert.False(t, strings.Contains(string(src), `path: "changesets/$id"`),
		`%s declares path: "changesets/$id" — that was the CLI's old broken link; `+
			`if the app now serves it, changesetURL should be pointed back at it deliberately, not by accident`,
		routes)
}

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
	relation := terms.ConceptRelation{
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
			_ = json.NewEncoder(w).Encode([]terms.ConceptRelation{relation})
			return
		}
		_ = json.NewEncoder(w).Encode([]terms.ConceptRelation{})
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

// newProjectTerms opens a throwaway project's store and returns its terms
// schema — the shape a concept pull and push now work against. One handle for
// the whole test: the store carries every subsystem, so a second opener would be
// a second connection pool on one file.
func newProjectTerms(t *testing.T) *terms.SQLiteStore {
	t.Helper()
	root := t.TempDir()
	db, err := projectdb.Open(t.Context(), coreproj.Layout{
		Root: root, StateDir: filepath.Join(root, coreproj.StateDirName),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db.Terms()
}

// pullInto runs a concept pull into a fresh project's terms and returns the
// store and the recorded baseline.
func pullInto(t *testing.T, srv *httptest.Server) (*terms.SQLiteStore, *bproject.ConceptBaseline) {
	t.Helper()
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	tb := newProjectTerms(t)
	res, baseline, err := PullConcepts(context.Background(), client, tb, "", false)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, baseline)
	return tb, baseline
}

// editConcept applies mutate to the named concept and upserts it, simulating a
// local edit between pull and push.
func editConcept(t *testing.T, tb *terms.SQLiteStore, conceptID string, mutate func(*terms.Concept)) {
	t.Helper()
	c, ok, err := tb.GetConcept(context.Background(), conceptID)
	require.NoError(t, err)
	require.True(t, ok)
	mutate(&c)
	require.NoError(t, tb.AddConcept(context.Background(), c))
}

func TestPullConceptsWritesTermsAndBaseline(t *testing.T) {
	srv, _ := conceptSyncServer(t)
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	tb := newProjectTerms(t)

	res, baseline, err := PullConcepts(context.Background(), client, tb, "", false)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Concepts)
	assert.Equal(t, 3, res.Terms)
	assert.Equal(t, 1, res.Relations)

	// The baseline carries the pulled concepts + relation for a later push diff.
	require.NotNil(t, baseline)
	assert.Len(t, baseline.Concepts, 2)
	require.Contains(t, baseline.Concepts, "c-greeting")
	assert.Len(t, baseline.Relations, 1)

	// The concepts + relation are queryable in the project's terms for offline
	// gating.
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
	tb := newProjectTerms(t)

	res, baseline, err := PullConcepts(context.Background(), client, tb, "", true)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Concepts)
	require.NotNil(t, baseline)

	// Dry-run must write no concept. The old assertion was that the terms FILE
	// stayed absent, which the merged store cannot answer: every subsystem's
	// schema exists from the store's first open, so emptiness is the only
	// evidence that nothing was written.
	concepts, err := tb.Concepts(context.Background())
	require.NoError(t, err)
	assert.Empty(t, concepts, "dry-run pull must not write into the terms store")
}

func TestPushConceptsOrdinaryEditAppliesDirectly(t *testing.T) {
	srv, rec := conceptSyncServer(t)
	tb, baseline := pullInto(t, srv)

	// Ordinary edit: change a definition.
	editConcept(t, tb, "c-cta", func(c *terms.Concept) {
		c.Definition = "Primary call to action."
	})

	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	res, err := PushConcepts(context.Background(), client, tb, baseline, "", false)
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
	tb, baseline := pullInto(t, srv)

	// Governed edit: ban a term (approved → forbidden).
	editConcept(t, tb, "c-greeting", func(c *terms.Concept) {
		for i := range c.Terms {
			if c.Terms[i].Text == "Hi" {
				c.Terms[i].Status = model.TermForbidden
			}
		}
	})

	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	res, err := PushConcepts(context.Background(), client, tb, baseline, "", false)
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
	tb, baseline := pullInto(t, srv)

	editConcept(t, tb, "c-greeting", func(c *terms.Concept) {
		for i := range c.Terms {
			if c.Terms[i].Text == "Hi" {
				c.Terms[i].Status = model.TermForbidden
			}
		}
	})

	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	res, err := PushConcepts(context.Background(), client, tb, baseline, "", true)
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
	tb, baseline := pullInto(t, srv)

	// No local edit: the push must be a no-op.
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	res, err := PushConcepts(context.Background(), client, tb, baseline, "", false)
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
	tb, baseline := pullInto(t, srv)

	// A brand-new local concept with only proposed terms → ordinary create.
	require.NoError(t, tb.AddConcept(context.Background(), terms.Concept{
		ID:         "c-new",
		Domain:     "ui",
		Definition: "A freshly minted concept.",
		Terms:      []terms.Term{{Text: "Widget", Locale: "en", Status: model.TermProposed}},
	}))

	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	res, err := PushConcepts(context.Background(), client, tb, baseline, "", false)
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

	// The App is what reaches the project's store, and it is what closes it:
	// terminology now lives in `.kapi/work/store.db`, so pull, push and the connector
	// all go through the one handle this App memoizes for the root.
	prev := app
	app = &cli.App{}
	t.Cleanup(func() {
		app.Shutdown()
		app = prev
	})

	srv, rec := conceptSyncServer(t)

	root := t.TempDir()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	recipe := &bproject.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
		Server: &bproject.ServerSpec{
			URL:    srv.URL + "/acme/proj1",
			Stream: "main",
		},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)

	// Simulate a prior block pull that advanced + saved the stream position, so
	// the connector loads real block-sync state it will flush on its deferred
	// Close().
	seedRefs := refcache.Load(proj.Layout, config.NormalizeServerURL(srv.URL), "proj1")
	seedRefs.Consume("main", 7)
	require.NoError(t, seedRefs.Save(proj.Layout))

	conn, err := bconn.NewSourceConnector(app, proj, reg)
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
	assert.Equal(t, int64(7),
		refcache.Load(proj.Layout, config.NormalizeServerURL(srv.URL), "proj1").Ref("main").Content,
		"the block-sync position must persist through the deferred conn.Close()")
	require.NotNil(t, reloaded.ConceptBaseline,
		"concept baseline must survive the deferred conn.Close(), not be erased by it")
	assert.Len(t, reloaded.ConceptBaseline.Concepts, 2)

	// A later push reads that persisted baseline: an ordinary local edit applies
	// directly, proving the baseline round-tripped to disk and is usable — i.e.
	// concept push is not inert after a pull.
	projTerms, err := projectTerms(t.Context(), proj)
	require.NoError(t, err)
	editConcept(t, projTerms, "c-cta", func(c *terms.Concept) {
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

// A project with no terminology of its own has nothing to push. That is the one
// condition that legitimately skips, and it is now a row question the caller
// asks (HasTerms) rather than a stat of a terms FILE — the file stopped
// distinguishing anything once every subsystem shared one store.
func TestPushConceptsWithoutTermsSkips(t *testing.T) {
	srv, _ := conceptSyncServer(t)
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")

	res, err := PushConcepts(context.Background(), client, nil, nil, "", false)
	require.NoError(t, err)
	assert.Nil(t, res, "push must skip when the project has no terms")
}
