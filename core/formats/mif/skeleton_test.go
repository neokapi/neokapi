package mif_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/mif"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := mif.NewReader()
	writer := mif.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

func TestSkeletonStore_ByteExact_SimpleMIF(t *testing.T) {
	input := `<MIFFile 2015>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Hello World'>" + `
  >
 >
>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple MIF roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleParagraphs(t *testing.T) {
	input := `<MIFFile 2015>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`This is the first paragraph.'>" + `
  >
 >
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`This is the second paragraph.'>" + `
  >
 >
 <Para
  <PgfTag ` + "`Heading'>" + `
  <ParaLine
   <String ` + "`A heading paragraph.'>" + `
  >
 >
>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multi-paragraph MIF roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_SimpleFile(t *testing.T) {
	data, err := os.ReadFile("testdata/simple.mif")
	require.NoError(t, err)
	input := string(data)
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple.mif file roundtrip should be byte-exact")
}

// TestSkeletonStore_ByteExact_InlineCodeParagraph pins the round-trip
// fidelity that regressed with 5bacf636 (#615). #615 switched the reader to
// the inline-code paragraph model — a whole Para becomes ONE Block whose runs
// interleave text and inline-code (Ph) placeholders — but the writer-side
// skeleton-ref machinery (findStringPositions + writeFromSkeleton) still
// assigned one blockIdx per source `<String>`. The two manifests drifted, so
// the writer's block lookup landed off-by-N and scrambled / dropped output.
//
// This paragraph exercises the clusters the fix touches in one document:
//   - a `<Font>` statement BETWEEN two translatable `<String>`s (one Block,
//     two output text-groups distributed across two `<String>` slots), and
//   - a `<String>` whose only content is a building-block code `<$paratext>`
//     (Parameters.java:202 `<\$.*?>`), which must stay in the skeleton
//     verbatim — NOT be extracted or emptied.
//
// With NO translation applied the output must be byte-identical to the input:
// the Fonts stay between their Strings, each String keeps its own value, and
// the building-block-only String is left untouched. Before the fix the writer
// scrambled these slots (blockIdx drift) — emptying the `<$paratext>` String
// and mis-routing the `world` text.
func TestSkeletonStore_ByteExact_InlineCodeParagraph(t *testing.T) {
	input := `<MIFFile 2015>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Hello '>" + `
   <Font
    <FTag ` + "`'>" + `
    <FWeight ` + "`Bold'>" + `
   > # end of Font
   <String ` + "`world'>" + `
  >
 >
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`<$paratext\\>'>" + `
  >
 >
>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output,
		"inline-code paragraph (Font between two Strings + building-block-only String) must round-trip byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := `<MIFFile 2015>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Hello'>" + `
  >
 >
>
`
	ctx := t.Context()

	reader := mif.NewReader()
	writer := mif.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Modify text
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Hello" {
				b.Source = []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}
			}
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := `<MIFFile 2015>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Bonjour'>" + `
  >
 >
>
`
	assert.Equal(t, expected, buf.String())
}
