package changes_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/changes"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changetest"
	"github.com/neokapi/neokapi/core/model"
)

func TestMain(m *testing.M) {
	pgtest.UseTemplate(func(db *storage.PgDB) error { return migrations.Apply(db, nil) })
	os.Exit(m.Run())
}

// fixture is a project on a real PostgreSQL store with two items.
type fixture struct {
	store   *bstore.PostgresStore
	project *platstore.Project
	home    *changes.Home
	svc     *change.Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	cs, err := bstore.NewPostgresStoreFromDB(pgtest.NewTestDB(t))
	require.NoError(t, err)
	ctx := t.Context()
	p := &platstore.Project{Name: "Stream home", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr", "de"}}
	require.NoError(t, cs.CreateProject(ctx, p))
	seed := func(item string, blocks ...*model.Block) {
		require.NoError(t, cs.StoreItem(ctx, p.ID, "main", &platstore.Item{Name: item, Format: "json"}))
		require.NoError(t, cs.StoreBlocksForItem(ctx, p.ID, "main", item, blocks))
	}
	one := named("one", "First")
	one.SetTargetText("fr", "Premier")
	seed("a.json", one, named("two", "Second"))
	seed("b.json", named("three", "Third"))
	home := &changes.Home{Store: cs, ProjectID: p.ID, Stream: "main", SourceLocale: "en", Locales: p.TargetLanguages}
	return &fixture{store: cs, project: p, home: home, svc: changes.NewService(home, nil)}
}

func named(name, text string) *model.Block {
	b := model.NewBlock(name, text)
	b.Name = name
	return b
}

// snapshot is what the stream holds for an item, edition by edition.
func (f *fixture) snapshot(t *testing.T, item string) []byte {
	t.Helper()
	rows, err := f.store.ItemBlocks(t.Context(), f.project.ID, "main", item, nil)
	require.NoError(t, err)
	var b strings.Builder
	for _, sb := range rows {
		var lines []string
		for k, e := range sb.Block.EachEdition {
			text, _ := k.MarshalText()
			lines = append(lines, fmt.Sprintf("%s@%s=%s [%s]", sb.SourceID, text, model.RunsEditText(e.Runs), e.Status))
		}
		slices.Sort(lines)
		b.WriteString(strings.Join(lines, "\n"))
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func TestStreamHome_Conformance(t *testing.T) {
	changetest.Run(t, func(t *testing.T) changetest.Env {
		f := newFixture(t)
		return changetest.Env{
			Service:  f.svc,
			DocA:     "a.json",
			DocB:     "b.json",
			Snapshot: f.snapshot,
			SetBeforeSettle: func(fn func(string)) {
				f.home.BeforeLock = fn
			},
		}
	})
}

var person = change.Actor{Kind: change.ActorPerson, Name: "ada"}

func read(t *testing.T, svc *change.Service, doc, key string) change.BlockRead {
	t.Helper()
	page, err := svc.Read(t.Context(), change.ReadRequest{Doc: doc, Blocks: []string{key}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	return page.Blocks[0]
}

func setText(at change.Ref, rev, text string) change.Op {
	return change.Op{Kind: change.KindSetContent, At: at, IfMatch: rev, Body: &change.SetContent{Text: &text}}
}

// A translation edited through the stream home lands on its row, with the
// history row and the change-log row the transaction writes beside it, under
// the correlation id the record is named by.
func TestStreamHome_ATranslationLandsWithItsHistory(t *testing.T) {
	f := newFixture(t)
	ctx, corr := changes.WithChange(t.Context(), "")
	b := read(t, f.svc, "a.json", "one")
	fr := b.Editions["fr"]
	require.Equal(t, "Premier", fr.Text)
	at := b.Ref
	at.Edition = model.EditionKey{Locale: "fr"}

	svc := changes.NewService(f.home, nil, change.WithRecorder(changes.Recorder{ProjectID: f.project.ID, Stream: "main"}))
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setText(at, fr.Rev, "Le premier")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Record)
	assert.Equal(t, corr, *res.Record)
	assert.Equal(t, "stream:main", res.Docs[0].Home)

	rows, err := f.store.ItemBlocks(ctx, f.project.ID, "main", "a.json", []string{"one"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	target := rows[0].Block.Target("fr")
	require.NotNil(t, target)
	assert.Equal(t, "Le premier", model.RunsText(target.Runs))
	assert.Equal(t, model.TargetStatusTranslated, target.Status, "a person's edit leaves the translation translated")
	assert.Equal(t, model.OriginHuman, target.Origin.Kind)

	history, err := f.store.GetBlockHistory(ctx, f.project.ID, "main", rows[0].Block.ID, "fr", 10)
	require.NoError(t, err)
	require.NotEmpty(t, history)
	assert.Equal(t, "Le premier", history[0].Text)
	assert.Equal(t, corr, history[0].CorrelationID)

	feed, err := f.store.GetChanges(ctx, f.project.ID, "main", 0, []string{"fr"}, 100)
	require.NoError(t, err)
	assert.True(t, slices.ContainsFunc(feed.Changes, func(c platstore.ChangeEntry) bool {
		return c.BlockID == rows[0].Block.ID && c.ChangeType == "target_modified" && c.Locale == "fr"
	}), "the change log names the edited translation")
}

// An edition in a language the project does not translate into has no home in
// the stream.
func TestStreamHome_AnEditionTheProjectHasNoLanguageForIsRefused(t *testing.T) {
	f := newFixture(t)
	b := read(t, f.svc, "a.json", "two")
	at := b.Ref
	at.Edition = model.EditionKey{Locale: "ja"}
	res, err := f.svc.Apply(t.Context(), change.Set{Ops: []change.Op{setText(at, model.AbsentRevision, "二番目")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	assert.Contains(t, res.Ops[0].Error.Message, "does not translate into ja")
}

// An item the stream does not hold is not found.
func TestStreamHome_AnItemTheStreamDoesNotHoldIsNotFound(t *testing.T) {
	f := newFixture(t)
	res, err := f.svc.Apply(context.Background(), change.Set{Ops: []change.Op{
		setText(change.Ref{Doc: "missing.json", Block: "one"}, "r:0000000000000000", "x"),
	}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status)
	assert.Equal(t, change.CodeNotFound, res.Ops[0].Error.Code)
}

// A block is addressed by its durable key, its name or its row id, and each
// reaches the same row.
func TestStreamHome_ABlockIsAddressedByKeyNameOrRowID(t *testing.T) {
	f := newFixture(t)
	rows, err := f.store.ItemBlocks(t.Context(), f.project.ID, "main", "a.json", []string{"two"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	b := read(t, f.svc, "a.json", rows[0].Block.ID)
	assert.Equal(t, "two", b.Ref.Block, "a read names the block by its durable key")
	res, err := f.svc.Apply(t.Context(), change.Set{Ops: []change.Op{
		setText(change.Ref{Doc: "a.json", Block: rows[0].Block.ID}, b.Rev, "Second, edited"),
	}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, "two", res.Ops[0].At.Block)
	assert.Equal(t, "Second, edited", read(t, f.svc, "a.json", "two").Text)
}

// A translation the change removes leaves the stream: its row goes, and the
// block's history and the stream's change log record the removal.
func TestStreamHome_ARemovedTranslationLeavesTheStream(t *testing.T) {
	f := newFixture(t)
	ctx, corr := changes.WithChange(t.Context(), "")
	b := read(t, f.svc, "a.json", "one")
	at := b.Ref
	at.Edition = model.EditionKey{Locale: "fr"}
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindRemoveEdition, At: at, IfMatch: b.Editions["fr"].Rev, Body: &change.RemoveEdition{},
	}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	_, held := read(t, f.svc, "a.json", "one").Editions["fr"]
	assert.False(t, held, "the stream holds no French translation of the block")
	rows, err := f.store.ItemBlocks(ctx, f.project.ID, "main", "a.json", []string{"one"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Nil(t, rows[0].Block.Target("fr"))

	history, err := f.store.GetBlockHistory(ctx, f.project.ID, "main", rows[0].Block.ID, "fr", 10)
	require.NoError(t, err)
	require.NotEmpty(t, history)
	assert.Equal(t, "target_removed", history[0].ChangeType)
	assert.Empty(t, history[0].Text)
	assert.Equal(t, corr, history[0].CorrelationID)

	feed, err := f.store.GetChanges(ctx, f.project.ID, "main", 0, []string{"fr"}, 100)
	require.NoError(t, err)
	assert.True(t, slices.ContainsFunc(feed.Changes, func(c platstore.ChangeEntry) bool {
		return c.BlockID == rows[0].Block.ID && c.ChangeType == "target_removed" && c.Locale == "fr"
	}), "the change log names the removed translation")
}
