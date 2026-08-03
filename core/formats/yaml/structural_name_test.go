package yaml_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// YAML names blocks by key path, and a sequence item by its index WITHIN its own
// sequence. That was structural from the start — these tests pin it, because a
// block's Name is folded into its context hash (core/reconcile) and the rest of
// the identity work now depends on this format staying where it is.

func yamlNamesByText(t *testing.T, content string) map[string]string {
	t.Helper()
	ctx := t.Context()
	reader := yamlfmt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))
	defer reader.Close()

	out := map[string]string{}
	for _, b := range testutil.CollectBlocks(t, reader.Read(ctx)) {
		out[b.SourceText()] = b.Name
	}
	return out
}

func yamlBlockNames(t *testing.T, content string) []string {
	t.Helper()
	ctx := t.Context()
	reader := yamlfmt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))
	defer reader.Close()

	var out []string
	for _, b := range testutil.CollectBlocks(t, reader.Read(ctx)) {
		out = append(out, b.Name)
	}
	return out
}

// The stability claim pinned in core/formats/identity_conformance_test.go, kept
// here too so a change to the YAML reader fails in the YAML package first.
func TestStructuralName_UnrelatedKeyDeletionDoesNotRename(t *testing.T) {
	before := yamlNamesByText(t, "a: Alpha\nb: Bravo\nc: Charlie\n")
	after := yamlNamesByText(t, "a: Alpha\nc: Charlie\n")

	require.Equal(t, "a", before["Alpha"])
	require.Equal(t, "c", before["Charlie"])
	assert.Equal(t, before["Charlie"], after["Charlie"],
		"deleting an unrelated key must not rename the block after it")
	assert.Equal(t, before["Alpha"], after["Alpha"])
}

// A sequence item is indexed within its own sequence, so deleting an item from
// one list leaves every item of every other list alone.
func TestStructuralName_DeletionInAnotherSequenceDoesNotRename(t *testing.T) {
	const v1 = "intro:\n  - Alpha\n  - Bravo\nbody:\n  - Charlie\n  - Delta\n"
	const v2 = "intro:\n  - Alpha\nbody:\n  - Charlie\n  - Delta\n"

	before, after := yamlNamesByText(t, v1), yamlNamesByText(t, v2)

	require.Equal(t, "intro.[0]", before["Alpha"])
	require.Equal(t, "intro.[1]", before["Bravo"])
	require.Equal(t, "body.[0]", before["Charlie"])
	require.Equal(t, "body.[1]", before["Delta"])

	assert.Equal(t, before["Charlie"], after["Charlie"],
		"an item of an untouched sequence must keep its name")
	assert.Equal(t, before["Delta"], after["Delta"])
	assert.Equal(t, before["Alpha"], after["Alpha"])
}

func TestStructuralName_IdenticalTextGetsDistinctNames(t *testing.T) {
	names := yamlBlockNames(t, "a: Same\nb: Same\nlist:\n  - Same\n  - Same\n")
	require.Len(t, names, 4)
	assert.Equal(t, []string{"a", "b", "list.[0]", "list.[1]"}, names)

	seen := map[string]bool{}
	for _, n := range names {
		assert.False(t, seen[n], "duplicate block name %q", n)
		seen[n] = true
	}
}

func TestStructuralName_StableAcrossTwoReads(t *testing.T) {
	const doc = "app:\n  title: Hello\n  items:\n    - One\n    - Two\nfooter: Bye\n"
	assert.Equal(t, yamlBlockNames(t, doc), yamlBlockNames(t, doc))
	assert.Equal(t,
		[]string{"app.title", "app.items.[0]", "app.items.[1]", "footer"},
		yamlBlockNames(t, doc))
}
