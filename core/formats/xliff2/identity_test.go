package xliff2_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// XLIFF states where every unit lives: a required `id`, unique within its
// <file>, under whatever <group> nesting the document declares. That address is
// the block's name — it does not move when a sibling unit is deleted, which is
// what lets a decision recorded about a unit still name that unit after the
// file is regenerated.

func identityNames(t *testing.T, input string) []string {
	t.Helper()
	ctx := t.Context()
	reader := xliff2.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	t.Cleanup(func() { reader.Close() })
	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	names := make([]string, 0, len(blocks))
	for _, b := range blocks {
		names = append(names, b.Name)
	}
	return names
}

// identityNamesStreaming reads through the skeleton path, a separate parse that
// must name blocks identically.
func identityNamesStreaming(t *testing.T, input string) []string {
	t.Helper()
	ctx := t.Context()
	reader := xliff2.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	reader.SetSkeletonStore(store)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	t.Cleanup(func() { reader.Close() })
	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	names := make([]string, 0, len(blocks))
	for _, b := range blocks {
		names = append(names, b.Name)
	}
	return names
}

func xliffDoc(units string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
` + units + `  </file>
</xliff>
`
}

const (
	unitAlpha   = `    <unit id="u1"><segment><source>Alpha</source></segment></unit>` + "\n"
	unitBravo   = `    <unit id="u2"><segment><source>Bravo</source></segment></unit>` + "\n"
	unitCharlie = `    <unit id="u3"><segment><source>Charlie</source></segment></unit>` + "\n"
)

func TestIdentity_NameIsTheUnitAddress(t *testing.T) {
	names := identityNames(t, xliffDoc(unitAlpha+unitBravo+unitCharlie))
	assert.Equal(t, []string{"f1/u1", "f1/u2", "f1/u3"}, names)
}

func TestIdentity_DeletingAnEarlierUnitLeavesLaterNames(t *testing.T) {
	after := identityNames(t, xliffDoc(unitAlpha+unitCharlie))
	assert.Equal(t, []string{"f1/u1", "f1/u3"}, after,
		"deleting u2 must not rename u3")
}

func TestIdentity_ReorderingLeavesNames(t *testing.T) {
	after := identityNames(t, xliffDoc(unitCharlie+unitAlpha+unitBravo))
	assert.Equal(t, []string{"f1/u3", "f1/u1", "f1/u2"}, after)
}

// Two units may carry the same source text — the same word in two places is
// ordinary — and they are still two units.
func TestIdentity_SameTextDifferentUnitIDs(t *testing.T) {
	names := identityNames(t, xliffDoc(
		`    <unit id="u1"><segment><source>Open</source></segment></unit>`+"\n"+
			`    <unit id="u2"><segment><source>Open</source></segment></unit>`+"\n"))
	assert.Equal(t, []string{"f1/u1", "f1/u2"}, names)
}

func TestIdentity_StableAcrossTwoReads(t *testing.T) {
	src := xliffDoc(unitAlpha + unitBravo)
	first := identityNames(t, src)
	second := identityNames(t, src)
	assert.Equal(t, first, second)
}

// Group nesting is part of the address, and a unit id is only unique within its
// <file> — the same id under two groups is legal enough in the wild that the
// name has to keep them apart.
func TestIdentity_GroupNestingIsPartOfTheAddress(t *testing.T) {
	names := identityNames(t, xliffDoc(
		`    <group id="g1">`+"\n"+
			`      <unit id="u1"><segment><source>Alpha</source></segment></unit>`+"\n"+
			`    </group>`+"\n"+
			`    <group id="g2">`+"\n"+
			`      <unit id="u1"><segment><source>Alpha</source></segment></unit>`+"\n"+
			`    </group>`+"\n"))
	assert.Equal(t, []string{"f1/g1/u1", "f1/g2/u1"}, names)
}

// A file that repeats a unit id in one scope is malformed, but two blocks may
// not come back sharing one name.
func TestIdentity_RepeatedUnitIDIsDisambiguated(t *testing.T) {
	names := identityNames(t, xliffDoc(
		unitAlpha+unitBravo+
			`    <unit id="u1"><segment><source>Alpha again</source></segment></unit>`+"\n"))
	assert.Equal(t, []string{"f1/u1", "f1/u2", "f1/u1#2"}, names)
}

func TestIdentity_StreamingPathAgreesWithDOMRead(t *testing.T) {
	src := xliffDoc(
		unitAlpha +
			`    <group id="g1">` + "\n" +
			`      <unit id="u9"><segment><source>Bravo</source></segment></unit>` + "\n" +
			`    </group>` + "\n" +
			unitCharlie)
	assert.Equal(t, identityNames(t, src), identityNamesStreaming(t, src))
}

// A name must never derive from the block's own text: reconcile grades the name
// as CONTEXT and the text as CONTENT, so a name that moves with the text moves
// both at once and an edited unit grades New, losing its history. An XLIFF name
// is built from ids, which the source text cannot touch — so editing a source
// keeps the unit.
func TestIdentity_EditedSourceKeepsIdentity(t *testing.T) {
	const scope = "messages.xlf"
	blocksOf := func(src string) []*model.Block {
		ctx := t.Context()
		reader := xliff2.NewReader()
		require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(src, model.LocaleEnglish)))
		t.Cleanup(func() { reader.Close() })
		return testutil.CollectBlocks(t, reader.Read(ctx))
	}

	var prior []reconcile.Unit
	for _, b := range blocksOf(xliffDoc(unitAlpha + unitBravo)) {
		u := reconcile.Identify(scope, b)
		u.Key = b.Name
		prior = append(prior, u)
	}

	edited := `    <unit id="u2"><segment><source>Bravo, revised</source></segment></unit>` + "\n"
	grades := make(map[string]reconcile.Kind)
	for _, res := range reconcile.Blocks(scope, blocksOf(xliffDoc(unitAlpha+edited)), prior) {
		grades[res.Block.Name] = res.Kind
	}

	assert.Equal(t, reconcile.Unchanged, grades["f1/u1"])
	assert.Equal(t, reconcile.Edited, grades["f1/u2"],
		"the unit address held still while the words changed")
}

// The unit's `name` attribute is a label, not identity. It rides as a property
// and comes back on the way out; nothing stamps the structural address into the
// document as a name attribute.
func TestIdentity_UnitNameAttributeRoundTrips(t *testing.T) {
	const src = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1" name="greeting"><segment><source>Alpha</source></segment></unit>
    <unit id="u2"><segment><source>Bravo</source></segment></unit>
  </file>
</xliff>
`
	ctx := t.Context()
	reader := xliff2.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(src, model.LocaleEnglish)))
	t.Cleanup(func() { reader.Close() })
	parts := testutil.CollectParts(t, reader.Read(ctx))

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "greeting", blocks[0].Properties[xliff2.PropUnitName])
	_, ok := blocks[1].Properties[xliff2.PropUnitName]
	assert.False(t, ok, "a unit with no name attribute records none")

	out := writeGenerated(t, blocks)
	assert.Contains(t, out, `name="greeting"`)
	assert.NotContains(t, out, `name="f1/u1"`,
		"the structural address is identity, not a name attribute")
	assert.NotContains(t, out, `name="f1/u2"`)
}

// writeGenerated serializes blocks through the scratch-build path — a fresh
// Layer carrying no source DOM, so the writer builds the document rather than
// patching the original bytes.
func writeGenerated(t *testing.T, blocks []*model.Block) string {
	t.Helper()
	buf := &bytes.Buffer{}
	w := xliff2.NewWriter()
	require.NoError(t, w.SetOutputWriter(buf))

	layer := &model.Layer{
		ID: "file-f1", Name: "f1", Format: "xliff2", Locale: "en", IsMultilingual: true,
		Properties: map[string]string{"target-language": "fr"},
	}
	parts := make(chan *model.Part, len(blocks)+1)
	parts <- &model.Part{Type: model.PartLayerStart, Resource: layer}
	for _, b := range blocks {
		parts <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(parts)
	require.NoError(t, w.Write(t.Context(), parts))
	return buf.String()
}

// A block converted from another format has no captured `name` attribute, and
// its name IS that format's key. Dropping it would throw the source key away,
// so the writer falls back to it — the one case where it may.
func TestIdentity_ForeignBlockKeepsItsKeyAsUnitName(t *testing.T) {
	buf := &bytes.Buffer{}
	w := xliff2.NewWriter()
	require.NoError(t, w.SetOutputWriter(buf))

	layer := &model.Layer{
		ID: "doc1", Name: "app.properties", Format: "properties", Locale: "en",
		Properties: map[string]string{"target-language": "fr"},
	}
	block := &model.Block{
		ID: "tu1", Name: "app.title", Translatable: true,
		Source:     []model.Run{{Text: &model.TextRun{Text: "Alpha"}}},
		Properties: map[string]string{},
	}
	parts := make(chan *model.Part, 2)
	parts <- &model.Part{Type: model.PartLayerStart, Resource: layer}
	parts <- &model.Part{Type: model.PartBlock, Resource: block}
	close(parts)
	require.NoError(t, w.Write(t.Context(), parts))

	assert.Contains(t, buf.String(), `name="app.title"`)
}
