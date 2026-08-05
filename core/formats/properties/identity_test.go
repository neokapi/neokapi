package properties

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// A properties file names its own entries: the key on the left of the
// separator is the identity the author controls. These tests hold the reader to
// that — a name that survives an edit elsewhere in the file is the whole reason
// reconcile can tell "this entry moved" from "this entry is new".

// readNames reads src and returns each translatable block's name, in document
// order.
func readNames(t *testing.T, src string) []string {
	t.Helper()
	blocks := readIdentityBlocks(t, src)
	names := make([]string, 0, len(blocks))
	for _, b := range blocks {
		names = append(names, b.Name)
	}
	return names
}

func readIdentityBlocks(t *testing.T, src string) []*model.Block {
	t.Helper()
	r := NewReader()
	// The default config extracts only keys matching `.*text.*`; these fixtures
	// use plain keys, so extract everything.
	r.cfg.ExtractOnlyMatchingKey = false
	doc := &model.RawDocument{Reader: io.NopCloser(strings.NewReader(src))}
	require.NoError(t, r.Open(context.Background(), doc))
	var blocks []*model.Block
	for res := range r.Read(context.Background()) {
		require.NoError(t, res.Error)
		if b, ok := res.Part.Resource.(*model.Block); ok {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

func TestIdentity_NameIsTheKey(t *testing.T) {
	names := readNames(t, "app.title=Alpha\napp.body=Bravo\n")
	assert.Equal(t, []string{"app.title", "app.body"}, names)
}

func TestIdentity_DeletingAnEarlierEntryLeavesLaterNames(t *testing.T) {
	before := readNames(t, "a=Alpha\nb=Bravo\nc=Charlie\n")
	after := readNames(t, "a=Alpha\nc=Charlie\n")

	require.Equal(t, []string{"a", "b", "c"}, before)
	assert.Equal(t, []string{"a", "c"}, after,
		"deleting b must not rename c")
}

func TestIdentity_ReorderingLeavesNames(t *testing.T) {
	before := readNames(t, "a=Alpha\nb=Bravo\nc=Charlie\n")
	after := readNames(t, "c=Charlie\na=Alpha\nb=Bravo\n")

	assert.ElementsMatch(t, before, after)
	assert.Equal(t, []string{"c", "a", "b"}, after)
}

func TestIdentity_SameTextDifferentKeys(t *testing.T) {
	names := readNames(t, "one=Alpha\ntwo=Alpha\n")
	assert.Equal(t, []string{"one", "two"}, names)
	assert.NotEqual(t, names[0], names[1],
		"identical text under different keys must not share an identity")
}

func TestIdentity_StableAcrossTwoReads(t *testing.T) {
	const src = "a=Alpha\nb=Bravo\n# note\nc=Charlie\n"
	first := readNames(t, src)
	second := readNames(t, src)
	assert.Equal(t, first, second)
}

// A repeated key is malformed — Java's Properties keeps the last one — but the
// reader still surfaces both entries, and two blocks may not share one name or
// downstream every decision about the first silently lands on the second.
func TestIdentity_RepeatedKeyIsDisambiguated(t *testing.T) {
	blocks := readIdentityBlocks(t, "dup=Alpha\nother=Bravo\ndup=Charlie\n")
	require.Len(t, blocks, 3)

	assert.Equal(t, "dup", blocks[0].Name)
	assert.Equal(t, "other", blocks[1].Name)
	assert.Equal(t, "dup#2", blocks[2].Name)

	// The raw key rides separately so the writer emits the key, not the name.
	assert.Equal(t, "dup", blocks[2].Properties[PropKey])
	_, ok := blocks[0].Properties[PropKey]
	assert.False(t, ok, "an unambiguous key is already the name")
}

// A name must never derive from the block's own text: reconcile grades the name
// as CONTEXT and the text as CONTENT, so a name that moves with the text moves
// both signals at once and an edited entry grades New, losing its history. Here
// the name is the key and the text is the value, which are independent — so
// editing a value keeps the entry.
func TestIdentity_EditedValueKeepsIdentity(t *testing.T) {
	const scope = "app.properties"
	var prior []reconcile.Unit
	for _, b := range readIdentityBlocks(t, "a=Alpha\nb=Bravo\n") {
		u := reconcile.Identify(scope, b)
		u.Key = b.Name
		prior = append(prior, u)
	}

	grades := make(map[string]reconcile.Kind)
	for _, res := range reconcile.Blocks(scope, readIdentityBlocks(t, "a=Alpha\nb=Bravo, revised\n"), prior) {
		grades[res.Block.Name] = res.Kind
	}

	assert.Equal(t, reconcile.Unchanged, grades["a"])
	assert.Equal(t, reconcile.Edited, grades["b"],
		"the key held still while the words changed")
}

// The generating writer must emit the file's key, never the disambiguating
// ordinal the reader appended to tell two entries apart.
func TestIdentity_WriterEmitsTheKeyNotTheOrdinal(t *testing.T) {
	blocks := readIdentityBlocks(t, "dup=Alpha\ndup=Charlie\n")
	require.Len(t, blocks, 2)

	var out strings.Builder
	w := NewWriter()
	w.Output = &out
	parts := make(chan *model.Part, len(blocks))
	for _, b := range blocks {
		parts <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(parts)
	require.NoError(t, w.Write(context.Background(), parts))

	assert.Equal(t, "dup=Alpha\ndup=Charlie", out.String())
}
