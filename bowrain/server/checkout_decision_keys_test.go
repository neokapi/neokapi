package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	bconn "github.com/neokapi/neokapi/host/venue/source"
)

// A checkout's records name a block by the key its reader gives it, and the
// venue files the block under the key a push resolves it to. These tests drive
// a real checkout (the change service, a flow and BowrainSourceConnector's push
// and pull) against the real server, each push landing in PostgreSQL, so the
// two meet only where a push and a pull carry them.

const keyedItem = "locales/en.json"

// keyedCheckout is a checkout of a project the server holds nothing of yet,
// written in en-US and translated into French by a pseudo-translation flow.
type keyedCheckout struct {
	srv  *Server
	pid  string
	app  *host.App
	proj *bproject.Project
	conn *bconn.BowrainSourceConnector
}

func newKeyedCheckout(t *testing.T) *keyedCheckout {
	t.Helper()
	srv, token := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test/projects", strings.NewReader(
		`{"name":"Keys","default_source_language":"en-US","target_languages":["fr"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	ts := httptest.NewServer(srv.GetEcho())
	t.Cleanup(ts.Close)

	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Setenv("BOWRAIN_PROJECT_URL", "")
	t.Setenv("BOWRAIN_AUTH_TOKEN", token)

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(keyedItem)),
		[]byte(`{"greeting": "Hello there", "farewell": "Goodbye now"}`+"\n"), 0o644))
	recipe := &bproject.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage: "en-US", TargetLanguages: []model.LocaleID{"fr"}, Flow: "pseudo",
			TranslateAfter: string(model.TranslateAfterNone), Materialize: coreproj.MaterializeManual,
		},
		Collections: []coreproj.Collection{{Name: "site", Path: keyedItem, Target: "locales/{lang}.json"}},
		Flows:       map[string]*flow.StepsSpec{"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}}},
		Server:      &bproject.ServerSpec{URL: ts.URL + "/test/" + created.ID, Stream: "main"},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)

	app := &host.App{}
	t.Cleanup(app.Shutdown)
	app.InitRegistries()
	app.SourceLang = "en-US"
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	conn, err := bconn.NewSourceConnector(app, proj, reg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return &keyedCheckout{srv: srv, pid: created.ID, app: app, proj: proj, conn: conn}
}

// up runs one convergence pass on the checkout, as kapi up does.
func (k *keyedCheckout) up(t *testing.T) {
	t.Helper()
	cmd := host.NewEnvCommand(context.Background(), "up")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	k.app.AddFlowRunFlags(cmd)
	host.AddUpFlags(cmd)
	host.AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", k.proj.RecipePath()))
	proj, err := coreproj.Load(k.proj.RecipePath())
	require.NoError(t, err)
	require.NoError(t, k.app.RunDefaultFlowConverge(cmd, proj, k.proj.RecipePath(), host.ConvergeOptions{MaxPasses: 1}))
}

// approve approves the French translation of key through the change service,
// as a person's review on the checkout does, and returns the revision it
// approved.
func (k *keyedCheckout) approve(t *testing.T, key string) string {
	t.Helper()
	ctx := context.Background()
	svc, err := k.app.ChangeService(ctx, host.ChangeServiceOptions{Project: k.proj.RecipePath(), SourceLocale: "en-US"})
	require.NoError(t, err)
	fr := model.EditionKey{Locale: "fr"}
	var read change.BlockRead
	_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: keyedItem, Editions: []model.EditionKey{fr}},
		func(_ *model.Block, r change.BlockRead) error {
			if r.Ref.Block == key {
				read = r
			}
			return nil
		})
	require.NoError(t, err)
	require.NotEmpty(t, read.Editions["fr"].Rev, "the flow translated %s", key)
	at := read.Ref
	at.Edition = fr
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindDecide, At: at, IfMatch: read.Editions["fr"].Rev,
		Body: &change.Decide{Outcome: change.OutcomeEstablish},
	}}}, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	return read.Editions["fr"].Rev
}

// push pushes the checkout and lands the push as a deployed worker does.
func (k *keyedCheckout) push(t *testing.T) {
	t.Helper()
	res, err := k.conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	drainWithAuthority(t, k.srv)
	require.Nil(t, res.Governance, "the server accepted every verdict the push carried")
}

// stored is the row the server holds for the block the checkout's reader
// names name.
func (k *keyedCheckout) stored(t *testing.T, name string) *venue.StoredBlock {
	t.Helper()
	rows, err := k.srv.ContentStore.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: k.pid, Stream: "main", ItemName: keyedItem})
	require.NoError(t, err)
	for _, sb := range rows {
		if sb.Block.Name == name {
			return sb
		}
	}
	t.Fatalf("the server holds no %s", name)
	return nil
}

// A checkout translates its catalog and approves one translation before the
// project's first push. The server holds nothing to resolve the push against,
// so it files each block under a key the push mints for it; the approval the
// same push carries lands on that unit, and the review reads it there.
func TestCheckoutDecisionJoinsTheBlockItsFirstPushFiled(t *testing.T) {
	k := newKeyedCheckout(t)
	k.up(t)
	approved := k.approve(t, "greeting")

	k.push(t)

	sb := k.stored(t, "greeting")
	require.NotEqual(t, "greeting", sb.Key, "the push minted the unit a key of its own")

	ds, ok := k.srv.ContentStore.(platstore.DecisionStore)
	require.True(t, ok)
	records, err := ds.ListUnitDecisions(t.Context(), k.pid, "main")
	require.NoError(t, err)
	var units []string
	for _, d := range records {
		if d.Variant == "fr" && d.IsDecision() {
			units = append(units, d.ItemName+"|"+d.Unit)
		}
	}
	assert.Equal(t, []string{keyedItem + "|" + sb.Key}, units,
		"the approval is filed under the unit the server holds, and under no other key")

	d := k.srv.unitDecisionFor(t.Context(), k.pid, "main", sb, "fr")
	require.NotNil(t, d, "the review reads the approval on the block it judges")
	assert.Equal(t, venue.ReviewStateApproved, d.ReviewState)
	assert.Equal(t, approved, d.Revision)
	assert.False(t, reviewProvenanceOf(d, sb, "fr").Stale, "the approval's source is the one the server holds")

	tallies, err := ds.TallyDecisionBasis(t.Context(), k.pid, "main")
	require.NoError(t, err)
	for _, tl := range tallies {
		if tl.Variant == "fr" {
			assert.Zero(t, tl.Stale, "nothing reads stale")
			assert.Zero(t, tl.BasisUnknown, "the approval names its source")
		}
	}
}

// A decision the server holds comes back to the checkout under the key the
// checkout's records name its block by, so the change service reads it on the
// block it judges, and the next push sends it under the server's key again.
func TestPulledDecisionLandsOnTheCheckoutsBlock(t *testing.T) {
	k := newKeyedCheckout(t)
	k.up(t)
	k.approve(t, "greeting")
	k.push(t)

	_, err := k.conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)

	st, err := k.app.OpenProjectState(context.Background(), k.proj.Root)
	require.NoError(t, err)
	units, err := st.All(context.Background())
	require.NoError(t, err)
	var keys []string
	for _, u := range units {
		if u.Variant.Locale == "fr" && u.Decision.ReviewState != "" {
			keys = append(keys, u.Unit)
		}
	}
	assert.ElementsMatch(t, []string{"greeting"}, unique(keys),
		"the checkout holds the approval under its reader's key alone")
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
