package host

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
)

// changeProject writes a project with one collection and its files, and
// returns an App and the recipe.
func changeProject(t *testing.T, item project.ContentItem, files map[string]string, edit ...func(*project.KapiProject)) (*App, string) {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	recipe := filepath.Join(root, project.RecipeFileName)
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "changes",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"de", "fr"},
		},
		Collections: []project.Collection{{Name: "docs", Content: []project.ContentItem{item}}},
	}
	for _, e := range edit {
		e(proj)
	}
	require.NoError(t, project.Save(recipe, proj))
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

// TestChangeAssets_ADecisionBindsToTheContentThatLanded pins that a decision
// is recorded on the content the change service hands the host, which it read
// under the commit lock, and never on what the file says by the time the
// decision is written: a save that reaches the file in between leaves the
// decision stale instead of taking it over.
func TestChangeAssets_ADecisionBindsToTheContentThatLanded(t *testing.T) {
	item := project.ContentItem{Path: "locales/en.json", Target: "locales/{lang}.json"}
	a, recipe := changeProject(t, item, map[string]string{
		"locales/en.json": `{"title": "Welcome"}` + "\n",
		"locales/de.json": `{"title": "Willkommen"}` + "\n",
	})
	root := filepath.Dir(recipe)
	svc := changeService(t, a, recipe)
	ctx := context.Background()
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "locales/en.json", Editions: []model.EditionKey{editionKey(t, "de")}})
	require.NoError(t, err)
	title := blockWith(t, page, "Welcome")
	at := title.Ref
	at.Edition = editionKey(t, "de")
	assets := &changeAssets{app: a, recipe: recipe}
	decide := func(target *change.DecisionTarget) {
		t.Helper()
		status, cerr := assets.Apply(ctx, changePerson, &change.Set{}, change.Op{Kind: change.KindDecide, At: target.Ref, Body: &change.Decide{Outcome: change.OutcomeEstablish}}, target)
		require.Nil(t, cerr)
		assert.Equal(t, change.OpApplied, status)
	}
	write := func(rel, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(body), 0o644))
	}

	t.Run("a translation", func(t *testing.T) {
		// Another writer saves the German file after the change set landed
		// "Willkommen" and before its decision is written.
		write("locales/de.json", `{"title": "Hallo"}`+"\n")
		decide(&change.DecisionTarget{Doc: change.DocInfo{Doc: "locales/en.json"}, Ref: at,
			Place: change.Place{Kind: change.PlaceOwnFile, File: "locales/de.json"}, Rev: title.Editions["de"].Rev,
			Text: "Willkommen", SourceText: "Welcome", Role: change.RoleDerived})

		ref := ReviewUnitRef{File: filepath.FromSlash("locales/de.json"), Key: title.Ref.Block, Locale: "de"}
		info, err := a.ReviewUnit(ctx, recipe, "", ref)
		require.NoError(t, err)
		assert.Equal(t, "Hallo", info.Target)
		assert.NotEqual(t, string(model.TargetStatusEstablished), info.Status, "the decision is about Willkommen, so Hallo is not established")

		write("locales/de.json", `{"title": "Willkommen"}`+"\n")
		info, err = a.ReviewUnit(ctx, recipe, "", ref)
		require.NoError(t, err)
		assert.Equal(t, string(model.TargetStatusEstablished), info.Status, "the wording the decision was made on is established")
	})

	t.Run("the document's own wording", func(t *testing.T) {
		write("locales/en.json", `{"title": "Hello there"}`+"\n")
		decide(&change.DecisionTarget{Doc: change.DocInfo{Doc: "locales/en.json"}, Ref: title.Ref,
			Place: change.Place{Kind: change.PlaceInDocument}, Rev: title.Rev, Text: "Welcome", SourceText: "Welcome", Role: change.RoleAuthoritative})

		st, err := a.OpenProjectState(ctx, root)
		require.NoError(t, err)
		scope := a.documentIndexOrEmpty(ctx, root).Scope(root, filepath.Join(root, "locales", "en.json"))
		rec, ok := st.Get(ctx, state.Key{Scope: scope, Unit: title.Ref.Block, Variant: sourceVariant("en")})
		require.True(t, ok, "the approval is recorded")
		assert.Equal(t, state.SourceHash("Welcome"), rec.ContentHash, "the approval binds to the wording that landed")
	})
}

// TestChangeService_FormatOverrideTakesNoConfigurationWrittenForAnother pins
// that --format in place of the format a content item binds reads with the
// project's defaults for the format used: the item's configuration is written
// for its own reader, and another reader refuses its keys.
func TestChangeService_FormatOverrideTakesNoConfigurationWrittenForAnother(t *testing.T) {
	item := project.ContentItem{
		Path:   "docs/*.md",
		Format: &project.FormatSpec{Name: "markdown", Config: map[string]any{"translateFrontMatter": true, "frontMatterKeys": []any{"title"}}},
	}
	a, recipe := changeProject(t, item, map[string]string{"docs/tides.md": formatConfigDoc})
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Project: recipe, Format: "plaintext"})
	require.NoError(t, err)
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: "docs/tides.md"})
	require.NoError(t, err)
	assert.Equal(t, "plaintext", page.Format)
	assert.NotEmpty(t, page.Blocks)
}

// TestChangeService_AnArchiveMemberTakesTheProjectsFormatDefaults pins that a
// member of an archive in a project is read with the project's configuration
// of the member's format.
func TestChangeService_AnArchiveMemberTakesTheProjectsFormatDefaults(t *testing.T) {
	item := project.ContentItem{Path: "docs/*.md"}
	a, recipe := changeProject(t, item, map[string]string{"docs/readme.md": "# Readme\n"}, func(p *project.KapiProject) {
		p.Defaults.Formats = map[string]project.FormatDefaults{"markdown": {Config: map[string]any{
			"translateFrontMatter": true, "frontMatterKeys": []any{"title"},
		}}}
	})
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("docs/tides.md")
	require.NoError(t, err)
	_, err = io.WriteString(w, formatConfigDoc)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(recipe), "bundle.zip"), buf.Bytes(), 0o644))

	svc := changeService(t, a, recipe)
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: "bundle.zip!docs/tides.md"})
	require.NoError(t, err)
	blockWith(t, page, "Tide tables")
	for _, b := range page.Blocks {
		assert.NotEqual(t, "How to read them", b.Text, "the project's configuration reads the title alone from the front matter")
	}
}

// TestChangeService_ABilingualFileIsEditedInTheLanguageItHolds pins that the
// translation a PO catalog holds is read and written as the edition of the
// language the service is told, and that without one the edit is refused
// rather than reported applied and dropped.
func TestChangeService_ABilingualFileIsEditedInTheLanguageItHolds(t *testing.T) {
	po := "msgid \"\"\nmsgstr \"\"\n\"Language: fr\\n\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	tests := []struct {
		name   string
		target model.LocaleID
		status change.SetStatus
		want   string
	}{
		{"with the language it holds", "fr", change.SetApplied, strings.Replace(po, `msgstr "Bonjour"`, `msgstr "Salut"`, 1)},
		{"without one", "", change.SetRefused, po},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "fr.po"), []byte(po), 0o644))
			a := &App{}
			a.InitRegistries()
			svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Root: dir, TargetLocale: tc.target})
			require.NoError(t, err)
			page, err := svc.Read(context.Background(), change.ReadRequest{Doc: "fr.po"})
			require.NoError(t, err)
			hello := blockWith(t, page, "Hello")
			rev := model.AbsentRevision
			if ed, ok := hello.Editions["fr"]; ok {
				assert.Equal(t, "Bonjour", ed.Text)
				rev = ed.Rev
			}
			at := hello.Ref
			at.Edition = editionKey(t, "fr")
			res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{setTo(at, rev, "Salut")}}, changePerson)
			require.NoError(t, err)
			require.Equal(t, tc.status, res.Status, "%+v", res.Ops)
			b, err := os.ReadFile(filepath.Join(dir, "fr.po"))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(b))
		})
	}
}

// TestChangeService_AProjectCatalogsTranslationLivesInItsTargetFile pins that
// a PO source the recipe gives a target template keeps each translation in
// the file the template names, for every service: the French of po/en.po (or
// of the template po/messages.pot) is read from and written to po/fr.po,
// whether the service is told a target language (kapi apply, MCP, the
// browser and Kapi Desktop derive one from the change set) or not, and
// whether the change names the source or the translation's file. The source
// catalog keeps its bytes.
func TestChangeService_AProjectCatalogsTranslationLivesInItsTargetFile(t *testing.T) {
	const msgs = "msgid \"Hello\"\nmsgstr \"%s\"\n"
	catalog := func(lang, msgstr string) string {
		return "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\"Language: " + lang + "\\n\"\n\n" + fmt.Sprintf(msgs, msgstr)
	}
	tests := []struct {
		name, source string
		target       model.LocaleID
		doc          string
	}{
		{"a catalog, told the language", "po/en.po", "fr", "po/en.po"},
		{"a catalog, told no language", "po/en.po", "", "po/en.po"},
		{"a catalog, addressed by the translation's file", "po/en.po", "fr", "po/fr.po"},
		{"a template, told the language", "po/messages.pot", "fr", "po/messages.pot"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := catalog("en", "")
			fr := catalog("fr", "Bonjour")
			a, recipe := changeProject(t, project.ContentItem{
				Path: tc.source, Format: &project.FormatSpec{Name: "po"}, Target: "po/{lang}.po",
			}, map[string]string{tc.source: src, "po/fr.po": fr})
			ctx := context.Background()
			svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "test", TargetLocale: tc.target})
			require.NoError(t, err)
			french := editionKey(t, "fr")
			page, err := svc.Read(ctx, change.ReadRequest{Doc: tc.doc, Editions: []model.EditionKey{french}})
			require.NoError(t, err)
			require.Len(t, page.Blocks, 1)
			hello := page.Blocks[0]
			ed, ok := hello.Editions["fr"]
			if hello.Ref.Edition == french {
				// A read of the translation's file names its edition.
				ed, ok = change.EditionRead{Rev: hello.Rev, Text: hello.Text}, true
			}
			require.True(t, ok, "the French the translation's file holds reads back")
			assert.Equal(t, "Bonjour", ed.Text)

			at := hello.Ref
			at.Edition = french
			res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(at, ed.Rev, "Salut")}}, changePerson)
			require.NoError(t, err)
			require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
			assert.Equal(t, catalog("fr", "Salut"), readFile(t, recipe, "po/fr.po"), "the translation lands in its own file")
			assert.Equal(t, src, readFile(t, recipe, tc.source), "the source catalog keeps its bytes")
		})
	}
}

// TestChangeService_KeepsItsLockFilesOutOfACommit pins that the lock files a
// project's change service takes sit in .kapi/work, under the ignore rule the
// project's state directory is created with.
func TestChangeService_KeepsItsLockFilesOutOfACommit(t *testing.T) {
	item := project.ContentItem{Path: "locales/en.json"}
	a, recipe := changeProject(t, item, map[string]string{"locales/en.json": `{"title": "Welcome"}` + "\n"})
	root := filepath.Dir(recipe)
	svc := changeService(t, a, recipe)
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: "locales/en.json"})
	require.NoError(t, err)
	b := page.Blocks[0]
	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{setTo(b.Ref, b.Rev, "Welcome aboard")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	locks, err := os.ReadDir(filepath.Join(root, project.StateDirName, project.WorkDirName, "locks"))
	require.NoError(t, err)
	assert.NotEmpty(t, locks, "the lock files sit under .kapi/work/locks")
	ignore, err := os.ReadFile(filepath.Join(root, project.StateDirName, project.StateGitignoreFilename))
	require.NoError(t, err)
	assert.True(t, project.GitignoreCovers(string(ignore), project.WorkDirName), "the state directory's ignore rule covers them: %q", ignore)
}

// TestChangeService_AReadWritesNothing pins that reading a project's document
// and describing its format leave the project as they found it: the state
// directory and its ignore rule are written by the first commit.
func TestChangeService_AReadWritesNothing(t *testing.T) {
	item := project.ContentItem{Path: "locales/en.json", Target: "locales/{lang}.json"}
	a, recipe := changeProject(t, item, map[string]string{"locales/en.json": `{"title": "Welcome"}` + "\n"})
	root := filepath.Dir(recipe)
	svc := changeService(t, a, recipe)
	ctx := context.Background()
	_, err := svc.Read(ctx, change.ReadRequest{Doc: "locales/en.json", Editions: []model.EditionKey{editionKey(t, "de")}})
	require.NoError(t, err)
	_, err = svc.Describe(ctx, change.DescribeRequest{Doc: "locales/en.json"})
	require.NoError(t, err)
	h, err := svc.History(ctx, change.HistoryRequest{Ref: change.Ref{Doc: "locales/en.json", Block: "title"}})
	require.NoError(t, err)
	assert.Empty(t, h.Entries, "a project with no store has recorded no change")
	_, err = os.Stat(filepath.Join(root, project.StateDirName))
	assert.ErrorIs(t, err, os.ErrNotExist, "a read creates no state directory")
}

// TestChangeService_ASourceDocumentResolvesWithoutTheWholeRecipe pins that a
// reference to a source file locates its document, translations included,
// without expanding the recipe's content patterns, and that the file of a
// translation, which no pattern of a source names, expands them once.
func TestChangeService_ASourceDocumentResolvesWithoutTheWholeRecipe(t *testing.T) {
	item := project.ContentItem{Path: "locales/en.json", Target: "locales/{lang}.json"}
	a, recipe := changeProject(t, item, map[string]string{
		"locales/en.json": `{"title": "Welcome"}` + "\n",
		"locales/de.json": `{"title": "Willkommen"}` + "\n",
	})
	l, err := a.newProjectLayout(ChangeServiceOptions{Project: recipe})
	require.NoError(t, err)
	ctx := context.Background()

	d, err := l.Locate(ctx, "locales/en.json")
	require.NoError(t, err)
	assert.Equal(t, "json", d.Format.Name)
	assert.Equal(t, []model.EditionKey{{Locale: "de"}}, d.Derived, "the source's translations are found from the source alone")
	f, ok := d.EditionFile(model.EditionKey{Locale: "fr"})
	require.True(t, ok, "a declared translation with no file yet has a home")
	assert.Equal(t, "locales/fr.json", f.Ref)
	assert.Nil(t, l.index, "no content pattern was expanded")

	d, err = l.Locate(ctx, "locales/de.json")
	require.NoError(t, err)
	assert.Equal(t, "locales/en.json", d.Ref)
	require.NotNil(t, d.Edition)
	assert.Equal(t, model.LocaleID("de"), d.Edition.Locale)
	assert.NotNil(t, l.index, "the file of a translation is found by expanding the recipe")
}

// TestChangeAssets_APreReviewThatAnnotatesNothingIsRefused pins that an
// agent's pre-review the review queue has no unit for is refused as
// not_found, rather than reported as recorded.
func TestChangeAssets_APreReviewThatAnnotatesNothingIsRefused(t *testing.T) {
	item := project.ContentItem{Path: "locales/en.json", Target: "locales/{lang}.json"}
	a, recipe := changeProject(t, item, map[string]string{
		"locales/en.json": `{"title": "Welcome"}` + "\n",
		"locales/de.json": `{"title": "Willkommen"}` + "\n",
	})
	assets := &changeAssets{app: a, recipe: recipe}
	score := 80
	at := change.Ref{Doc: "locales/en.json", Block: "no-such-block", Edition: editionKey(t, "de")}
	target := &change.DecisionTarget{Doc: change.DocInfo{Doc: "locales/en.json"}, Ref: at,
		Place: change.Place{Kind: change.PlaceOwnFile, File: "locales/de.json"}, Text: "Willkommen", SourceText: "Welcome", Role: change.RoleDerived}
	agent := change.Actor{Kind: change.ActorAgent, Name: "review-agent", Session: "s1"}
	_, cerr := assets.Apply(context.Background(), agent, &change.Set{},
		change.Op{Kind: change.KindDecide, At: at, Body: &change.Decide{Outcome: change.OutcomeAdvise, Score: &score}}, target)
	require.NotNil(t, cerr)
	assert.Equal(t, change.CodeNotFound, cerr.Code)
}
