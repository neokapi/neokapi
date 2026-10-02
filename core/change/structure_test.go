package change_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// newKVHome is a key-value catalog "c" in memory whose home adds and removes
// blocks.
func newKVHome(blocks ...memBlock) *memHome {
	h := newMemHome(map[string][]memBlock{"c": blocks})
	h.kv = true
	return h
}

func insertOp(doc, after, name string, editions map[string]string) change.Op {
	ed := map[string]change.Content{}
	for k, text := range editions {
		ed[k] = change.Content{Text: &text}
	}
	return change.Op{Kind: change.KindInsertBlock, At: change.Ref{Doc: doc}, Body: &change.InsertBlock{After: after, Name: name, Editions: ed}}
}

func insertBefore(doc, before, name, text string) change.Op {
	op := insertOp(doc, "", name, map[string]string{"en": text})
	op.Body.(*change.InsertBlock).Before = before
	return op
}

func deleteOp(doc, key string, revs map[string]string) change.Op {
	return change.Op{Kind: change.KindDeleteBlock, At: change.Ref{Doc: doc, Block: key}, Body: &change.DeleteBlock{IfMatch: revs}}
}

// keysOf lists the document's blocks in order.
func keysOf(t *testing.T, svc *change.Service, doc string) []string {
	t.Helper()
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: doc})
	require.NoError(t, err)
	var out []string
	for _, b := range page.Blocks {
		out = append(out, b.Ref.Block+"="+b.Text)
	}
	return out
}

func applySet(t *testing.T, svc *change.Service, actor change.Actor, ops ...change.Op) *change.Result {
	t.Helper()
	res, err := svc.Apply(context.Background(), change.Set{Ops: ops}, actor)
	require.NoError(t, err)
	return res
}

func TestService_DescribesStructureWhereTheFormatWritesIt(t *testing.T) {
	ctx := context.Background()
	svc := newMemService(newKVHome(textBlock("a", "A")))
	d, err := svc.Describe(ctx, change.DescribeRequest{Format: "memory-kv"})
	require.NoError(t, err)
	assert.NotNil(t, d.Ops[change.KindInsertBlock])
	assert.NotNil(t, d.Ops[change.KindDeleteBlock])
	assert.Contains(t, readBlock(t, svc, "c", "a").Ops, change.KindDeleteBlock, "a block of the catalog accepts delete_block")

	d, err = svc.Describe(ctx, change.DescribeRequest{Format: "memory"})
	require.NoError(t, err)
	assert.Nil(t, d.Ops[change.KindInsertBlock], "a format whose writer declares no structure refuses it")
	assert.Nil(t, d.Ops[change.KindDeleteBlock])
}

func TestService_InsertBlock(t *testing.T) {
	newHome := func() *memHome { return newKVHome(textBlock("x", "X"), textBlock("y", "Y")) }

	t.Run("a new block lands beside its anchor and reports its revision", func(t *testing.T) {
		h := newHome()
		svc := newMemService(h)
		res := applySet(t, svc, svcPerson, insertOp("c", "x", "n", map[string]string{"en": "New"}))
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops[0].Error)
		assert.Equal(t, []string{"x=X", "n=New", "y=Y"}, keysOf(t, svc, "c"))
		op := res.Ops[0]
		assert.Equal(t, change.OpApplied, op.Status)
		assert.Equal(t, &change.Ref{Doc: "c", Block: "n"}, op.At)
		assert.Equal(t, readBlock(t, svc, "c", "n").Rev, op.After, "the revision a read reports, to send as if_match next")
	})

	t.Run("before an anchor, and last with none", func(t *testing.T) {
		svc := newMemService(newHome())
		res := applySet(t, svc, svcPerson, insertBefore("c", "x", "first", "F"), insertOp("c", "", "last", map[string]string{"en": "L"}))
		require.Equal(t, change.SetApplied, res.Status)
		assert.Equal(t, []string{"first=F", "x=X", "y=Y", "last=L"}, keysOf(t, svc, "c"))
	})

	t.Run("each insert sees the blocks the ones before it added", func(t *testing.T) {
		svc := newMemService(newHome())
		res := applySet(t, svc, svcPerson,
			insertOp("c", "x", "a", map[string]string{"en": "A"}),
			insertOp("c", "a", "b", map[string]string{"en": "B"}),
			insertBefore("c", "a", "z", "Z"))
		require.Equal(t, change.SetApplied, res.Status)
		assert.Equal(t, []string{"x=X", "z=Z", "a=A", "b=B", "y=Y"}, keysOf(t, svc, "c"))
	})

	t.Run("a key a block already answers to is stale, with the block as it stands", func(t *testing.T) {
		h := newHome()
		svc := newMemService(h)
		before := h.snapshot("c")
		res := applySet(t, svc, svcPerson, insertOp("c", "x", "y", map[string]string{"en": "Again"}))
		err := requireRefused(t, res.Ops[0], change.CodeStale)
		assert.Equal(t, "name", err.Field)
		require.NotNil(t, res.Ops[0].Current)
		assert.Equal(t, "Y", res.Ops[0].Current.Text)
		assert.Equal(t, before, h.snapshot("c"), "nothing is written")
	})

	t.Run("an anchor no block answers to is not found, with the nearest keys", func(t *testing.T) {
		h := newKVHome(textBlock("nav.cart", "Cart"), textBlock("nav.home", "Home"))
		svc := newMemService(h)
		res := applySet(t, svc, svcPerson, insertOp("c", "nav.carts", "nav.checkout", map[string]string{"en": "Checkout"}))
		err := requireRefused(t, res.Ops[0], change.CodeNotFound)
		assert.Equal(t, "after", err.Field)
		require.NotEmpty(t, err.Candidates)
		assert.Equal(t, "nav.cart", err.Candidates[0].Key)
	})

	t.Run("the content is held to the rules of every content operation", func(t *testing.T) {
		svc := newMemService(newHome())
		res := applySet(t, svc, svcPerson, insertOp("c", "x", "n", map[string]string{"en": `Read <x id="1"/> now`}))
		err := requireRefused(t, res.Ops[0], change.CodeGuard)
		assert.Equal(t, change.SubcodeCodesChanged, err.Subcode, "a new block has no codes to name")
		assert.Equal(t, "editions/en", err.Field)
	})

	t.Run("the document's own edition is required", func(t *testing.T) {
		svc := newMemService(newHome())
		res := applySet(t, svc, svcPerson, insertOp("c", "x", "n", map[string]string{"en;channel=short": "N"}))
		err := requireRefused(t, res.Ops[0], change.CodeInvalid)
		assert.Equal(t, "editions", err.Field)
	})

	t.Run("an edition the document has no place for is unsupported", func(t *testing.T) {
		svc := newMemService(newHome())
		res := applySet(t, svc, svcPerson, insertOp("c", "x", "n", map[string]string{"en": "N", "fr": "N"}))
		err := requireRefused(t, res.Ops[0], change.CodeUnsupported)
		assert.Equal(t, "editions/fr", err.Field)
	})

	t.Run("the own edition named twice is invalid", func(t *testing.T) {
		svc := newMemService(newHome())
		res := applySet(t, svc, svcPerson, insertOp("c", "x", "n", map[string]string{"": "N", "en": "M"}))
		assert.Equal(t, "editions/en", requireRefused(t, res.Ops[0], change.CodeInvalid).Field)
	})

	t.Run("a block with no name is invalid", func(t *testing.T) {
		svc := newMemService(newHome())
		res := applySet(t, svc, svcPerson, insertOp("c", "x", "", map[string]string{"en": "N"}))
		assert.Equal(t, "name", requireRefused(t, res.Ops[0], change.CodeInvalid).Field)
	})

	t.Run("a preview writes nothing", func(t *testing.T) {
		h := newHome()
		svc := newMemService(h)
		before := h.snapshot("c")
		res, err := svc.Apply(context.Background(), change.Set{Mode: change.ModePreview, Ops: []change.Op{insertOp("c", "x", "n", map[string]string{"en": "N"})}}, svcPerson)
		require.NoError(t, err)
		assert.Equal(t, change.SetPreviewed, res.Status)
		assert.Equal(t, change.OpPreviewed, res.Ops[0].Status)
		assert.Equal(t, before, h.snapshot("c"))
	})
}

func TestService_DeleteBlock(t *testing.T) {
	newHome := func() *memHome {
		return newKVHome(textBlock("x", "X"), textBlock("y", "Y"), textBlock("t", "T", "en-GB", "Colour"))
	}

	t.Run("a block named with its revision is removed", func(t *testing.T) {
		h := newHome()
		svc := newMemService(h)
		rev := readBlock(t, svc, "c", "y").Rev
		res := applySet(t, svc, svcPerson, deleteOp("c", "y", map[string]string{"en": rev}))
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops[0].Error)
		assert.Equal(t, rev, res.Ops[0].Before)
		assert.Equal(t, &change.Ref{Doc: "c", Block: "y"}, res.Ops[0].At)
		assert.Equal(t, []string{"x=X", "t=T"}, keysOf(t, svc, "c"))
	})

	t.Run("a revision that moved is stale, with the block as it stands", func(t *testing.T) {
		h := newHome()
		svc := newMemService(h)
		before := h.snapshot("c")
		res := applySet(t, svc, svcPerson, deleteOp("c", "y", map[string]string{"en": "r:0000000000000000"}))
		err := requireRefused(t, res.Ops[0], change.CodeStale)
		assert.Equal(t, "if_match/en", err.Field)
		require.NotNil(t, res.Ops[0].Current)
		assert.Equal(t, "Y", res.Ops[0].Current.Text)
		assert.Equal(t, before, h.snapshot("c"))
	})

	t.Run("the own edition is named, and every edition named is checked", func(t *testing.T) {
		h := newHome()
		svc := newMemService(h)
		before := h.snapshot("c")
		b := readBlock(t, svc, "c", "t")
		res := applySet(t, svc, svcPerson, deleteOp("c", "t", map[string]string{"en-GB": b.Editions["en-GB"].Rev}))
		err := requireRefused(t, res.Ops[0], change.CodeInvalid)
		assert.Equal(t, "if_match/en", err.Field, "a removal names the revision of the block's own edition")

		res = applySet(t, svc, svcPerson, deleteOp("c", "t", map[string]string{"en": b.Rev, "en-GB": "r:0000000000000000"}))
		err = requireRefused(t, res.Ops[0], change.CodeStale)
		assert.Equal(t, "if_match/en-GB", err.Field, "an edition named at a revision it has left")
		assert.Equal(t, "Colour", res.Ops[0].Current.Text)

		res = applySet(t, svc, svcPerson, deleteOp("c", "t", map[string]string{"en": b.Rev, "de": b.Rev}))
		err = requireRefused(t, res.Ops[0], change.CodeStale)
		assert.Equal(t, "if_match/de", err.Field, "an edition the block lacks")
		assert.Equal(t, model.AbsentRevision, res.Ops[0].Current.Rev)
		assert.Equal(t, before, h.snapshot("c"))
	})

	t.Run("an edition the map leaves out goes with the block", func(t *testing.T) {
		h := newHome()
		svc := newMemService(h)
		b := readBlock(t, svc, "c", "t")
		res := applySet(t, svc, svcPerson, deleteOp("c", "t", map[string]string{"en": b.Rev}))
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops[0].Error)
		assert.Equal(t, []string{"x=X", "y=Y"}, keysOf(t, svc, "c"))
	})

	t.Run("a block addressed by its reader-local id is removed, the ids after it renumbered", func(t *testing.T) {
		h := newHome()
		svc := newMemService(h)
		rev := readBlock(t, svc, "c", "x").Rev
		res := applySet(t, svc, svcPerson, deleteOp("c", "tu1", map[string]string{"en": rev}))
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops[0].Error)
		assert.Equal(t, []string{"y=Y", "t=T"}, keysOf(t, svc, "c"))
	})

	t.Run("a block that is not there is not found", func(t *testing.T) {
		svc := newMemService(newHome())
		res := applySet(t, svc, svcPerson, deleteOp("c", "nope", map[string]string{"en": "r:0000000000000000"}))
		requireRefused(t, res.Ops[0], change.CodeNotFound)
	})

	t.Run("a block is replaced by adding the new one beside it and removing it", func(t *testing.T) {
		h := newHome()
		svc := newMemService(h)
		rev := readBlock(t, svc, "c", "y").Rev
		res := applySet(t, svc, svcPerson, insertOp("c", "y", "y2", map[string]string{"en": "Y2"}), deleteOp("c", "y", map[string]string{"en": rev}))
		require.Equal(t, change.SetApplied, res.Status)
		assert.Equal(t, []string{"x=X", "y2=Y2", "t=T"}, keysOf(t, svc, "c"))

		res = applySet(t, svc, svcPerson, deleteOp("c", "y2", map[string]string{"en": readBlock(t, svc, "c", "y2").Rev}), insertOp("c", "y2", "y3", map[string]string{"en": "Y3"}))
		err := requireRefused(t, res.Ops[1], change.CodeNotFound)
		assert.Contains(t, err.Message, "operation 0 removes block y2", "an operation sees the document the ones before it left")
	})
}

func TestService_StructureAndContentOfOneBlockAreRefused(t *testing.T) {
	h := newKVHome(textBlock("x", "X"), textBlock("y", "Y"))
	svc := newMemService(h)
	rev := readBlock(t, svc, "c", "y").Rev
	res := applySet(t, svc, svcPerson, deleteOp("c", "y", map[string]string{"en": rev}), edit(change.Ref{Doc: "c", Block: "y"}, rev, "Why"))
	err := requireRefused(t, res.Ops[1], change.CodeInvalid)
	assert.Contains(t, err.Message, "operation 0 removes block y")

	res = applySet(t, svc, svcPerson, insertOp("c", "x", "n", map[string]string{"en": "N"}), edit(change.Ref{Doc: "c", Block: "n"}, model.AbsentRevision, "N2"))
	err = requireRefused(t, res.Ops[1], change.CodeInvalid)
	assert.Contains(t, err.Message, "send its content in that operation's editions")

	res = applySet(t, svc, svcPerson, insertOp("c", "x", "n", map[string]string{"en": "N"}), deleteOp("c", "n", map[string]string{"en": rev}))
	requireRefused(t, res.Ops[1], change.CodeInvalid)
}

func TestService_StructureNeedsAHomeThatWritesIt(t *testing.T) {
	h := newKVHome(textBlock("x", "X"))
	h.kv = false
	svc := newMemService(h)
	res := applySet(t, svc, svcPerson, insertOp("c", "x", "n", map[string]string{"en": "N"}))
	assert.Equal(t, string(change.KindInsertBlock), requireRefused(t, res.Ops[0], change.CodeUnsupported).Capability)
}

// A document whose home writes fewer structural operations than its format
// declares, such as a catalog whose writer is configured to read notes from
// the members beside a block, is described, read and applied by what the
// home writes there.
func TestService_DescribesADocumentByWhatItsHomeWrites(t *testing.T) {
	ctx := context.Background()
	h := newKVHome(textBlock("x", "X"))
	h.writes = []change.Kind{}
	svc := newMemService(h)

	d, err := svc.Describe(ctx, change.DescribeRequest{Format: "memory-kv"})
	require.NoError(t, err)
	assert.NotNil(t, d.Ops[change.KindInsertBlock], "the format declares it")

	d, err = svc.Describe(ctx, change.DescribeRequest{Doc: "c"})
	require.NoError(t, err)
	assert.Nil(t, d.Ops[change.KindInsertBlock])
	assert.Nil(t, d.Ops[change.KindDeleteBlock])
	assert.NotNil(t, d.Ops[change.KindSetContent])

	b := readBlock(t, svc, "c", "x")
	assert.NotContains(t, b.Ops, change.KindDeleteBlock)

	res := applySet(t, svc, svcPerson, deleteOp("c", "x", map[string]string{"en": b.Rev}))
	err2 := requireRefused(t, res.Ops[0], change.CodeUnsupported)
	assert.Equal(t, string(change.KindDeleteBlock), err2.Capability)
	assert.Contains(t, err2.Message, "describe the document")
}

func TestService_StructureReachesTheDocumentOrIsRefused(t *testing.T) {
	t.Run("a new block the home did not write", func(t *testing.T) {
		h := newKVHome(textBlock("x", "X"))
		h.restructure = func(blocks []*model.Block, _ change.StructuralEdit) ([]*model.Block, *change.Error) {
			return blocks, nil
		}
		res := applySet(t, newMemService(h), svcPerson, insertOp("c", "x", "n", map[string]string{"en": "N"}))
		err := requireRefused(t, res.Ops[0], change.CodeUnsupported)
		assert.Contains(t, err.Message, "reads no block keyed n")
	})
	t.Run("a new block the format reads differently", func(t *testing.T) {
		h := newKVHome(textBlock("x", "X"))
		h.restructure = func(blocks []*model.Block, e change.StructuralEdit) ([]*model.Block, *change.Error) {
			e.Editions = map[model.EditionKey][]model.Run{{}: {model.TextR("Mangled")}}
			return memRestructure(blocks, e)
		}
		res := applySet(t, newMemService(h), svcPerson, insertOp("c", "x", "n", map[string]string{"en": "N"}))
		err := requireRefused(t, res.Ops[0], change.CodeUnsupported)
		assert.Equal(t, "editions/en", err.Field)
	})
	t.Run("a block the home did not remove", func(t *testing.T) {
		h := newKVHome(textBlock("x", "X"))
		h.restructure = func(blocks []*model.Block, _ change.StructuralEdit) ([]*model.Block, *change.Error) {
			return blocks, nil
		}
		svc := newMemService(h)
		res := applySet(t, svc, svcPerson, deleteOp("c", "x", map[string]string{"en": readBlock(t, svc, "c", "x").Rev}))
		err := requireRefused(t, res.Ops[0], change.CodeUnsupported)
		assert.Contains(t, err.Message, "still reads a block keyed x")
	})
	t.Run("an edit the home cannot write", func(t *testing.T) {
		h := newKVHome(textBlock("x", "X"))
		before := h.snapshot("c")
		h.restructure = func([]*model.Block, change.StructuralEdit) ([]*model.Block, *change.Error) {
			return nil, &change.Error{Code: change.CodeUnsupported, Message: "no place for it"}
		}
		res := applySet(t, newMemService(h), svcPerson, insertOp("c", "x", "n", map[string]string{"en": "N"}), insertOp("c", "n", "m", map[string]string{"en": "M"}))
		assert.Equal(t, "no place for it", requireRefused(t, res.Ops[0], change.CodeUnsupported).Message)
		assert.Equal(t, change.OpNotApplied, res.Ops[1].Status)
		assert.Equal(t, before, h.snapshot("c"))
	})
}

func TestService_StructureIsCheckedAndRecorded(t *testing.T) {
	t.Run("a new block that introduces a failing finding is refused", func(t *testing.T) {
		h := newKVHome(textBlock("x", "X"))
		check := &wordCheck{word: "utilize"}
		res := applySet(t, newMemService(h, change.WithCommitCheck(check)), svcAgent, insertOp("c", "x", "n", map[string]string{"en": "Please utilize it"}))
		requireRefused(t, res.Ops[0], change.CodeGateFailed)
		require.NotEmpty(t, check.seen)
		assert.Equal(t, model.AbsentRevision, check.seen[0][0].BeforeRev, "the check sees the new edition as created")
	})

	t.Run("the record holds the transitions of blocks added and removed", func(t *testing.T) {
		h := newKVHome(textBlock("x", "X"), textBlock("y", "Y"))
		rec := &memRecorder{}
		svc := newMemService(h, change.WithRecorder(rec))
		rev := readBlock(t, svc, "c", "y").Rev
		res := applySet(t, svc, svcPerson, deleteOp("c", "y", map[string]string{"en": rev}), insertOp("c", "x", "n", map[string]string{"en": "N"}))
		require.Equal(t, change.SetApplied, res.Status)
		require.Len(t, rec.records, 1)
		var got []string
		for _, tr := range rec.records[0].Transitions {
			got = append(got, tr.Key+":"+tr.BeforeRev[:2]+"->"+tr.AfterRev[:2])
		}
		slices.Sort(got)
		assert.Equal(t, []string{"n:ab->r:", "y:r:->ab"}, got, strings.Join(got, ","))
		assert.Empty(t, res.Ops[0].Invalidates, "a removed block leaves no translation on an older basis")
	})
}
