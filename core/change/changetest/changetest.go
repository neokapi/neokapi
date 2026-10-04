// Package changetest is the conformance suite every home of the change
// service passes: one table of change sets, run against a service over the
// home under test, asserting what a sender of the contract relies on whatever
// keeps the text.
//
// A home's test builds an Env (a service over the home, two documents it
// holds, a way to see what the home holds, and a point between a stage and
// its commit where the suite runs a second sender) and calls Run.
package changetest

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// Service is what the suite sends change sets to and reads documents
// through: a *change.Service, or a surface that carries the contract to one,
// such as an application binding that takes and returns its JSON.
type Service interface {
	Read(ctx context.Context, q change.ReadRequest) (*change.Page, error)
	Apply(ctx context.Context, set change.Set, actor change.Actor) (*change.Result, error)
}

// Env is a home under test.
type Env struct {
	// Service applies change sets to documents in the home.
	Service Service
	// DocA holds at least two editable blocks; DocB at least one.
	DocA, DocB string
	// Translated holds an editable block that has, or can be given, a French
	// translation: a document whose translations the home keeps, or a
	// bilingual document holding one. Empty is DocA.
	Translated string
	// TranslationFile is the document whose bytes hold Translated's French
	// translation when the home keeps it apart from Translated (a catalog
	// whose translation is the catalog its target template names). The
	// suite then asserts that the translation's writes land there and that
	// Translated keeps its bytes. Empty is Translated.
	TranslationFile string
	// Translations are more documents the removal case runs on, as it runs
	// on Translated, for a home that keeps translations in more than one
	// way: in a bilingual catalog, and in a file of a one-language format.
	Translations []Translation
	// Snapshot returns what the home holds for a document, byte for byte,
	// so the suite can tell that a refusal or a preview wrote nothing.
	Snapshot func(t *testing.T, doc string) []byte
	// Mode returns the file mode a document has, for a home that keeps
	// files. Nil skips the check that a write keeps it.
	Mode func(t *testing.T, doc string) os.FileMode
	// SetBeforeSettle installs fn, which the home calls once a document of a
	// change set is staged and before its commit lock is taken; nil removes
	// it. The suite runs a second sender there, between the first one's
	// stage and its commit. Nil skips the cases that interleave two senders.
	SetBeforeSettle func(fn func(doc string))
}

// Translation is a document whose French translation the removal case makes
// and removes: Doc as Env.Translated and File as Env.TranslationFile.
type Translation struct {
	Doc, File string
}

// person is the actor every change set of the suite is sent as.
var person = change.Actor{Kind: change.ActorPerson, Name: "conformance"}

// Run runs the suite. newEnv builds a fresh Env for each case.
func Run(t *testing.T, newEnv func(t *testing.T) Env) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, env Env)
	}{
		{"an edit lands and reads back", editLands},
		{"a replayed change set is stale, carries the current content and writes nothing", staleWritesNothing},
		{"edits to different blocks commute", differentBlocksCommute},
		{"an edit that lands between another's stage and commit is kept", interleavedEditsCommute},
		{"a refusal in one document leaves every document as it was", allOrNothingAcrossDocuments},
		{"a refusal found at commit leaves every document as it was", refusalAtCommit},
		{"a preview writes nothing", previewWritesNothing},
		{"a block no document holds is not found and nothing is written", missingBlock},
		{"the same edition said again is unchanged", unchangedIsIdempotent},
		{"a write keeps the file mode", modeKept},
		{"a removed translation reads back absent, a replay is stale, and it is created again", removedEdition},
		{"a change set with no operation applies and writes nothing", emptyWritesNothing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, newEnv(t))
		})
	}
}

// blocks reads every block of doc.
func blocks(t *testing.T, env Env, doc string) []change.BlockRead {
	t.Helper()
	page, err := env.Service.Read(context.Background(), change.ReadRequest{Doc: doc, Limit: change.MaxReadLimit})
	require.NoError(t, err)
	var out []change.BlockRead
	for _, b := range page.Blocks {
		if len(b.Ops) > 0 && strings.TrimSpace(b.Text) != "" {
			out = append(out, b)
		}
	}
	return out
}

// editable returns n editable blocks of doc.
func editable(t *testing.T, env Env, doc string, n int) []change.BlockRead {
	t.Helper()
	bs := blocks(t, env, doc)
	require.GreaterOrEqual(t, len(bs), n, "%s holds fewer than %d editable blocks", doc, n)
	return bs[:n]
}

// setText is a set_content of b to text, guarded by rev.
func setText(b change.BlockRead, rev, text string) change.Op {
	return change.Op{Kind: change.KindSetContent, At: b.Ref, IfMatch: rev, Body: &change.SetContent{Text: &text}}
}

func apply(t *testing.T, env Env, set change.Set) *change.Result {
	t.Helper()
	res, err := env.Service.Apply(context.Background(), set, person)
	require.NoError(t, err)
	return res
}

// textOf reads the text of the block keyed key in doc.
func textOf(t *testing.T, env Env, doc, key string) string {
	t.Helper()
	page, err := env.Service.Read(context.Background(), change.ReadRequest{Doc: doc, Blocks: []string{key}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1, "%s holds one block keyed %s", doc, key)
	return page.Blocks[0].Text
}

func editLands(t *testing.T, env Env) {
	b := editable(t, env, env.DocA, 1)[0]
	want := b.Text + " edited"
	res := apply(t, env, change.Set{Ops: []change.Op{setText(b, b.Rev, want)}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, change.OpApplied, res.Ops[0].Status)
	assert.Equal(t, b.Rev, res.Ops[0].Before)
	assert.NotEqual(t, b.Rev, res.Ops[0].After)
	// The result lists every file the document's change read; the edit
	// landed in one of them, which is the document's own file unless the
	// reference names the file of one edition.
	written := slices.IndexFunc(res.Docs, func(d change.DocResult) bool { return d.Written })
	require.GreaterOrEqual(t, written, 0, "the result names the file the edit was written to: %+v", res.Docs)
	d := res.Docs[written]
	require.NotNil(t, d.After)
	assert.NotEqual(t, d.Before, *d.After, "the digests around the write differ")
	assert.Equal(t, want, textOf(t, env, env.DocA, b.Ref.Block))
}

func staleWritesNothing(t *testing.T, env Env) {
	b := editable(t, env, env.DocA, 1)[0]
	set := change.Set{Ops: []change.Op{setText(b, b.Rev, b.Text+" first")}}
	first := apply(t, env, set)
	require.Equal(t, change.SetApplied, first.Status, "%+v", first.Ops)
	before := env.Snapshot(t, env.DocA)

	set.Ops[0] = setText(b, b.Rev, b.Text+" second")
	res := apply(t, env, set)
	require.Equal(t, change.SetRefused, res.Status)
	op := res.Ops[0]
	require.Equal(t, change.OpRefused, op.Status)
	require.NotNil(t, op.Error)
	assert.Equal(t, change.CodeStale, op.Error.Code)
	require.NotNil(t, op.Current, "a stale refusal carries the current content")
	assert.Equal(t, first.Ops[0].After, op.Current.Rev)
	assert.Equal(t, b.Text+" first", op.Current.Text)
	assert.Equal(t, before, env.Snapshot(t, env.DocA), "a refusal writes nothing")
}

func differentBlocksCommute(t *testing.T, env Env) {
	bs := editable(t, env, env.DocA, 2)
	one, two := bs[0], bs[1]
	r1 := apply(t, env, change.Set{Ops: []change.Op{setText(one, one.Rev, one.Text+" one")}})
	require.Equal(t, change.SetApplied, r1.Status, "%+v", r1.Ops)
	// The second sender read the document before the first edit landed.
	r2 := apply(t, env, change.Set{Ops: []change.Op{setText(two, two.Rev, two.Text+" two")}})
	require.Equal(t, change.SetApplied, r2.Status, "%+v", r2.Ops)
	assert.Equal(t, one.Text+" one", textOf(t, env, env.DocA, one.Ref.Block))
	assert.Equal(t, two.Text+" two", textOf(t, env, env.DocA, two.Ref.Block))
}

// interleave makes the home run second, once, after the next change set is
// staged and before it takes its commit lock, and reports whether it ran.
func interleave(t *testing.T, env Env, second func()) *bool {
	t.Helper()
	if env.SetBeforeSettle == nil {
		t.Skip("the home has no point between a stage and its commit to hold a sender at")
	}
	ran := new(bool)
	env.SetBeforeSettle(func(string) {
		if *ran {
			return
		}
		*ran = true
		env.SetBeforeSettle(nil)
		second()
	})
	t.Cleanup(func() { env.SetBeforeSettle(nil) })
	return ran
}

func interleavedEditsCommute(t *testing.T, env Env) {
	bs := editable(t, env, env.DocA, 2)
	one, two := bs[0], bs[1]
	var second *change.Result
	ran := interleave(t, env, func() {
		second = apply(t, env, change.Set{Ops: []change.Op{setText(two, two.Rev, two.Text+" two")}})
	})
	// The first sender stages against the content both read; the second
	// lands before it commits.
	first := apply(t, env, change.Set{Ops: []change.Op{setText(one, one.Rev, one.Text+" one")}})
	require.True(t, *ran, "the second sender ran between the first one's stage and commit")
	require.Equal(t, change.SetApplied, second.Status, "%+v", second.Ops)
	require.Equal(t, change.SetApplied, first.Status, "%+v", first.Ops)
	assert.Equal(t, one.Text+" one", textOf(t, env, env.DocA, one.Ref.Block))
	assert.Equal(t, two.Text+" two", textOf(t, env, env.DocA, two.Ref.Block), "the edit that landed first is kept")
}

func refusalAtCommit(t *testing.T, env Env) {
	a := editable(t, env, env.DocA, 1)[0]
	b := editable(t, env, env.DocB, 1)[0]
	beforeA := env.Snapshot(t, env.DocA)
	ran := interleave(t, env, func() {
		res := apply(t, env, change.Set{Ops: []change.Op{setText(b, b.Rev, b.Text+" first")}})
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	})
	res := apply(t, env, change.Set{Ops: []change.Op{
		setText(a, a.Rev, a.Text+" lands only with the other"),
		setText(b, b.Rev, b.Text+" second"),
	}})
	require.True(t, *ran)
	require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Ops[1].Error)
	assert.Equal(t, change.CodeStale, res.Ops[1].Error.Code, "the edit the other sender overtook is stale")
	assert.Equal(t, change.OpNotApplied, res.Ops[0].Status)
	assert.Equal(t, beforeA, env.Snapshot(t, env.DocA), "the document whose edit still applied is unchanged")
	assert.Equal(t, b.Text+" first", textOf(t, env, env.DocB, b.Ref.Block))
}

func allOrNothingAcrossDocuments(t *testing.T, env Env) {
	a := editable(t, env, env.DocA, 1)[0]
	b := editable(t, env, env.DocB, 1)[0]
	beforeA, beforeB := env.Snapshot(t, env.DocA), env.Snapshot(t, env.DocB)
	stale := "r:0000000000000000"
	res := apply(t, env, change.Set{Ops: []change.Op{
		setText(a, a.Rev, a.Text+" lands only with the other"),
		setText(b, stale, b.Text+" stale"),
	}})
	require.Equal(t, change.SetRefused, res.Status)
	assert.Equal(t, change.OpNotApplied, res.Ops[0].Status)
	require.NotNil(t, res.Ops[0].BlockedBy)
	assert.Equal(t, 1, *res.Ops[0].BlockedBy)
	assert.Equal(t, change.OpRefused, res.Ops[1].Status)
	assert.Equal(t, change.CodeStale, res.Ops[1].Error.Code)
	assert.Equal(t, beforeA, env.Snapshot(t, env.DocA), "the document whose edit was valid is unchanged")
	assert.Equal(t, beforeB, env.Snapshot(t, env.DocB))
}

func previewWritesNothing(t *testing.T, env Env) {
	b := editable(t, env, env.DocA, 1)[0]
	before := env.Snapshot(t, env.DocA)
	res := apply(t, env, change.Set{Mode: change.ModePreview, Ops: []change.Op{setText(b, b.Rev, b.Text+" previewed")}})
	require.Equal(t, change.SetPreviewed, res.Status, "%+v", res.Ops)
	assert.Equal(t, change.OpPreviewed, res.Ops[0].Status)
	assert.NotEqual(t, res.Ops[0].Before, res.Ops[0].After, "a preview computes the revision the edit would make")
	require.NotEmpty(t, res.Docs)
	assert.False(t, res.Docs[0].Written)
	assert.Nil(t, res.Record)
	assert.Equal(t, before, env.Snapshot(t, env.DocA), "a preview writes nothing")
	assert.Equal(t, b.Text, textOf(t, env, env.DocA, b.Ref.Block))
}

func missingBlock(t *testing.T, env Env) {
	b := editable(t, env, env.DocA, 1)[0]
	before := env.Snapshot(t, env.DocA)
	ghost := b
	ghost.Ref.Block = b.Ref.Block + "-missing"
	res := apply(t, env, change.Set{Ops: []change.Op{
		setText(b, b.Rev, b.Text+" never"),
		setText(ghost, b.Rev, "ghost"),
	}})
	require.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[1].Error)
	assert.Equal(t, change.CodeNotFound, res.Ops[1].Error.Code)
	assert.Equal(t, change.OpNotApplied, res.Ops[0].Status)
	assert.Equal(t, before, env.Snapshot(t, env.DocA))
}

func unchangedIsIdempotent(t *testing.T, env Env) {
	b := editable(t, env, env.DocA, 1)[0]
	before := env.Snapshot(t, env.DocA)
	res := apply(t, env, change.Set{Ops: []change.Op{setText(b, b.Rev, b.Text)}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, change.OpUnchanged, res.Ops[0].Status)
	assert.Equal(t, b.Rev, res.Ops[0].After)
	for _, d := range res.Docs {
		assert.False(t, d.Written, "nothing changed, so nothing is written")
	}
	assert.Equal(t, before, env.Snapshot(t, env.DocA))
}

// translation is the language of the translation the suite makes and removes.
var translation = model.EditionKey{Locale: "fr"}

// readEdition reads the block keyed key in doc with its French translation.
func readEdition(t *testing.T, env Env, doc, key string) change.BlockRead {
	t.Helper()
	page, err := env.Service.Read(context.Background(), change.ReadRequest{Doc: doc, Blocks: []string{key}, Editions: []model.EditionKey{translation}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1, "%s holds one block keyed %s", doc, key)
	return page.Blocks[0]
}

func removedEdition(t *testing.T, env Env) {
	doc := env.Translated
	if doc == "" {
		doc = env.DocA
	}
	if len(env.Translations) == 0 {
		removeTranslation(t, env, Translation{Doc: doc, File: env.TranslationFile})
		return
	}
	for _, tr := range append([]Translation{{Doc: doc, File: env.TranslationFile}}, env.Translations...) {
		t.Run(tr.Doc, func(t *testing.T) { removeTranslation(t, env, tr) })
	}
}

// removeTranslation makes a French translation of the first editable block
// of tr.Doc where it has none, removes it, checks that a replay of the
// removal is stale and a removal of what is not there changes nothing, and
// creates the translation again.
func removeTranslation(t *testing.T, env Env, tr Translation) {
	doc := tr.Doc
	holder := tr.File
	if holder == "" {
		holder = doc
	}
	docBefore := env.Snapshot(t, doc)
	b := editable(t, env, doc, 1)[0]
	at := b.Ref
	at.Edition = translation
	fr, held := readEdition(t, env, doc, b.Ref.Block).Editions["fr"]
	if !held {
		res := apply(t, env, change.Set{Ops: []change.Op{setText(change.BlockRead{Ref: at}, model.AbsentRevision, "Traduction")}})
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		fr, held = readEdition(t, env, doc, b.Ref.Block).Editions["fr"]
		require.True(t, held, "the translation the suite made reads back")
	}

	translated := env.Snapshot(t, holder)
	remove := change.Op{Kind: change.KindRemoveEdition, At: at, IfMatch: fr.Rev, Body: &change.RemoveEdition{}}
	res := apply(t, env, change.Set{Ops: []change.Op{remove}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, change.OpApplied, res.Ops[0].Status)
	assert.Equal(t, fr.Rev, res.Ops[0].Before)
	assert.Equal(t, model.AbsentRevision, res.Ops[0].After)
	got := readEdition(t, env, doc, b.Ref.Block)
	_, held = got.Editions["fr"]
	assert.False(t, held, "the removed translation reads back absent")
	assert.Equal(t, b.Text, got.Text, "the block's own edition is untouched")
	assert.NotEqual(t, translated, env.Snapshot(t, holder), "the removal is written to %s, which holds the translation", holder)
	if holder != doc {
		assert.Equal(t, docBefore, env.Snapshot(t, doc), "%s keeps its bytes: its translation lives in %s", doc, holder)
	}

	snapshot := env.Snapshot(t, holder)
	res = apply(t, env, change.Set{Ops: []change.Op{remove}})
	require.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code, "the revision the removal named no longer holds")
	require.NotNil(t, res.Ops[0].Current)
	assert.Equal(t, model.AbsentRevision, res.Ops[0].Current.Rev)

	remove.IfMatch = change.AnyRevision
	res = apply(t, env, change.Set{Ops: []change.Op{remove}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, change.OpUnchanged, res.Ops[0].Status, "removing a translation the block does not hold changes nothing")
	assert.Equal(t, snapshot, env.Snapshot(t, holder), "neither the refusal nor the removal that changed nothing writes")
	if holder != doc {
		assert.Equal(t, docBefore, env.Snapshot(t, doc), "%s keeps its bytes: its translation lives in %s", doc, holder)
	}

	// The removed translation is created again as one the block never held.
	res = apply(t, env, change.Set{Ops: []change.Op{setText(change.BlockRead{Ref: at}, model.AbsentRevision, "Traduction nouvelle")}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, model.AbsentRevision, res.Ops[0].Before)
	again, held := readEdition(t, env, doc, b.Ref.Block).Editions["fr"]
	require.True(t, held, "the translation created again reads back")
	assert.Equal(t, "Traduction nouvelle", again.Text)
	assert.Equal(t, res.Ops[0].After, again.Rev)
	if holder != doc {
		assert.Equal(t, docBefore, env.Snapshot(t, doc), "%s keeps its bytes: its translation lives in %s", doc, holder)
	}
}

func emptyWritesNothing(t *testing.T, env Env) {
	before, beforeB := env.Snapshot(t, env.DocA), env.Snapshot(t, env.DocB)
	res := apply(t, env, change.Set{Ops: []change.Op{}})
	require.Equal(t, change.SetApplied, res.Status)
	assert.Empty(t, res.Ops)
	assert.Nil(t, res.Record, "nothing is recorded")
	for _, d := range res.Docs {
		assert.False(t, d.Written)
	}
	assert.Equal(t, before, env.Snapshot(t, env.DocA))
	assert.Equal(t, beforeB, env.Snapshot(t, env.DocB))
}

func modeKept(t *testing.T, env Env) {
	if env.Mode == nil {
		t.Skip("the home keeps no files")
	}
	b := editable(t, env, env.DocA, 1)[0]
	mode := env.Mode(t, env.DocA)
	res := apply(t, env, change.Set{Ops: []change.Op{setText(b, b.Rev, b.Text+" moded")}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, mode, env.Mode(t, env.DocA))
}
