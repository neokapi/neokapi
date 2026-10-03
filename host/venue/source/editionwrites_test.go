package source

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/ref/refcache"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// recordTranslation records a flow's write of the French translation of the
// greeting, made from the source the checkout holds, as kapi up records it.
func recordTranslation(t *testing.T, c *BowrainSourceConnector, text string) {
	t.Helper()
	b := &model.Block{ID: "greeting", Name: "greeting", Translatable: true, SourceLocale: "en"}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", text)
	fr := model.EditionKey{Locale: "fr"}
	rec, err := c.app.EditRecorder(t.Context(), c.project.Root)
	require.NoError(t, err)
	_, err = rec.Record(t.Context(), change.Record{
		Actor: change.Actor{Kind: change.ActorTool, Name: "translate"}, Origin: "flow:translate",
		Docs: []change.DocResult{{Doc: "locales/en.json", Home: "file", Written: true}},
		Transitions: []change.Transition{{
			Ref: change.Ref{Doc: "locales/en.json", Block: "greeting", Edition: fr}, Role: change.RoleDerived,
			BeforeRev: model.AbsentRevision, AfterRev: model.EditionRevision(b, fr),
			Basis: model.EditionRevision(b, model.EditionKey{Locale: "en"}), Block: b,
		}},
	})
	require.NoError(t, err)
}

// A push carries the last recorded write of each translation the checkout
// holds, beside its decisions, until the venue has applied them, and again
// when a write moves.
func TestPush_SendsTheEditionWritesUntilTheVenueAppliedThem(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 5})
	conn := newRefConnector(t, srv.Server, "proj1")
	recordTranslation(t, conn, "Bonjour")

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, srv.writesSent, "the translation the run wrote goes with the push")

	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, srv.writesSent, "the venue applied them, and nothing moved")

	recordTranslation(t, conn, "Salut")
	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, srv.writesSent, "a later write goes again")
}

// translatedCheckout is a checkout of proj1 on srv whose English catalog a
// pseudo-translation flow translates into French.
func translatedCheckout(t *testing.T, srv *refServer) *BowrainSourceConnector {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en.json"),
		[]byte(`{"greeting": "Hello world", "farewell": "Goodbye now"}`+"\n"), 0o644))
	recipe := &bproject.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}, Flow: "pseudo",
			TranslateAfter: string(model.TranslateAfterNone), Materialize: coreproj.MaterializeManual,
		},
		Collections: []coreproj.Collection{{Name: "app", Path: "locales/en.json", Target: "locales/{lang}.json"}},
		Flows:       map[string]*flow.StepsSpec{"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}}},
		Server:      &bproject.ServerSpec{URL: srv.URL + "/projects/proj1", Stream: "main"},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	client := apiclient.NewProjectBearerClient(srv.URL, "proj1", "test-token")
	client.SetStream("main")
	a := testApp(t)
	a.InitRegistries()
	a.SourceLang = "en"
	return &BowrainSourceConnector{
		app: a, project: proj, client: client, formatReg: reg,
		cache:  bproject.LoadSyncCache(proj.Layout),
		refs:   refcache.Load(proj.Layout, config.NormalizeServerURL(srv.URL), "proj1"),
		stream: "main", maxBatch: 1000,
	}
}

// up runs one convergence pass on the checkout, as kapi up does.
func up(t *testing.T, c *BowrainSourceConnector) {
	t.Helper()
	cmd := host.NewEnvCommand(context.Background(), "up")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	c.app.AddFlowRunFlags(cmd)
	host.AddUpFlags(cmd)
	host.AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", c.project.RecipePath()))
	proj, err := coreproj.Load(c.project.RecipePath())
	require.NoError(t, err)
	require.NoError(t, c.app.RunDefaultFlowConverge(cmd, proj, c.project.RecipePath(), host.ConvergeOptions{MaxPasses: 1}))
}

// After kapi up, the push carries how each translation was written: the source
// the run made it from, and that a tool in the flow wrote it. After a person
// rewrites one, the push says the person wrote it, from the source in front of
// them.
func TestPush_CarriesHowEachTranslationWasWritten(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 5})
	conn := translatedCheckout(t, srv)
	up(t, conn)

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	require.Len(t, srv.writes, 2, "one write for each translation the run produced")
	sources := map[string]string{"greeting": "Hello world", "farewell": "Goodbye now"}
	svc, err := conn.app.ChangeService(context.Background(), host.ChangeServiceOptions{Project: conn.project.RecipePath(), SourceLocale: "en"})
	require.NoError(t, err)
	fr := model.EditionKey{Locale: "fr"}
	reads := map[string]change.BlockRead{}
	_, err = svc.ReadEach(context.Background(), change.ReadRequest{Doc: "locales/en.json", Editions: []model.EditionKey{fr}},
		func(_ *model.Block, r change.BlockRead) error {
			reads[r.Ref.Block] = r
			return nil
		})
	require.NoError(t, err)
	for _, w := range srv.writes {
		local := w.Unit
		if w.Block != "" {
			local = w.Block
		}
		assert.Equal(t, "locales/en.json", w.ItemName)
		assert.Equal(t, "fr", w.Variant)
		assert.Equal(t, venue.WriterTool, w.Writer, "%s: a tool in the flow wrote it", local)
		assert.Equal(t, "flow:pseudo", w.Origin)
		assert.Equal(t, state.SourceHash(sources[local]), w.Basis, "%s: the source the run made it from", local)
		assert.Equal(t, reads[local].Editions["fr"].Rev, w.Revision, "%s: the translation on disk", local)
		assert.False(t, w.ByHand())
	}

	// A person rewrites the greeting's translation through kapi apply.
	text := "Bonjour tout le monde"
	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{{
		Kind: change.KindSetContent, At: change.Ref{Doc: "locales/en.json", Block: "greeting", Edition: fr},
		IfMatch: reads["greeting"].Editions["fr"].Rev, Body: &change.SetContent{Text: &text},
	}}}, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	var greeting *venue.EditionWrite
	for i, w := range srv.writes {
		if w.Unit == "greeting" || w.Block == "greeting" {
			greeting = &srv.writes[i]
		}
	}
	require.NotNil(t, greeting, "the push carries the new write")
	assert.Equal(t, venue.WriterPerson, greeting.Writer)
	assert.Equal(t, "apply", greeting.Origin)
	assert.Equal(t, state.SourceHash("Hello world"), greeting.Basis, "the source in front of the person who wrote it")
	assert.True(t, greeting.ByHand(), "the pusher wrote it")
}
