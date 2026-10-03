package host

import (
	"context"
	"fmt"
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

// TestChangeRead_ABasisRecordedAfterTheServiceFirstReadIsRead pins that a
// service that first read a project before it had a store, as a long-lived
// surface's does, reads the basis a later change records there. The
// translation sits in its own file before any store exists, so the first read
// asks the history about it and finds none.
func TestChangeRead_ABasisRecordedAfterTheServiceFirstReadIsRead(t *testing.T) {
	a, recipe := changeProject(t, project.ContentItem{Path: "docs/*.md", Target: "i18n/{lang}/{path}.md"}, map[string]string{
		"docs/guide.md":    "# Install\n\nRun the installer.\n",
		"i18n/fr/guide.md": "# Installer\n\nLancez-le.\n",
	})
	t.Cleanup(a.Shutdown)
	ctx := t.Context()
	svc := changeService(t, a, recipe)
	fr, err := model.ParseEditionKey("fr")
	require.NoError(t, err)
	read := func() change.BlockRead {
		page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md", Editions: []model.EditionKey{fr}})
		require.NoError(t, err)
		return blockWith(t, page, "Run the installer.")
	}

	before := read()
	require.Equal(t, "Lancez-le.", before.Editions["fr"].Text)
	require.Empty(t, before.Editions["fr"].Basis, "no store, so no history vouches for the translation")

	at := before.Ref
	at.Edition = fr
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(at, before.Editions["fr"].Rev, "Lancez le programme d'installation.")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status)

	after := read()
	assert.Equal(t, before.Rev, after.Editions["fr"].Basis, "the basis the change recorded")
	assert.False(t, after.Editions["fr"].Stale)
}

// translatedGuide writes a project holding one Markdown document of n
// paragraphs, translates every paragraph into French through one change set,
// so the block history holds a basis for each, and returns the App, the
// recipe and a service over the project.
func translatedGuide(tb testing.TB, n int) (*App, *change.Service) {
	tb.Helper()
	return translatedGuideIn(tb, n, []model.LocaleID{"fr"}, 1)
}

// translatedGuideIn is translatedGuide into every language of langs, each
// translation rewritten writes times, one change set per language and
// rewrite, so the block history holds writes changes to every translation.
func translatedGuideIn(tb testing.TB, n int, langs []model.LocaleID, writes int) (*App, *change.Service) {
	tb.Helper()
	root := tb.TempDir()
	var doc strings.Builder
	doc.WriteString("# Guide\n\n")
	for i := range n {
		fmt.Fprintf(&doc, "Paragraph %d tells the reader how the shop opens.\n\n", i)
	}
	require.NoError(tb, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(tb, os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte(doc.String()), 0o644))
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(tb, project.Save(recipe, &project.KapiProject{
		Version:  project.CurrentVersion,
		Name:     "bench",
		Defaults: project.Defaults{SourceLanguage: "en", TargetLanguages: langs},
		Collections: []project.Collection{{Name: "docs", Content: []project.ContentItem{
			{Path: "docs/*.md", Target: "i18n/{lang}/{path}.md"},
		}}},
	}))
	a := &App{}
	a.InitRegistries()
	tb.Cleanup(a.Shutdown)
	ctx := context.Background()
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "test"})
	require.NoError(tb, err)

	for _, lang := range langs {
		k := model.EditionKey{Locale: lang}.Canonical()
		for w := range writes {
			page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md", Editions: []model.EditionKey{k}, Limit: change.MaxReadLimit})
			require.NoError(tb, err)
			var set change.Set
			for _, b := range page.Blocks {
				at := b.Ref
				at.Edition = k
				rev := model.AbsentRevision
				if ed, ok := b.Editions[string(lang)]; ok {
					rev = ed.Rev
				}
				set.Ops = append(set.Ops, setTo(at, rev, fmt.Sprintf("%s%d %s", strings.ToUpper(string(lang)), w, b.Text)))
			}
			res, err := svc.Apply(ctx, set, changePerson)
			require.NoError(tb, err)
			require.Equal(tb, change.SetApplied, res.Status)
		}
	}
	return a, svc
}

// BenchmarkChangeRead_TranslatedDocument measures a read of a document whose
// every block has a recorded translation, the read that asks the block
// history where each translation's basis stands.
func BenchmarkChangeRead_TranslatedDocument(b *testing.B) {
	_, svc := translatedGuide(b, 400)
	ctx := context.Background()
	fr, err := model.ParseEditionKey("fr")
	require.NoError(b, err)
	for b.Loop() {
		page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md", Editions: []model.EditionKey{fr}, Limit: change.MaxReadLimit})
		if err != nil {
			b.Fatal(err)
		}
		if len(page.Blocks) == 0 || page.Blocks[1].Editions["fr"].Basis == "" {
			b.Fatal("the read names no basis")
		}
	}
}

// BenchmarkChangeRead_MultilingualHistory measures reads of one language of a
// document translated into five, each translation rewritten three times: a
// read naming one block, a page of 100 blocks, and the whole document. The
// block history holds every language's changes, and only the language read
// is shown.
func BenchmarkChangeRead_MultilingualHistory(b *testing.B) {
	_, svc := translatedGuideIn(b, 400, []model.LocaleID{"fr", "de", "ja", "nb", "es"}, 3)
	ctx := context.Background()
	fr, err := model.ParseEditionKey("fr")
	require.NoError(b, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md", Editions: []model.EditionKey{fr}, Limit: 1})
	require.NoError(b, err)
	one := page.Blocks[0].Ref.Block
	based := func(r change.BlockRead) {
		if r.Editions["fr"].Basis == "" {
			b.Fatalf("no basis for %s", r.Ref.Block)
		}
	}
	b.Run("one-block", func(b *testing.B) {
		for b.Loop() {
			page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md", Editions: []model.EditionKey{fr}, Blocks: []string{one}, Limit: 1})
			if err != nil || len(page.Blocks) != 1 {
				b.Fatal(err)
			}
			based(page.Blocks[0])
		}
	})
	b.Run("page-of-100", func(b *testing.B) {
		for b.Loop() {
			page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md", Editions: []model.EditionKey{fr}, Limit: 100})
			if err != nil || len(page.Blocks) != 100 {
				b.Fatal(err)
			}
			based(page.Blocks[99])
		}
	})
	b.Run("whole-document", func(b *testing.B) {
		for b.Loop() {
			_, err := svc.ReadEach(ctx, change.ReadRequest{Doc: "docs/guide.md", Editions: []model.EditionKey{fr}}, func(_ *model.Block, r change.BlockRead) error {
				based(r)
				return nil
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkInterchangeRevisions measures the read kapi extract makes of every
// (source, target) pair: the source with its translation's file joined, for
// the revisions each unit carries.
func BenchmarkInterchangeRevisions(b *testing.B) {
	_, svc := translatedGuide(b, 400)
	ctx := context.Background()
	for b.Loop() {
		revs, err := interchangeRevisions(ctx, svc, "docs/guide.md", "fr")
		if err != nil {
			b.Fatal(err)
		}
		if len(revs) < 400 {
			b.Fatalf("read %d units", len(revs))
		}
	}
}
