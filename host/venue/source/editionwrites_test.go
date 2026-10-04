package source

import (
	"context"
	"encoding/json"
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

// applyFrench rewrites the French translation of the greeting through the
// change service, as a person's kapi apply does.
func applyFrench(t *testing.T, c *BowrainSourceConnector, text string) {
	t.Helper()
	ctx := context.Background()
	svc, err := c.app.ChangeService(ctx, host.ChangeServiceOptions{Project: c.project.RecipePath(), SourceLocale: "en"})
	require.NoError(t, err)
	fr := model.EditionKey{Locale: "fr"}
	var read change.BlockRead
	_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: "locales/en.json", Editions: []model.EditionKey{fr}},
		func(_ *model.Block, r change.BlockRead) error {
			if r.Ref.Block == "greeting" {
				read = r
			}
			return nil
		})
	require.NoError(t, err)
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindSetContent, At: change.Ref{Doc: "locales/en.json", Block: "greeting", Edition: fr},
		IfMatch: read.Editions["fr"].Rev, Body: &change.SetContent{Text: &text},
	}}}, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
}

// A push carries the recorded write of each translation the checkout holds,
// beside its decisions, until the venue has applied a push that carried it,
// and again when it changes.
func TestPush_SendsTheEditionWritesUntilTheVenueAppliedThem(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 5})
	conn := translatedCheckout(t, srv)
	up(t, conn)

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, srv.writesSent, "the translations the run wrote go with the push")

	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, srv.writesSent, "the venue applied them, and nothing moved")

	applyFrench(t, conn, "Salut tout le monde")
	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 3, srv.writesSent, "the write that changed goes again, alone")
}

// The block history is shared by every branch of a checkout, so the latest
// write of a translation can be another branch's. After a checkout of a branch
// a pass wrote earlier, the push sends the write that left the translation the
// checkout holds: that pass's, with the source it was made from.
func TestPush_SendsTheWriteOfTheTranslationTheCheckoutHolds(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 5})
	conn := translatedCheckout(t, srv)
	en := filepath.Join(conn.project.Root, "locales", "en.json")
	fr := filepath.Join(conn.project.Root, "locales", "fr.json")

	// Branch A, translated by a pass.
	up(t, conn)
	enA, err := os.ReadFile(en)
	require.NoError(t, err)
	frA, err := os.ReadFile(fr)
	require.NoError(t, err)

	// Branch B rewords the greeting, and a pass translates it.
	require.NoError(t, os.WriteFile(en, []byte(`{"greeting": "Hello there", "farewell": "Goodbye now"}`+"\n"), 0o644))
	up(t, conn)

	// git checkout A.
	require.NoError(t, os.WriteFile(en, enA, 0o644))
	require.NoError(t, os.WriteFile(fr, frA, 0o644))
	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)

	var greeting *venue.EditionWrite
	for i, w := range srv.writes {
		if w.Unit == "greeting" || w.Block == "greeting" {
			greeting = &srv.writes[i]
		}
	}
	require.NotNil(t, greeting)
	assert.Equal(t, venue.WriterTool, greeting.Writer, "the pass on A wrote it")
	assert.Equal(t, state.SourceHash("Hello world"), greeting.Basis, "from the source branch A holds")
	b := &model.Block{}
	var held map[string]string
	require.NoError(t, json.Unmarshal(frA, &held))
	b.SetTargetText("fr", held["greeting"])
	assert.Equal(t, model.EditionRevision(b, model.EditionKey{Locale: "fr"}), greeting.Revision, "the translation A holds")
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

// A pull brings down a venue's translation, and the venue's record says which
// source it made it from. Where that is the source the checkout holds, the
// pull records it as the translation's basis, and a later source edit here is
// drift. Where it is another wording (the checkout edited the source and has
// not pushed it), or the venue does not know, the pull records none, and the
// checkout claims nothing about the translation. Either way the push names no
// source for a pulled translation: the venue keeps its own record of what it
// made.
func TestPull_RecordsTheBasisTheVenueGaveATranslation(t *testing.T) {
	cases := []struct {
		name      string
		venue     string // the source the venue's record says it translated
		basis     bool
		staleNext int
	}{
		{name: "the venue made it from the source the checkout holds", venue: "Hello there", basis: true, staleNext: 1},
		{name: "the venue made it from another wording", venue: "Hello world"},
		{name: "the venue does not know its source"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newRefServer(t, "proj1", ref.Ref{Content: 5})
			conn := translatedCheckout(t, srv)
			ctx := context.Background()
			require.NoError(t, os.WriteFile(filepath.Join(conn.project.Root, "locales", "en.json"),
				[]byte(`{"greeting": "Hello there", "farewell": "Goodbye now"}`+"\n"), 0o644))
			match := targetMatchKey("greeting", "Hello there")
			targets := map[string][]model.Run{match: {{Text: &model.TextRun{Text: "Bonjour le monde"}}}}
			var bases map[string]pulledBasis
			if tc.venue != "" {
				bases = map[string]pulledBasis{match: {hash: state.SourceHash(tc.venue)}}
			}
			wrote, err := conn.pullEdition(ctx, pullServices{}, "locales/en.json", "fr", targets, bases, nil)
			require.NoError(t, err)
			require.True(t, wrote)

			svc, err := conn.app.ChangeService(ctx, host.ChangeServiceOptions{Project: conn.project.RecipePath(), SourceLocale: "en"})
			require.NoError(t, err)
			fr := model.EditionKey{Locale: "fr"}
			var read change.BlockRead
			_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: "locales/en.json", Editions: []model.EditionKey{fr}},
				func(_ *model.Block, r change.BlockRead) error {
					if r.Ref.Block == "greeting" {
						read = r
					}
					return nil
				})
			require.NoError(t, err)
			require.NotEmpty(t, read.Editions["fr"].Rev)
			if tc.basis {
				assert.Equal(t, read.Rev, read.Editions["fr"].Basis, "made from the source the checkout holds")
			} else {
				assert.Empty(t, read.Editions["fr"].Basis, "the checkout knows no source it was made from")
			}

			_, err = conn.Push(ctx, bowrainconn.PushOptions{})
			require.NoError(t, err)
			require.Len(t, srv.writes, 1)
			w := srv.writes[0]
			assert.Equal(t, venue.WriterTool, w.Writer)
			assert.Equal(t, venue.OriginPull, w.Origin)
			assert.Equal(t, read.Editions["fr"].Rev, w.Revision)
			assert.Empty(t, w.Basis, "the push names no source for it")
			assert.False(t, w.Produced(), "the venue marks no draft for it")
			assert.False(t, w.KnowsNoBasis(), "and keeps its own basis")

			require.NoError(t, os.WriteFile(filepath.Join(conn.project.Root, "locales", "en.json"),
				[]byte(`{"greeting": "Hello, everyone", "farewell": "Goodbye now"}`+"\n"), 0o644))
			plan, err := conn.app.UpPlan(ctx, conn.project.RecipePath(), "en")
			require.NoError(t, err)
			assert.Equal(t, tc.staleNext, plan.Totals.Stale, "a source edit after the pull")
		})
	}
}

// pulledBases reads the venue's record of each pulled translation: the source
// a decision or a draft of the venue's names, while the record describes the
// translation pulled.
func TestPulledBases_NameTheSourceTheVenueRecorded(t *testing.T) {
	block := func(name, source, target string) apiclient.SyncBlock {
		b := &model.Block{ID: "row-" + name, Name: name, Translatable: true}
		b.SetSourceText(source)
		b.SetTargetText("fr", target)
		return apiclient.BlockToSyncBlock(b, "locales/en.json")
	}
	record := func(unit, source, target string) venue.UnitDecision {
		return venue.UnitDecision{ItemName: "locales/en.json", Unit: unit, Variant: "fr",
			ContentHash: state.SourceHash(source), TargetHash: state.TargetHash(target)}
	}
	blocks := []apiclient.SyncBlock{
		block("greeting", "Hello world", "Bonjour le monde"),
		block("farewell", "Goodbye now", "Au revoir"),
		block("thanks", "Thank you", "Merci"),
		block("title", "Welcome", "Bienvenue"),
	}
	revisioned := record("title", "Welcome", "Bienvenue")
	revisioned.Revision = "r:0000000000000000" // a record of another translation, by revision
	got := pulledBases(blocks, "fr", []venue.UnitDecision{
		record("greeting", "Hello", "Bonjour le monde"),
		record("farewell", "Goodbye now", "Salut"), // a record of another translation
		{ItemName: "locales/en.json", Unit: "thanks", Variant: "fr", TargetHash: state.TargetHash("Merci")},
		revisioned,
	})
	assert.Equal(t, map[string]pulledBasis{targetMatchKey("greeting", "Hello world"): {hash: state.SourceHash("Hello")}}, got)
}

// A venue's record that names the source by revision is read by it: the basis
// names the source the checkout holds while the revision does, under any key a
// read of the document gives it, and a link moved in the source, which no hash
// sees, means it names another.
func TestPulledBasis_NamesTheSourceByRevision(t *testing.T) {
	link := func(href string) []model.Run {
		return []model.Run{
			model.TextR("Read the "),
			model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link", Data: `<a href="` + href + `">`}),
			model.TextR("guide"),
			model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link", Data: "</a>"}),
		}
	}
	held := model.NewRunsBlock("tu1", link("https://a.example"))
	held.SourceLocale = "en"
	venueRead := model.NewRunsBlock("tu1", link("https://a.example"))
	basis := pulledBasis{rev: model.EditionRevision(venueRead, model.EditionKey{}), hash: state.SourceHash(venueRead.SourceText())}
	assert.True(t, basis.names(held, "en"), "the venue read the source under no language; the content is the one held")

	moved := model.NewRunsBlock("tu1", link("https://b.example"))
	moved.SourceLocale = "en"
	assert.False(t, basis.names(moved, "en"), "the link moved under the venue's translation")
	assert.True(t, pulledBasis{hash: basis.hash}.names(moved, "en"), "a record made before revisions is read by its hash")
	assert.False(t, pulledBasis{}.names(held, "en"), "a record that names no source names none")
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
	require.Len(t, srv.writes, 1, "the venue applied the other write, which has not changed")
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
