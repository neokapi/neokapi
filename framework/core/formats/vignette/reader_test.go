package vignette_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats/vignette"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: VignetteFilterTest#testPlainText
func TestPlainText(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	input := "This is plain text."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "This is plain text.", blocks[0].SourceText())
}

// okapi: VignetteFilterTest#testYAMLFrontMatter
func TestYAMLFrontMatter(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	input := "---\ntitle: \"Test\"\n---\nHello world."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello world.", blocks[0].SourceText())

	// YAML should be Data
	hasYAML := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Properties["type"] == "yaml-frontmatter" {
				hasYAML = true
			}
		}
	}
	assert.True(t, hasYAML, "YAML front matter should be Data")
}

// okapi: VignetteFilterTest#testRmdCodeChunk
func TestRmdCodeChunk(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	input := "Before code.\n```{r setup}\nlibrary(ggplot2)\n```\nAfter code."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Before code.", blocks[0].SourceText())
	assert.Equal(t, "After code.", blocks[1].SourceText())
}

// okapi: VignetteFilterTest#testRnwCodeChunk
func TestRnwCodeChunk(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	input := "Before code.\n<<setup>>=\nlibrary(ggplot2)\n@\nAfter code."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Before code.", blocks[0].SourceText())
	assert.Equal(t, "After code.", blocks[1].SourceText())
}

// okapi: VignetteFilterTest#testCodeChunkWithOptions
func TestCodeChunkWithOptions(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	input := "Text.\n```{r plot, echo=FALSE, fig.width=10}\nplot(data)\n```\nMore text."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Text.", blocks[0].SourceText())
	assert.Equal(t, "More text.", blocks[1].SourceText())
}

// okapi: VignetteFilterTest#testRnwWithOptions
func TestRnwCodeChunkWithOptions(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	input := "Text.\n<<plot, echo=FALSE>>=\nplot(data)\n@\nMore text."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Text.", blocks[0].SourceText())
	assert.Equal(t, "More text.", blocks[1].SourceText())
}

// okapi: VignetteFilterTest#testMultipleCodeChunks
func TestMultipleCodeChunks(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	input := "Para 1.\n```{r}\ncode1()\n```\nPara 2.\n```{r}\ncode2()\n```\nPara 3."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "Para 1.", blocks[0].SourceText())
	assert.Equal(t, "Para 2.", blocks[1].SourceText())
	assert.Equal(t, "Para 3.", blocks[2].SourceText())
}

// okapi: VignetteFilterTest#testCodeContentAsData
func TestCodeContentAsData(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	input := "```{r}\nx <- 1\ny <- 2\n```"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks, "code chunk content should not produce blocks")

	codeData := 0
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Properties["type"] == "code" {
				codeData++
			}
		}
	}
	assert.Equal(t, 2, codeData, "each code line should be Data")
}

func TestReadEmpty(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestReadNilDocument(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	err := reader.Open(ctx, nil)
	assert.Error(t, err)
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	input := "Hello"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "vignette", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := vignette.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.Extensions, ".Rmd")
	assert.Contains(t, sig.Extensions, ".Rnw")
}

func TestReaderMetadata(t *testing.T) {
	reader := vignette.NewReader()
	assert.Equal(t, "vignette", reader.Name())
	assert.Equal(t, "R Vignette", reader.DisplayName())
}

func TestConfigFormatName(t *testing.T) {
	cfg := &vignette.Config{}
	assert.Equal(t, "vignette", cfg.FormatName())
}

func TestConfigValidate(t *testing.T) {
	cfg := &vignette.Config{}
	assert.NoError(t, cfg.Validate())
}

func TestConfigReset(t *testing.T) {
	cfg := &vignette.Config{}
	cfg.Reset()
	assert.NoError(t, cfg.Validate())
}

func TestConfigApplyMapUnknown(t *testing.T) {
	cfg := &vignette.Config{}
	err := cfg.ApplyMap(map[string]any{"unknown": true})
	assert.Error(t, err)
}

func TestConfigApplyMapEmpty(t *testing.T) {
	cfg := &vignette.Config{}
	err := cfg.ApplyMap(map[string]any{})
	assert.NoError(t, err)
}

func TestRoundTripRmd(t *testing.T) {
	ctx := context.Background()

	f, err := os.Open("testdata/simple.Rmd")
	require.NoError(t, err)
	reader := vignette.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.Rmd", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := vignette.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "This is an introduction paragraph.")
	assert.Contains(t, output, "This is text after the code chunk.")
	assert.Contains(t, output, "This is the conclusion.")
	assert.Contains(t, output, "library(ggplot2)")
}

func TestRoundTripRnw(t *testing.T) {
	ctx := context.Background()

	f, err := os.Open("testdata/simple.Rnw")
	require.NoError(t, err)
	reader := vignette.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.Rnw", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := vignette.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "This is an introduction.")
	assert.Contains(t, output, "This is the conclusion.")
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := context.Background()

	input := "Hello world.\n```{r}\ncode()\n```\nGoodbye world."
	reader := vignette.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello world." {
				block.SetTargetText(model.LocaleFrench, "Bonjour le monde.")
			} else if block.SourceText() == "Goodbye world." {
				block.SetTargetText(model.LocaleFrench, "Au revoir le monde.")
			}
		}
	}

	var buf bytes.Buffer
	writer := vignette.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour le monde.")
	assert.Contains(t, output, "Au revoir le monde.")
	assert.NotContains(t, output, "Hello world.")
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reader := vignette.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello\n```{r}\ncode()\n```\nWorld", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)
	var count int
	for range ch {
		count++
	}
	assert.LessOrEqual(t, count, 10)
}

func TestWriterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var buf bytes.Buffer
	writer := vignette.NewWriter()
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := make(chan *model.Part)
	cancel()

	err = writer.Write(ctx, ch)
	assert.ErrorIs(t, err, context.Canceled)
}

// okapi: VignetteFilterTest#testMultiLineText
func TestMultiLineTextBlock(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	input := "Line one.\nLine two.\nLine three."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Line one.\nLine two.\nLine three.", blocks[0].SourceText())
}

// okapi: VignetteFilterTest#testYAMLNotAtStart
func TestYAMLNotAtStart(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	// --- not at line 1 should be treated as text, not YAML
	input := "Some text.\n---\ntitle: test\n---"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	// All content should be in a single block since no YAML parsing is triggered
	assert.Contains(t, blocks[0].SourceText(), "Some text.")
	assert.Contains(t, blocks[0].SourceText(), "---")
}

// okapi: VignetteFilterTest#testOnlyCode
func TestOnlyCode(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	input := "```{r}\nx <- 1\n```"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	assert.Empty(t, blocks, "code-only content should produce no blocks")
}

// okapi: VignetteFilterTest#testOnlyYAML
func TestOnlyYAML(t *testing.T) {
	ctx := context.Background()
	reader := vignette.NewReader()
	input := "---\ntitle: Test\nauthor: Author\n---"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	assert.Empty(t, blocks, "YAML-only content should produce no blocks")
}
