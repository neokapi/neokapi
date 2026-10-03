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
		Defaults: project.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
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

	page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md", Limit: change.MaxReadLimit})
	require.NoError(tb, err)
	fr, err := model.ParseEditionKey("fr")
	require.NoError(tb, err)
	var set change.Set
	for _, b := range page.Blocks {
		at := b.Ref
		at.Edition = fr
		set.Ops = append(set.Ops, setTo(at, model.AbsentRevision, "FR "+b.Text))
	}
	res, err := svc.Apply(ctx, set, changePerson)
	require.NoError(tb, err)
	require.Equal(tb, change.SetApplied, res.Status)
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
