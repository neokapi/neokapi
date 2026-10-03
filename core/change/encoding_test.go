package change_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// latin1Runs is "Café au <link>lait</link>." as a reader hands it on from a
// file in ISO-8859-1 or windows-1252: é is the byte 0xE9, which is not UTF-8.
func latin1Runs() []model.Run {
	return []model.Run{
		model.TextR("Caf\xe9 au "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link:hyperlink", Data: `<a href="https://old.example/milk">`, Attrs: map[string]string{"href": "https://old.example/milk"}}),
		model.TextR("lait"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link:hyperlink", Data: "</a>"}),
		model.TextR("."),
	}
}

// latin1Block is that paragraph, with a translation that is UTF-8.
func latin1Block() *model.Block {
	b := model.NewRunsBlock("tu1", latin1Runs())
	b.Name = "p"
	b.SourceLocale = "fr"
	b.SetTarget("nb", &model.Target{Runs: []model.Run{model.TextR("Kaffe med melk.")}, Status: model.TargetStatusTranslated})
	return b
}

// An operation that rewrites the text of an edition holding bytes that are not
// UTF-8 is refused: the sender read U+FFFD where those bytes are, and the edit
// would write it over them. An operation that leaves the text alone, and an
// edit of an edition whose text is UTF-8, land, and the bytes stay as they
// were.
func TestApplyBlock_TextThatIsNotUTF8(t *testing.T) {
	env := withFormat(person, htmlCaps())
	tests := []struct {
		name    string
		op      func(b *model.Block) change.Op
		refused bool
	}{
		{name: "replace_text", refused: true, op: func(b *model.Block) change.Op {
			return replace("", sourceRev(b), find("lait", "crème"))
		}},
		{name: "set_content", refused: true, op: func(b *model.Block) change.Op {
			return setText("", sourceRev(b), `Caf� au <x id="1"/>crème<x id="/1"/>.`)
		}},
		{name: "mark", refused: true, op: func(b *model.Block) change.Op {
			return mark("", sourceRev(b), findSel("au"), "fmt:bold", nil)
		}},
		{name: "set_attribute", op: func(b *model.Block) change.Op {
			return setAttr("", sourceRev(b), "1", "href", "https://new.example/milk")
		}},
		{name: "replace_text on a translation that is UTF-8", op: func(b *model.Block) change.Op {
			return replace("nb", editionRev(b, "nb"), find("melk", "fløte"))
		}},
		{name: "remove_edition of that translation", op: func(b *model.Block) change.Op {
			return change.Op{Kind: change.KindRemoveEdition, At: ref("nb"), IfMatch: editionRev(b, "nb"), Body: &change.RemoveEdition{}}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := latin1Block()
			before := sourceRev(b)
			res := apply(t, b, env, tc.op(b))
			if tc.refused {
				err := requireRefused(t, res[0], change.CodeUnsupported)
				assert.Equal(t, "encoding", err.Capability)
				assert.Contains(t, err.Message, "not UTF-8")
				assert.Equal(t, before, sourceRev(b), "a refusal leaves the block as it was")
				return
			}
			requireApplied(t, res)
			assert.Equal(t, "Caf\xe9 au ", b.Source[0].Text.Text, "the bytes stay as the file holds them")
		})
	}

	t.Run("a byte in a plural form", func(t *testing.T) {
		runs := pluralRuns()
		runs[1].Plural.Forms[model.PluralOther] = []model.Run{runs[1].Plural.Forms[model.PluralOther][0], model.TextR(" articl\xe9s")}
		b := model.NewRunsBlock("tu1", runs)
		b.Name = "p"
		res := apply(t, b, person, replace("", sourceRev(b), find("basket", "cart")))
		err := requireRefused(t, res[0], change.CodeUnsupported)
		assert.Equal(t, "encoding", err.Capability)
	})
}

// A read lists no operation that rewrites the text of a block whose text is
// not UTF-8, and keeps the operations that leave the text alone.
func TestService_ReadListsNoTextEditForTextThatIsNotUTF8(t *testing.T) {
	h := newMemHome(map[string][]memBlock{
		"a": {
			{key: "latin1", translatable: true, editions: map[model.EditionKey][]model.Run{{}: {model.TextR("Caf\xe9 au lait")}}},
			textBlock("plain", "Tea"),
		},
	})
	svc := newMemService(h)
	before := h.snapshot("a")
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: "a"})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 2)
	latin1, plain := page.Blocks[0], page.Blocks[1]
	assert.Contains(t, plain.Ops, change.KindSetContent)
	assert.Contains(t, plain.Ops, change.KindReplaceText)
	assert.NotContains(t, latin1.Ops, change.KindSetContent)
	assert.NotContains(t, latin1.Ops, change.KindReplaceText)
	assert.NotContains(t, latin1.Ops, change.KindMark)
	assert.Contains(t, latin1.Ops, change.KindRemoveEdition, "an operation that leaves the text alone is listed")

	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{edit(latin1.Ref, latin1.Rev, "Café au lait")}}, svcPerson)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	assert.Equal(t, before, h.snapshot("a"), "nothing is written")
}
