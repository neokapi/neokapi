package vignette_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/vignette"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func vignetteSkeletonRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := vignette.NewReader()
	writer := vignette.NewWriter()

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

func TestSkeletonStore_ByteExact_PlainText(t *testing.T) {
	input := "This is plain text."
	output := vignetteSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "plain text roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultiLine(t *testing.T) {
	input := "Line one.\nLine two.\nLine three."
	output := vignetteSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multi-line text roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	input := "Line one.\r\nLine two.\r\nLine three."
	output := vignetteSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "CRLF line endings should be preserved")
}

func TestSkeletonStore_ByteExact_WithCodeChunk(t *testing.T) {
	input := "Before code.\n```{r setup}\nlibrary(ggplot2)\n```\nAfter code."
	output := vignetteSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "code chunks should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_WithYAML(t *testing.T) {
	input := "---\ntitle: \"Test\"\nauthor: \"Author\"\n---\nHello world."
	output := vignetteSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "YAML front matter should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_RnwCodeChunk(t *testing.T) {
	input := "Before code.\n<<setup>>=\nlibrary(ggplot2)\n@\nAfter code."
	output := vignetteSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "Rnw code chunks should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleCodeChunks(t *testing.T) {
	input := "Para 1.\n```{r}\ncode1()\n```\nPara 2.\n```{r}\ncode2()\n```\nPara 3."
	output := vignetteSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multiple code chunks should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_TrailingNewline(t *testing.T) {
	input := "Hello world.\n"
	output := vignetteSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_EmptyInput(t *testing.T) {
	input := ""
	output := vignetteSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "empty input should produce empty output")
}

func TestSkeletonStore_ByteExact_OnlyCode(t *testing.T) {
	input := "```{r}\nx <- 1\n```"
	output := vignetteSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "code-only content should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "Hello world.\n```{r}\ncode()\n```\nGoodbye world."
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := vignette.NewReader()
	writer := vignette.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Hello world.":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Bonjour le monde.")}}
			case "Goodbye world.":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Au revoir le monde.")}}
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := "Bonjour le monde.\n```{r}\ncode()\n```\nAu revoir le monde."
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_ByteExact_YAMLWithCRLF(t *testing.T) {
	input := "---\r\ntitle: \"Test\"\r\n---\r\nHello world."
	output := vignetteSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "YAML with CRLF should be byte-exact")
}
