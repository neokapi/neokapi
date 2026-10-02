package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// changeProject writes a project with one collection and its files, and
// returns an App and the recipe.
func changeProject(t *testing.T, item project.ContentItem, files map[string]string) (*App, string) {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "changes",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"de", "fr"},
		},
		Collections: []project.Collection{{Name: "docs", Content: []project.ContentItem{item}}},
	}))
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
	a := &App{}
	a.InitRegistries()
	return a, recipe
}

func changeService(t *testing.T, a *App, recipe string) *change.Service {
	t.Helper()
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Project: recipe, Origin: "test"})
	require.NoError(t, err)
	return svc
}

func readFile(t *testing.T, recipe, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(filepath.Dir(recipe), filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(b)
}

func blockWith(t *testing.T, page *change.Page, text string) change.BlockRead {
	t.Helper()
	for _, b := range page.Blocks {
		if b.Text == text {
			return b
		}
	}
	require.Failf(t, "no block reads "+text, "%+v", page.Blocks)
	return change.BlockRead{}
}

var changePerson = change.Actor{Kind: change.ActorPerson}

func setTo(at change.Ref, ifMatch, text string) change.Op {
	return change.Op{Kind: change.KindSetContent, At: at, IfMatch: ifMatch, Body: &change.SetContent{Text: &text}}
}

func editionKey(t *testing.T, s string) model.EditionKey {
	t.Helper()
	k, err := model.ParseEditionKey(s)
	require.NoError(t, err)
	return k
}

// TestChangeService_ReadsAFileWithTheRecipesFormatBinding pins that a
// document is read and written with the format and the configuration its
// content item declares: front matter the recipe makes content is a block an
// operation can address.
func TestChangeService_ReadsAFileWithTheRecipesFormatBinding(t *testing.T) {
	item := project.ContentItem{
		Path:   "docs/*.md",
		Target: "i18n/{lang}/{path}.md",
		Format: &project.FormatSpec{Name: "markdown", Config: map[string]any{
			"translateFrontMatter": true,
			"frontMatterKeys":      []any{"title"},
		}},
	}
	a, recipe := changeProject(t, item, map[string]string{"docs/tides.md": formatConfigDoc})
	svc := changeService(t, a, recipe)
	ctx := context.Background()
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/tides.md"})
	require.NoError(t, err)
	title := blockWith(t, page, "Tide tables")

	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(title.Ref, title.Rev, "Tide charts")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, strings.Replace(formatConfigDoc, "title: Tide tables", "title: Tide charts", 1), readFile(t, recipe, "docs/tides.md"))
}

func TestChangeService_ATranslationIsAnEditionOfItsSource(t *testing.T) {
	item := project.ContentItem{Path: "docs/*.md", Target: "i18n/{lang}/{path}.md"}
	source := "# Install\n\nRun the installer.\n"
	german := "# Installieren\n\nFühre das Installationsprogramm aus.\n"
	a, recipe := changeProject(t, item, map[string]string{
		"docs/guide.md":    source,
		"i18n/de/guide.md": german,
	})
	svc := changeService(t, a, recipe)
	ctx := context.Background()

	t.Run("a read joins the German file to the source by address", func(t *testing.T) {
		page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md", Editions: []model.EditionKey{editionKey(t, "de")}})
		require.NoError(t, err)
		p := blockWith(t, page, "Run the installer.")
		require.Contains(t, p.Editions, "de")
		assert.Equal(t, "Führe das Installationsprogramm aus.", p.Editions["de"].Text)
	})

	t.Run("a read of the German file shows the German edition, and its reference edits it", func(t *testing.T) {
		page, err := svc.Read(ctx, change.ReadRequest{Doc: "i18n/de/guide.md"})
		require.NoError(t, err)
		assert.Equal(t, "docs/guide.md", page.Doc)
		p := blockWith(t, page, "Führe das Installationsprogramm aus.")
		assert.Equal(t, "de", p.Ref.EditionText(), "the reference names the German edition")
		require.Contains(t, p.Editions, "en", "the document's own edition is listed among the others")
		assert.Equal(t, "Run the installer.", p.Editions["en"].Text)

		res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(p.Ref, p.Rev, "Führe das neue Installationsprogramm aus.")}}, changePerson)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		assert.Equal(t, "# Installieren\n\nFühre das neue Installationsprogramm aus.\n", readFile(t, recipe, "i18n/de/guide.md"))
		assert.Equal(t, source, readFile(t, recipe, "docs/guide.md"), "copying the reference and revision of the German read edits the German")

		// A filter by the key the German file reads the block with finds it.
		page, err = svc.Read(ctx, change.ReadRequest{Doc: "i18n/de/guide.md", Blocks: []string{"installieren/p"}})
		require.NoError(t, err)
		require.Len(t, page.Blocks, 1)
		assert.Equal(t, p.Ref.Block, page.Blocks[0].Ref.Block)
	})

	t.Run("the German file addressed directly echoes the canonical reference", func(t *testing.T) {
		page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md", Editions: []model.EditionKey{editionKey(t, "de")}})
		require.NoError(t, err)
		p := blockWith(t, page, "Run the installer.")
		de := p.Editions["de"]

		// The block keyed as the German file names it: its heading is German.
		res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{
			setTo(change.Ref{Doc: "i18n/de/guide.md", Block: "installieren/p"}, de.Rev, "Starte das Installationsprogramm."),
		}}, changePerson)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		require.NotNil(t, res.Ops[0].At)
		assert.Equal(t, "docs/guide.md", res.Ops[0].At.Doc)
		assert.Equal(t, p.Ref.Block, res.Ops[0].At.Block)
		assert.Equal(t, "de", res.Ops[0].At.EditionText())
		assert.Equal(t, "# Installieren\n\nStarte das Installationsprogramm.\n", readFile(t, recipe, "i18n/de/guide.md"))
		assert.Equal(t, source, readFile(t, recipe, "docs/guide.md"), "the source is untouched")
	})

	t.Run("a translation with no file yet is written from the source's skeleton", func(t *testing.T) {
		page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md"})
		require.NoError(t, err)
		p := blockWith(t, page, "Run the installer.")
		at := p.Ref
		at.Edition = editionKey(t, "fr")
		res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(at, model.AbsentRevision, "Lancez le programme d'installation.")}}, changePerson)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		assert.Equal(t, "# Install\n\nLancez le programme d'installation.\n", readFile(t, recipe, "i18n/fr/guide.md"))
	})

	t.Run("an edit to the source names the translations it leaves stale", func(t *testing.T) {
		page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md"})
		require.NoError(t, err)
		p := blockWith(t, page, "Run the installer.")
		res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(p.Ref, p.Rev, "Run the new installer.")}}, changePerson)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		var stale []string
		for _, inv := range res.Ops[0].Invalidates {
			stale = append(stale, inv.Edition)
		}
		assert.ElementsMatch(t, []string{"de", "fr"}, stale)
	})

	t.Run("an edition the recipe does not declare has no home", func(t *testing.T) {
		page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md"})
		require.NoError(t, err)
		p := page.Blocks[0]
		at := p.Ref
		at.Edition = editionKey(t, "ja")
		res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(at, model.AbsentRevision, "x")}}, changePerson)
		require.NoError(t, err)
		require.Equal(t, change.SetRefused, res.Status)
		assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	})
}

func TestChangeService_DecisionsAndTermsLandAfterTheContent(t *testing.T) {
	item := project.ContentItem{Path: "locales/en.json", Target: "locales/{lang}.json"}
	a, recipe := changeProject(t, item, map[string]string{
		"locales/en.json": `{"title": "Welcome"}` + "\n",
		"locales/de.json": `{"title": "Wilkommen"}` + "\n",
	})
	svc := changeService(t, a, recipe)
	ctx := context.Background()
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "locales/en.json", Editions: []model.EditionKey{editionKey(t, "de")}})
	require.NoError(t, err)
	title := blockWith(t, page, "Welcome")
	de := title.Editions["de"]
	at := title.Ref
	at.Edition = editionKey(t, "de")

	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{
		setTo(at, de.Rev, "Willkommen"),
		{Kind: change.KindDecide, At: at, IfMatch: de.Rev, Body: &change.Decide{Outcome: change.OutcomeEstablish}},
		{Kind: change.KindTerm, Body: &change.Term{Action: "upsert", Term: "Welcome", Status: "preferred"}},
	}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, `{"title": "Willkommen"}`+"\n", readFile(t, recipe, "locales/de.json"))
	assert.Equal(t, change.OpApplied, res.Ops[1].Status)
	assert.Equal(t, change.OpApplied, res.Ops[2].Status)

	info, err := a.ReviewUnit(ctx, recipe, "", ReviewUnitRef{File: filepath.FromSlash("locales/de.json"), Key: title.Ref.Block, Locale: "de"})
	require.NoError(t, err)
	assert.Equal(t, string(model.TargetStatusEstablished), info.Status, "the decision binds to the wording that landed")
	assert.Equal(t, "Willkommen", info.Target)

	db, err := a.ProjectDB(ctx, filepath.Dir(recipe))
	require.NoError(t, err)
	concepts, err := db.Terms().Concepts(ctx)
	require.NoError(t, err)
	require.Len(t, concepts, 1)
	assert.Equal(t, "Welcome", concepts[0].Terms[0].Text)

	t.Run("an agent's decision is refused before anything is written", func(t *testing.T) {
		page, err := svc.Read(ctx, change.ReadRequest{Doc: "locales/en.json", Editions: []model.EditionKey{editionKey(t, "de")}})
		require.NoError(t, err)
		title := blockWith(t, page, "Welcome")
		de := title.Editions["de"]
		at := title.Ref
		at.Edition = editionKey(t, "de")
		res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{
			setTo(at, de.Rev, "Hallo"),
			{Kind: change.KindDecide, At: at, IfMatch: de.Rev, Body: &change.Decide{Outcome: change.OutcomeEstablish}},
		}}, change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s1"})
		require.NoError(t, err)
		require.Equal(t, change.SetRefused, res.Status)
		assert.Equal(t, change.CodeNotPermitted, res.Ops[1].Error.Code)
		assert.Equal(t, `{"title": "Willkommen"}`+"\n", readFile(t, recipe, "locales/de.json"))
	})
}

func TestChangeService_OutsideAProjectReadsTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.json"), []byte(`{"a": "One"}`), 0o600))
	a := &App{}
	a.InitRegistries()
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Root: dir})
	require.NoError(t, err)
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: "notes.json"})
	require.NoError(t, err)
	b := page.Blocks[0]
	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{
		setTo(b.Ref, b.Rev, "Uno"),
		{Kind: change.KindTerm, Body: &change.Term{Action: "upsert", Term: "One"}},
	}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status, "a term needs a project to land in")
	assert.Equal(t, change.CodeUnsupported, res.Ops[1].Error.Code)

	res, err = svc.Apply(context.Background(), change.Set{Ops: []change.Op{setTo(b.Ref, b.Rev, "Uno")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	info, err := os.Stat(filepath.Join(dir, "notes.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
