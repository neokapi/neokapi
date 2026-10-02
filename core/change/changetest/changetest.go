// Package changetest is the conformance suite every home of the change
// service passes: one table of change sets, run against a service over the
// home under test, asserting what a sender of the contract relies on whatever
// keeps the text.
//
// A home's test builds an Env (a service over the home, two documents it
// holds, and a way to see what the home holds) and calls Run.
package changetest

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

// Env is a home under test.
type Env struct {
	// Service applies change sets to documents in the home.
	Service *change.Service
	// DocA holds at least two editable blocks; DocB at least one.
	DocA, DocB string
	// Snapshot returns what the home holds for a document, byte for byte,
	// so the suite can tell that a refusal or a preview wrote nothing.
	Snapshot func(t *testing.T, doc string) []byte
	// Mode returns the file mode a document has, for a home that keeps
	// files. Nil skips the check that a write keeps it.
	Mode func(t *testing.T, doc string) os.FileMode
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
		{"a refusal in one document leaves every document as it was", allOrNothingAcrossDocuments},
		{"a preview writes nothing", previewWritesNothing},
		{"a block no document holds is not found and nothing is written", missingBlock},
		{"the same edition said again is unchanged", unchangedIsIdempotent},
		{"a write keeps the file mode", modeKept},
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
	require.NotEmpty(t, res.Docs)
	assert.True(t, res.Docs[0].Written)
	require.NotNil(t, res.Docs[0].After)
	assert.NotEqual(t, res.Docs[0].Before, *res.Docs[0].After, "the digests around the write differ")
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
