package editor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
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
		Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment("Hello world")}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
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
		Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment("Welcome")}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
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
	parts := makeHTMLParts()
	index := BuildBlockIndex(parts, "en", "html", "page.html")

	assert.Equal(t, "1.0", index.Version)
	assert.Equal(t, "en", index.SourceLocale)
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
	parts := makePartsWithTranslations()
	index := BuildBlockIndex(parts, "en", "html", "page.html")

	require.Len(t, index.Blocks, 2)
	assert.Equal(t, "Bonjour le monde", index.Blocks[0].Targets["fr"])
}

func TestBlockIndexRoundtrip(t *testing.T) {
	parts := makePartsWithTranslations()
	index := BuildBlockIndex(parts, "en", "html", "page.html")

	var buf bytes.Buffer
	err := WriteBlockIndex(&buf, index)
	require.NoError(t, err)

	index2, err := ReadBlockIndex(&buf)
	require.NoError(t, err)

	assert.Equal(t, index.Version, index2.Version)
	assert.Equal(t, index.SourceLocale, index2.SourceLocale)
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
	parts := makeHTMLParts()
	index := BuildBlockIndex(parts, "en", "html", "page.html")

	b := index.BlockByID("tu1")
	require.NotNil(t, b)
	assert.Equal(t, "Hello world", b.Source)

	b2 := index.BlockByID("nonexistent")
	assert.Nil(t, b2)
}

func TestUpdateTarget(t *testing.T) {
	parts := makeHTMLParts()
	index := BuildBlockIndex(parts, "en", "html", "page.html")

	err := index.UpdateTarget("tu1", "fr", "Bonjour le monde")
	require.NoError(t, err)
	assert.Equal(t, "Bonjour le monde", index.Blocks[0].Targets["fr"])

	err = index.UpdateTarget("nonexistent", "fr", "test")
	assert.Error(t, err)
}

func TestHTMLPreview(t *testing.T) {
	parts := makeHTMLParts()
	preview := BuildPreview(parts, nil, "html", "en")

	assert.Contains(t, preview, `<kat-block id="tu1">Hello world</kat-block>`)
	assert.Contains(t, preview, `<kat-block id="tu2">Welcome</kat-block>`)
	assert.Contains(t, preview, `<p>`)
	assert.Contains(t, preview, `<h1>`)
	assert.Contains(t, preview, "kat-block-click")
	assert.Contains(t, preview, "kat-select-block")
	assert.Contains(t, preview, "kat-update-block")
}

func TestHTMLPreviewWithSpans(t *testing.T) {
	frag := &model.Fragment{}
	frag.AppendText("Hello ")
	frag.AppendSpan(&model.Span{SpanType: model.SpanOpening, Type: "b", ID: "b", Data: "<b>"})
	frag.AppendText("world")
	frag.AppendSpan(&model.Span{SpanType: model.SpanClosing, Type: "b", ID: "b", Data: "</b>"})

	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       []*model.Segment{{ID: "s1", Content: frag}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
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

	preview := BuildPreview(parts, nil, "html", "en")
	assert.Contains(t, preview, `<kat-block id="tu1">Hello <b>world</b></kat-block>`)
}

func TestMarkdownPreview(t *testing.T) {
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: &model.Block{
			ID:           "tu1",
			Translatable: true,
			Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment("My Title")}},
			Targets:      make(map[model.LocaleID][]*model.Segment),
			Type:         "heading",
			Properties:   map[string]string{"level": "1"},
			Annotations:  make(map[string]model.Annotation),
		}},
		{Type: model.PartBlock, Resource: &model.Block{
			ID:           "tu2",
			Translatable: true,
			Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment("Some text")}},
			Targets:      make(map[model.LocaleID][]*model.Segment),
			Properties:   make(map[string]string),
			Annotations:  make(map[string]model.Annotation),
		}},
	}

	preview := BuildPreview(parts, nil, "markdown", "en")
	assert.Contains(t, preview, `<h1><kat-block id="tu1">My Title</kat-block></h1>`)
	assert.Contains(t, preview, `<p><kat-block id="tu2">Some text</kat-block></p>`)
}

func TestGenericPreview(t *testing.T) {
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: &model.Block{
			ID:           "tu1",
			Translatable: true,
			Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment("Hello <world>")}},
			Targets:      make(map[model.LocaleID][]*model.Segment),
			Properties:   make(map[string]string),
			Annotations:  make(map[string]model.Annotation),
		}},
	}

	preview := BuildPreview(parts, nil, "json", "en")
	assert.Contains(t, preview, `<kat-block id="tu1">Hello &lt;world&gt;</kat-block>`)
	assert.Contains(t, preview, "monospace")
}

func TestSourceHTMLWithSpans(t *testing.T) {
	frag := &model.Fragment{}
	frag.AppendText("Click ")
	frag.AppendSpan(&model.Span{SpanType: model.SpanOpening, Type: "a", ID: "a", Data: `<a href="/">`})
	frag.AppendText("here")
	frag.AppendSpan(&model.Span{SpanType: model.SpanClosing, Type: "a", ID: "a", Data: "</a>"})

	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       []*model.Segment{{ID: "s1", Content: frag}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}

	parts := []*model.Part{{Type: model.PartBlock, Resource: block}}
	index := BuildBlockIndex(parts, "en", "html", "page.html")

	assert.Equal(t, "Click here", index.Blocks[0].Source)
	assert.Equal(t, `Click <a href="/">here</a>`, index.Blocks[0].SourceHTML)
}

func TestRenderFragmentHTML(t *testing.T) {
	// Plain text
	block := model.NewBlock("tu1", "Hello world")
	assert.Equal(t, "Hello world", renderFragmentHTML(block))

	// With spans
	frag := &model.Fragment{}
	frag.AppendText("Hello ")
	frag.AppendSpan(&model.Span{SpanType: model.SpanOpening, Type: "b", ID: "b", Data: "<b>"})
	frag.AppendText("world")
	frag.AppendSpan(&model.Span{SpanType: model.SpanClosing, Type: "b", ID: "b", Data: "</b>"})

	block2 := &model.Block{
		ID:     "tu2",
		Source: []*model.Segment{{ID: "s1", Content: frag}},
	}
	assert.Equal(t, "Hello <b>world</b>", renderFragmentHTML(block2))
}

func TestReadWriteBlockIndex(t *testing.T) {
	index := &BlockIndex{
		Version:        "1.0",
		SourceLocale:   "en",
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
	assert.Contains(t, json, `"source_locale": "en"`)
	assert.Contains(t, json, `"Bonjour"`)

	index2, err := ReadBlockIndex(strings.NewReader(json))
	require.NoError(t, err)
	assert.Equal(t, "tu1", index2.Blocks[0].ID)
	assert.Equal(t, "Bonjour", index2.Blocks[0].Targets["fr"])
}
