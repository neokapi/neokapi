package csv_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	csvfmt "github.com/gokapi/gokapi/core/formats/csv"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSimpleCSV(t *testing.T) {
	ctx := context.Background()
	reader := csvfmt.NewReader()
	input := "name,description\nWidget,A useful widget\nGadget,A cool gadget"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 4)
	assert.Equal(t, "Widget", blocks[0].SourceText())
	assert.Equal(t, "A useful widget", blocks[1].SourceText())
	assert.Equal(t, "Gadget", blocks[2].SourceText())
	assert.Equal(t, "A cool gadget", blocks[3].SourceText())
}

func TestReadTSV(t *testing.T) {
	ctx := context.Background()
	reader := csvfmt.NewReader()

	// Configure tab separator
	cfg := reader.Config()
	csvCfg := cfg.(*csvfmt.Config)
	csvCfg.Separator = '\t'
	csvCfg.HasHeader = true

	input := "name\tdescription\nWidget\tA useful widget"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Widget", blocks[0].SourceText())
	assert.Equal(t, "A useful widget", blocks[1].SourceText())
}

func TestReadNoHeader(t *testing.T) {
	ctx := context.Background()
	reader := csvfmt.NewReader()

	cfg := reader.Config()
	csvCfg := cfg.(*csvfmt.Config)
	csvCfg.HasHeader = false

	input := "Widget,A useful widget\nGadget,A cool gadget"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 4)
	assert.Equal(t, "Widget", blocks[0].SourceText())
	assert.Equal(t, "A useful widget", blocks[1].SourceText())
}

func TestReadSpecificColumns(t *testing.T) {
	ctx := context.Background()
	reader := csvfmt.NewReader()

	cfg := reader.Config()
	csvCfg := cfg.(*csvfmt.Config)
	csvCfg.TranslatableColumns = []int{1} // Only description column

	input := "name,description,value\nWidget,A useful widget,100"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	// Only column 1 (description) should be a block
	require.Len(t, blocks, 1)
	assert.Equal(t, "A useful widget", blocks[0].SourceText())

	// Columns 0 and 2 should be Data
	dataCount := 0
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name != "header-row" {
				dataCount++
			}
		}
	}
	assert.Equal(t, 2, dataCount, "non-translatable columns should be Data")
}

func TestReadEmpty(t *testing.T) {
	ctx := context.Background()
	reader := csvfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := context.Background()
	reader := csvfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("name\nWidget", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "csv", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := csvfmt.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/csv")
	assert.Contains(t, sig.Extensions, ".csv")
}

func TestReaderMetadata(t *testing.T) {
	reader := csvfmt.NewReader()
	assert.Equal(t, "csv", reader.Name())
	assert.Equal(t, "CSV", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := context.Background()
	reader := csvfmt.NewReader()
	err := reader.Open(ctx, nil)
	assert.Error(t, err)
}

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()

	input := "name,description\nWidget,A useful widget\nGadget,A cool gadget\n"

	reader := csvfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := csvfmt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "name,description")
	assert.Contains(t, output, "Widget,A useful widget")
	assert.Contains(t, output, "Gadget,A cool gadget")
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := context.Background()

	input := "name,description\nWidget,A useful widget\n"

	reader := csvfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Widget" {
				block.SetTargetText(model.LocaleFrench, "Widget-FR")
			} else if block.SourceText() == "A useful widget" {
				block.SetTargetText(model.LocaleFrench, "Un widget utile")
			}
		}
	}

	var buf bytes.Buffer
	writer := csvfmt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Widget-FR")
	assert.Contains(t, output, "Un widget utile")
}
