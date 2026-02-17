package yaml_test

import (
	"context"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	yamlfmt "github.com/gokapi/gokapi/core/formats/yaml"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSimpleYAML(t *testing.T) {
	ctx := context.Background()
	reader := yamlfmt.NewReader()
	input := "title: Hello World\ndescription: A test"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
	assert.Contains(t, texts, "A test")
}

func TestReadNestedYAML(t *testing.T) {
	ctx := context.Background()
	reader := yamlfmt.NewReader()
	input := "root:\n  nested:\n    value: Deep content"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Deep content", blocks[0].SourceText())
	assert.Equal(t, "root.nested.value", blocks[0].Name)
}

func TestReadYAMLArray(t *testing.T) {
	ctx := context.Background()
	reader := yamlfmt.NewReader()
	input := "items:\n  - First\n  - Second\n  - Third"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "First", blocks[0].SourceText())
	assert.Equal(t, "Second", blocks[1].SourceText())
	assert.Equal(t, "Third", blocks[2].SourceText())
}

func TestReadYAMLLayerStartEnd(t *testing.T) {
	ctx := context.Background()
	reader := yamlfmt.NewReader()
	input := "key: value"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "yaml", layer.Format)
}

func TestReadYAMLEmpty(t *testing.T) {
	ctx := context.Background()
	reader := yamlfmt.NewReader()
	input := "{}"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	assert.Empty(t, blocks)
}

func TestReaderSignature(t *testing.T) {
	reader := yamlfmt.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/yaml")
	assert.Contains(t, sig.Extensions, ".yaml")
	assert.Contains(t, sig.Extensions, ".yml")
}

func TestReaderMetadata(t *testing.T) {
	reader := yamlfmt.NewReader()
	assert.Equal(t, "yaml", reader.Name())
	assert.Equal(t, "YAML", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := context.Background()
	reader := yamlfmt.NewReader()
	err := reader.Open(ctx, nil)
	assert.Error(t, err)
}
