package editor

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeHTMLParts() []*model.Part {
	layer := &model.Layer{
		ID:       "doc1",
		Name:     "page.html",
		Format:   "html",
		Locale:   model.LocaleEnglish,
		Encoding: "UTF-8",
	}

	block1 := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: "Hello world"}}},
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   map[string]string{"note": "greeting"},
		Annotations:  make(map[string]model.Annotation),
		Skeleton: &model.Skeleton{
			Strategy: model.SkeletonFragmentBased,
			Parts: []model.SkeletonPart{
				&model.SkeletonText{Text: "<p>"},
				&model.SkeletonRef{ResourceID: "tu1", Property: "target"},
				&model.SkeletonText{Text: "</p>"},
			},
		},
	}

	block2 := &model.Block{
		ID:           "tu2",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: "Welcome"}}},
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
		Skeleton: &model.Skeleton{
			Strategy: model.SkeletonFragmentBased,
			Parts: []model.SkeletonPart{
				&model.SkeletonText{Text: "<h1>"},
				&model.SkeletonRef{ResourceID: "tu2", Property: "target"},
				&model.SkeletonText{Text: "</h1>"},
			},
		},
	}

	data1 := &model.Data{
		ID:         "d1",
		Name:       "doctype",
		Properties: map[string]string{},
	}

	return []*model.Part{
		{Type: model.PartLayerStart, Resource: layer},
		{Type: model.PartData, Resource: data1},
		{Type: model.PartBlock, Resource: block1},
		{Type: model.PartBlock, Resource: block2},
		{Type: model.PartLayerEnd, Resource: layer},
	}
}

func makePartsWithTranslations() []*model.Part {
	parts := makeHTMLParts()
	block := parts[2].Resource.(*model.Block)
	block.SetTargetText("fr", "Bonjour le monde")
	return parts
}

func TestBuildBlockIndex(t *testing.T) {
	t.Parallel()
	parts := makeHTMLParts()
	index := BuildBlockIndex(parts, "en", "html", "page.html")

	assert.Equal(t, "1.0", index.Version)
	assert.Equal(t, "en", index.SourceLanguage)
	assert.Equal(t, "html", index.OriginalFormat)
	assert.Equal(t, "page.html", index.OriginalItem)

	// Blocks
	require.Len(t, index.Blocks, 2)
	assert.Equal(t, "tu1", index.Blocks[0].ID)
	assert.Equal(t, 0, index.Blocks[0].Index)
	assert.True(t, index.Blocks[0].Translatable)
	assert.Equal(t, "Hello world", index.Blocks[0].Source)
	assert.Equal(t, "greeting", index.Blocks[0].Properties["note"])

	assert.Equal(t, "tu2", index.Blocks[1].ID)
	assert.Equal(t, 1, index.Blocks[1].Index)
	assert.Equal(t, "Welcome", index.Blocks[1].Source)

	// Skeleton
	require.NotNil(t, index.Blocks[0].Skeleton)
	assert.Equal(t, "fragment", index.Blocks[0].Skeleton.Strategy)
	require.Len(t, index.Blocks[0].Skeleton.Parts, 3)
	assert.Equal(t, "text", index.Blocks[0].Skeleton.Parts[0].Type)
	assert.Equal(t, "<p>", index.Blocks[0].Skeleton.Parts[0].Text)
	assert.Equal(t, "ref", index.Blocks[0].Skeleton.Parts[1].Type)
	assert.Equal(t, "tu1", index.Blocks[0].Skeleton.Parts[1].ResourceID)
	assert.Equal(t, "text", index.Blocks[0].Skeleton.Parts[2].Type)

	// Data parts
	require.Len(t, index.DataParts, 1)
	assert.Equal(t, "d1", index.DataParts[0].ID)
	assert.Equal(t, "doctype", index.DataParts[0].Name)

	// Document order
	assert.Equal(t, []string{
		"layer_start:doc1",
		"data:d1",
		"block:tu1",
		"block:tu2",
		"layer_end:doc1",
	}, index.DocumentOrder)

	// Layers
	require.Len(t, index.Layers, 1)
	assert.Equal(t, "doc1", index.Layers[0].ID)
	assert.Equal(t, "html", index.Layers[0].Format)
}

func TestBuildBlockIndexWithTargets(t *testing.T) {
	t.Parallel()
	parts := makePartsWithTranslations()
	index := BuildBlockIndex(parts, "en", "html", "page.html")

	require.Len(t, index.Blocks, 2)
	assert.Equal(t, "Bonjour le monde", index.Blocks[0].Targets["fr"])
}

func TestBlockIndexRoundtrip(t *testing.T) {
	t.Parallel()
	parts := makePartsWithTranslations()
	index := BuildBlockIndex(parts, "en", "html", "page.html")

	var buf bytes.Buffer
	err := WriteBlockIndex(&buf, index)
	require.NoError(t, err)

	index2, err := ReadBlockIndex(&buf)
	require.NoError(t, err)

	assert.Equal(t, index.Version, index2.Version)
	assert.Equal(t, index.SourceLanguage, index2.SourceLanguage)
	assert.Equal(t, index.OriginalFormat, index2.OriginalFormat)
	assert.Equal(t, index.OriginalItem, index2.OriginalItem)
	require.Len(t, index2.Blocks, 2)
	assert.Equal(t, "tu1", index2.Blocks[0].ID)
	assert.Equal(t, "Hello world", index2.Blocks[0].Source)
	assert.Equal(t, "Bonjour le monde", index2.Blocks[0].Targets["fr"])

	// Skeleton survives
	require.NotNil(t, index2.Blocks[0].Skeleton)
	assert.Equal(t, "fragment", index2.Blocks[0].Skeleton.Strategy)
	require.Len(t, index2.Blocks[0].Skeleton.Parts, 3)

	// Document order survives
	assert.Equal(t, index.DocumentOrder, index2.DocumentOrder)
}

func TestBlockByID(t *testing.T) {
	t.Parallel()
	parts := makeHTMLParts()
	index := BuildBlockIndex(parts, "en", "html", "page.html")

	b := index.BlockByID("tu1")
	require.NotNil(t, b)
	assert.Equal(t, "Hello world", b.Source)

	b2 := index.BlockByID("nonexistent")
	assert.Nil(t, b2)
}

func TestUpdateTarget(t *testing.T) {
	t.Parallel()
	parts := makeHTMLParts()
	index := BuildBlockIndex(parts, "en", "html", "page.html")

	err := index.UpdateTarget("tu1", "fr", "Bonjour le monde")
	require.NoError(t, err)
	assert.Equal(t, "Bonjour le monde", index.Blocks[0].Targets["fr"])

	err = index.UpdateTarget("nonexistent", "fr", "test")
	require.Error(t, err)
}

func TestHTMLPreview(t *testing.T) {
	t.Parallel()
	parts := makeHTMLParts()
	preview := BuildHTMLPreview(parts)

	assert.Contains(t, preview, `<kat-block id="tu1">Hello world</kat-block>`)
	assert.Contains(t, preview, `<kat-block id="tu2">Welcome</kat-block>`)
	assert.Contains(t, preview, `<p>`)
	assert.Contains(t, preview, `<h1>`)
	assert.Contains(t, preview, "kat-block-click")
	assert.Contains(t, preview, "kat-select-block")
	assert.Contains(t, preview, "kat-update-block")
}

func TestHTMLPreviewWithSpans(t *testing.T) {
	t.Parallel()
	runs := []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{PcOpen: &model.PcOpenRun{ID: "b", Type: "b", Data: "<b>"}},
		{Text: &model.TextRun{Text: "world"}},
		{PcClose: &model.PcCloseRun{ID: "b", Type: "b", Data: "</b>"}},
	}

	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       runs,
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
		Skeleton: &model.Skeleton{
			Strategy: model.SkeletonFragmentBased,
			Parts: []model.SkeletonPart{
				&model.SkeletonText{Text: "<p>"},
				&model.SkeletonRef{ResourceID: "tu1", Property: "target"},
				&model.SkeletonText{Text: "</p>"},
			},
		},
	}

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block},
	}

	preview := BuildHTMLPreview(parts)
	assert.Contains(t, preview, `<kat-block id="tu1">Hello <b>world</b></kat-block>`)
}

func TestMarkdownPreview(t *testing.T) {
	t.Parallel()
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: &model.Block{
			ID:           "tu1",
			Translatable: true,
			Source:       []model.Run{{Text: &model.TextRun{Text: "My Title"}}},
			Targets:      make(map[model.VariantKey]*model.Target),
			Type:         "heading",
			Properties:   map[string]string{"level": "1"},
			Annotations:  make(map[string]model.Annotation),
		}},
		{Type: model.PartBlock, Resource: &model.Block{
			ID:           "tu2",
			Translatable: true,
			Source:       []model.Run{{Text: &model.TextRun{Text: "Some text"}}},
			Targets:      make(map[model.VariantKey]*model.Target),
			Properties:   make(map[string]string),
			Annotations:  make(map[string]model.Annotation),
		}},
	}

	preview := BuildMarkdownPreview(parts)
	assert.Contains(t, preview, `<h1><kat-block id="tu1">My Title</kat-block></h1>`)
	assert.Contains(t, preview, `<p><kat-block id="tu2">Some text</kat-block></p>`)
}

func TestGenericPreview(t *testing.T) {
	t.Parallel()
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: &model.Block{
			ID:           "tu1",
			Translatable: true,
			Source:       []model.Run{{Text: &model.TextRun{Text: "Hello <world>"}}},
			Targets:      make(map[model.VariantKey]*model.Target),
			Properties:   make(map[string]string),
			Annotations:  make(map[string]model.Annotation),
		}},
	}

	preview := buildGenericPreview(parts)
	assert.Contains(t, preview, `<kat-block id="tu1">Hello &lt;world&gt;</kat-block>`)
	assert.Contains(t, preview, "monospace")
}

func TestSourceHTMLWithSpans(t *testing.T) {
	t.Parallel()
	runs := []model.Run{
		{Text: &model.TextRun{Text: "Click "}},
		{PcOpen: &model.PcOpenRun{ID: "a", Type: "a", Data: `<a href="/">`}},
		{Text: &model.TextRun{Text: "here"}},
		{PcClose: &model.PcCloseRun{ID: "a", Type: "a", Data: "</a>"}},
	}

	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       runs,
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}

	parts := []*model.Part{{Type: model.PartBlock, Resource: block}}
	index := BuildBlockIndex(parts, "en", "html", "page.html")

	assert.Equal(t, "Click here", index.Blocks[0].Source)
	assert.Equal(t, `Click <a href="/">here</a>`, index.Blocks[0].SourceHTML)
}

func TestRenderFragmentHTML(t *testing.T) {
	t.Parallel()
	// Plain text
	block := model.NewBlock("tu1", "Hello world")
	assert.Equal(t, "Hello world", renderFragmentHTML(block))

	// With inline-code runs
	runs := []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{PcOpen: &model.PcOpenRun{ID: "b", Type: "b", Data: "<b>"}},
		{Text: &model.TextRun{Text: "world"}},
		{PcClose: &model.PcCloseRun{ID: "b", Type: "b", Data: "</b>"}},
	}

	block2 := &model.Block{
		ID:     "tu2",
		Source: runs,
	}
	assert.Equal(t, "Hello <b>world</b>", renderFragmentHTML(block2))
}

func TestReadWriteBlockIndex(t *testing.T) {
	t.Parallel()
	index := &BlockIndex{
		Version:        "1.0",
		SourceLanguage: "en",
		OriginalFormat: "html",
		OriginalItem:   "page.html",
		Blocks: []Block{
			{
				ID:           "tu1",
				Index:        0,
				Translatable: true,
				Source:       "Hello",
				SourceHTML:   "Hello",
				Targets:      map[string]string{"fr": "Bonjour"},
				Properties:   map[string]string{},
			},
		},
		DataParts:     []DataPart{},
		DocumentOrder: []string{"block:tu1"},
		Layers:        []LayerInfo{},
	}

	var buf bytes.Buffer
	err := WriteBlockIndex(&buf, index)
	require.NoError(t, err)

	// Verify JSON contains expected content
	json := buf.String()
	assert.Contains(t, json, `"kat_version": "1.0"`)
	assert.Contains(t, json, `"source_language": "en"`)
	assert.Contains(t, json, `"Bonjour"`)

	index2, err := ReadBlockIndex(strings.NewReader(json))
	require.NoError(t, err)
	assert.Equal(t, "tu1", index2.Blocks[0].ID)
	assert.Equal(t, "Bonjour", index2.Blocks[0].Targets["fr"])
}

func TestBuildPreviewFromBlockIndex(t *testing.T) {
	t.Parallel()
	// Create a BlockIndex manually
	index := &BlockIndex{
		Version:        "1.0",
		SourceLanguage: "en",
		OriginalFormat: "json",
		OriginalItem:   "messages.json",
		Blocks: []Block{
			{ID: "tu1", Source: "Hello world", SourceHTML: "Hello world"},
			{ID: "tu2", Source: "Goodbye", SourceHTML: "Goodbye"},
		},
		DocumentOrder: []string{"block:tu1", "block:tu2"},
	}

	indexJSON, _ := json.Marshal(index)
	preview := BuildPreviewFromBlockIndex(string(indexJSON))

	assert.Contains(t, preview, `<kat-block id="tu1">Hello world</kat-block>`)
	assert.Contains(t, preview, `<kat-block id="tu2">Goodbye</kat-block>`)
	assert.Contains(t, preview, "kat-block-click") // boilerplate JS
}

func TestBuildPreviewFromBlockIndex_WithSkeleton(t *testing.T) {
	t.Parallel()
	index := &BlockIndex{
		Version: "1.0",
		Blocks: []Block{
			{
				ID: "tu1", Source: "Hello", SourceHTML: "Hello",
				Skeleton: &SkeletonData{
					Strategy: "fragment",
					Parts: []SkeletonPartData{
						{Type: "text", Text: "<p>"},
						{Type: "ref", ResourceID: "tu1"},
						{Type: "text", Text: "</p>"},
					},
				},
			},
		},
		DocumentOrder: []string{"block:tu1"},
	}

	indexJSON, _ := json.Marshal(index)
	preview := BuildPreviewFromBlockIndex(string(indexJSON))

	assert.Contains(t, preview, "<p>")
	assert.Contains(t, preview, `<kat-block id="tu1">Hello</kat-block>`)
	assert.Contains(t, preview, "</p>")
}

func TestBuildPreviewFromBlockIndex_Empty(t *testing.T) {
	t.Parallel()
	preview := BuildPreviewFromBlockIndex("")
	assert.Empty(t, preview)
}

func TestBuildPreviewFromBlockIndex_NoDocumentOrder(t *testing.T) {
	t.Parallel()
	index := &BlockIndex{
		Blocks: []Block{
			{ID: "tu1", Source: "Hello", SourceHTML: "Hello"},
		},
	}
	indexJSON, _ := json.Marshal(index)
	preview := BuildPreviewFromBlockIndex(string(indexJSON))
	assert.Contains(t, preview, `<kat-block id="tu1">Hello</kat-block>`)
}
