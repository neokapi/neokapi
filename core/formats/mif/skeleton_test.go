package mif_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/mif"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := context.Background()

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
	ctx := context.Background()

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
				b.Source = []*model.Segment{{ID: "s1", Content: model.NewFragment("Bonjour")}}
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
