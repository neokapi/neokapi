package host

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projector"
)

// In a project a message catalog's translations are files of their own, and a
// key added to or removed from the source catalog reaches them through the
// change service.
func TestChangeService_AddsAndRemovesKeysAcrossACatalogsTranslations(t *testing.T) {
	item := project.ContentItem{Path: "locales/en.json", Target: "locales/{lang}.json"}
	a, recipe := changeProject(t, item, map[string]string{
		"locales/en.json": "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old\"\n  }\n}\n",
		"locales/de.json": "{\n  \"nav\": {\n    \"home\": \"Start\",\n    \"cart\": \"Warenkorb\",\n    \"legacy\": \"Alt\"\n  }\n}\n",
	})
	svc := changeService(t, a, recipe)
	ctx := context.Background()

	d, err := svc.Describe(ctx, change.DescribeRequest{Doc: "locales/en.json"})
	require.NoError(t, err)
	assert.NotNil(t, d.Ops.InsertBlock, "describe reports what the JSON writer writes")
	assert.NotNil(t, d.Ops.DeleteBlock)

	text := func(s string) *string { return &s }
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindInsertBlock, At: change.Ref{Doc: "locales/en.json"},
		Body: &change.InsertBlock{After: "nav.cart", Name: "nav.checkout", Editions: map[string]change.Content{
			"en": {Text: text("Checkout")}, "de": {Text: text("Kasse")}, "fr": {Text: text("Paiement")},
		}},
	}}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"checkout\": \"Checkout\",\n    \"legacy\": \"Old\"\n  }\n}\n", readFile(t, recipe, "locales/en.json"))
	assert.Equal(t, "{\n  \"nav\": {\n    \"home\": \"Start\",\n    \"cart\": \"Warenkorb\",\n    \"checkout\": \"Kasse\",\n    \"legacy\": \"Alt\"\n  }\n}\n", readFile(t, recipe, "locales/de.json"))
	assert.Equal(t, "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"checkout\": \"Paiement\",\n    \"legacy\": \"Old\"\n  }\n}\n", readFile(t, recipe, "locales/fr.json"),
		"a translation with no file yet is written from the source, the new key translated")

	// The removal names the revision of the source catalog's own edition,
	// and the translations go with the key.
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "locales/en.json", Blocks: []string{"nav.legacy"}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	legacy := page.Blocks[0]
	res, err = svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindDeleteBlock, At: legacy.Ref,
		Body: &change.DeleteBlock{IfMatch: map[string]string{"en": legacy.Rev}},
	}}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, "{\n  \"nav\": {\n    \"home\": \"Start\",\n    \"cart\": \"Warenkorb\",\n    \"checkout\": \"Kasse\"\n  }\n}\n", readFile(t, recipe, "locales/de.json"))
	assert.Equal(t, "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"checkout\": \"Paiement\"\n  }\n}\n", readFile(t, recipe, "locales/fr.json"))

	res, err = svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindInsertBlock, At: change.Ref{Doc: "locales/de.json"},
		Body: &change.InsertBlock{Name: "nav.x", Editions: map[string]change.Content{"de": {Text: text("X")}}},
	}}}, changePerson)
	require.NoError(t, err)
	assert.Equal(t, change.CodeInvalid, res.Ops[0].Error.Code, "a block is added through the source catalog: %s", res.Ops[0].Error.Message)
}

// A JSON catalog configured to read a block's note from the member beside it
// (noteRules, as a Chrome extension's messages.json is read) makes that member
// part of the block's shell. Its writer adds and removes no blocks, so
// describing the document reports neither operation and a removal is refused.
func TestChangeService_RefusesStructureWhereNotesAreReadFromBesideABlock(t *testing.T) {
	const catalog = "{\n  \"greeting\": {\n    \"message\": \"Hello\",\n    \"description\": \"Shown on the home page\"\n  },\n  \"cart\": {\n    \"message\": \"Cart\"\n  }\n}\n"
	item := project.ContentItem{Path: "locales/en.json", Format: &project.FormatSpec{Name: "json", Config: map[string]any{"noteRules": "^description$"}}}
	a, recipe := changeProject(t, item, map[string]string{"locales/en.json": catalog})
	svc := changeService(t, a, recipe)
	ctx := context.Background()

	d, err := svc.Describe(ctx, change.DescribeRequest{Doc: "locales/en.json"})
	require.NoError(t, err)
	assert.Nil(t, d.Ops.InsertBlock)
	assert.Nil(t, d.Ops.DeleteBlock)

	page, err := svc.Read(ctx, change.ReadRequest{Doc: "locales/en.json"})
	require.NoError(t, err)
	require.NotEmpty(t, page.Blocks)
	b := page.Blocks[0]
	assert.NotContains(t, b.Ops, change.KindDeleteBlock)
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindDeleteBlock, At: b.Ref, Body: &change.DeleteBlock{IfMatch: map[string]string{"en": b.Rev}},
	}}}, changePerson)
	require.NoError(t, err)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code, res.Ops[0].Error.Message)
	assert.Equal(t, catalog, readFile(t, recipe, "locales/en.json"))
}

// keyedCatalog is a project whose English catalog has translations in German
// and French files of their own.
func keyedCatalog(t *testing.T, name string) (*App, string) {
	t.Helper()
	item := project.ContentItem{Path: "locales/en.json", Target: "locales/{lang}.json"}
	return changeProject(t, item, map[string]string{
		"locales/en.json": "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"legacy\": \"Old\"\n  }\n}\n",
		"locales/de.json": "{\n  \"nav\": {\n    \"home\": \"Start\",\n    \"legacy\": \"Alt\"\n  }\n}\n",
		"locales/fr.json": "{\n  \"nav\": {\n    \"home\": \"Accueil\",\n    \"legacy\": \"Ancien\"\n  }\n}\n",
	}, func(p *project.KapiProject) { p.ID = projectIDFor("keyedcatalog" + name) })
}

// readLegacy reads nav.legacy with its translations.
func readLegacy(t *testing.T, svc *change.Service) change.BlockRead {
	t.Helper()
	page, err := svc.Read(t.Context(), change.ReadRequest{Doc: "locales/en.json", Blocks: []string{"nav.legacy"},
		Editions: []model.EditionKey{{Locale: "de"}, {Locale: "fr"}}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	return page.Blocks[0]
}

// spyCheck hands the host's commit check every change it is given and keeps
// a copy.
type spyCheck struct {
	inner change.CommitCheck
	mu    sync.Mutex
	seen  []change.EditionChange
}

func (s *spyCheck) Check(ctx context.Context, changes []change.EditionChange) ([]change.CheckOutcome, string, error) {
	s.mu.Lock()
	s.seen = append(s.seen, changes...)
	s.mu.Unlock()
	return s.inner.Check(ctx, changes)
}

// delete_block reaches the host's commit check with nothing after the change
// for every edition of the deleted block, the source catalog's and each
// translation's (AfterRev is model.AbsentRevision), and the host service
// records the removal as one content.edit operation with a block-history row
// per edition.
func TestChangeService_DeleteBlockReachesTheCommitCheckAndTheRecord(t *testing.T) {
	t.Run("the commit check sees each edition removed", func(t *testing.T) {
		a, recipe := keyedCatalog(t, "check")
		ctx := t.Context()
		opts := ChangeServiceOptions{Project: recipe, Origin: "test"}
		h, err := a.changeHome(opts)
		require.NoError(t, err)
		spy := &spyCheck{inner: a.CommitCheck(projectCommand(ctx, "change", recipe))}
		svc := change.NewService(filehome.Formats{Registry: a.FormatReg},
			change.OneHome(filehome.New(h.layout, filehome.Options{LockDir: h.lockDir, PrepareLocks: h.prepare})),
			change.WithOrigin("test"), change.WithCommitCheck(spy))

		legacy := readLegacy(t, svc)
		res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{
			Kind: change.KindDeleteBlock, At: legacy.Ref, Body: &change.DeleteBlock{IfMatch: map[string]string{"en": legacy.Rev}},
		}}}, changePerson)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

		removed := map[string]change.EditionChange{}
		for _, ch := range spy.seen {
			require.Equal(t, "nav.legacy", ch.Ref.Block)
			loc := string(ch.Ref.Edition.Locale)
			if loc == "" {
				loc = "en" // the source catalog's own edition
			}
			removed[loc] = ch
		}
		require.Len(t, removed, 3, "the check sees the source and both translations: %+v", spy.seen)
		for loc, ch := range removed {
			assert.Equal(t, model.AbsentRevision, ch.AfterRev, "%s has nothing after the change", loc)
			assert.Nil(t, ch.After, loc)
			assert.NotEqual(t, model.AbsentRevision, ch.BeforeRev, "%s existed before", loc)
		}
		assert.Equal(t, legacy.Rev, removed["en"].BeforeRev)
		assert.Equal(t, legacy.Editions["de"].Rev, removed["de"].BeforeRev)
		assert.Equal(t, legacy.Editions["fr"].Rev, removed["fr"].BeforeRev)
	})

	t.Run("the host service records the removal", func(t *testing.T) {
		a, recipe := keyedCatalog(t, "record")
		root := filepath.Dir(recipe)
		ctx := t.Context()
		svc := changeService(t, a, recipe)
		legacy := readLegacy(t, svc)
		res, err := svc.Apply(ctx, change.Set{Note: "Retire the old link", Ops: []change.Op{{
			Kind: change.KindDeleteBlock, At: legacy.Ref, Body: &change.DeleteBlock{IfMatch: map[string]string{"en": legacy.Rev}},
		}}}, changePerson)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		assert.NotContains(t, readFile(t, recipe, "locales/fr.json"), "legacy")
		require.NotNil(t, res.Record, "a removal is recorded")

		ops := editOps(t, a, root)
		require.Len(t, ops, 1)
		assert.Equal(t, projector.KindEdit, ops[0].Kind)
		assert.Equal(t, *res.Record, ops[0].ID)
		e := editPayload(t, ops[0].Payload)
		assert.Equal(t, "Retire the old link", e.Note)
		after := map[string]string{}
		for _, tr := range e.Transitions {
			assert.Equal(t, "nav.legacy", tr.Block)
			after[tr.Edition] = tr.After
		}
		assert.Equal(t, map[string]string{"en": model.AbsentRevision, "de": model.AbsentRevision, "fr": model.AbsentRevision}, after)

		docs, err := a.DocumentIndex(ctx, root)
		require.NoError(t, err)
		db, err := a.ProjectDB(ctx, root)
		require.NoError(t, err)
		for loc, before := range map[string]string{"en": legacy.Rev, "de": legacy.Editions["de"].Rev, "fr": legacy.Editions["fr"].Rev} {
			row, found, err := db.History().LastWrite(ctx, docs.Key("locales/en.json"), "nav.legacy", loc)
			require.NoError(t, err)
			require.True(t, found, "%s: the removal is in the block history", loc)
			assert.Equal(t, *res.Record, row.Op)
			assert.Equal(t, before, row.Before, loc)
			assert.Equal(t, model.AbsentRevision, row.After, loc)
		}
	})
}
