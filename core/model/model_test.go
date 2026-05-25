package model_test

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPartTypeString(t *testing.T) {
	tests := []struct {
		pt   model.PartType
		want string
	}{
		{model.PartTypeUnknown, "Unknown"},
		{model.PartLayerStart, "LayerStart"},
		{model.PartLayerEnd, "LayerEnd"},
		{model.PartBlock, "Block"},
		{model.PartData, "Data"},
		{model.PartMedia, "Media"},
		{model.PartGroupStart, "GroupStart"},
		{model.PartGroupEnd, "GroupEnd"},
		{model.PartRawDocument, "RawDocument"},
		{model.PartCustom, "Custom"},
		{model.PartType(999), "Unknown"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.pt.String())
	}
}

func TestBlockCreation(t *testing.T) {
	block := model.NewBlock("tu1", "Hello world")

	assert.Equal(t, "tu1", block.ID)
	assert.True(t, block.Translatable)
	assert.Equal(t, "Hello world", block.SourceText())
	assert.NotNil(t, block.Targets)
	assert.NotNil(t, block.Properties)
	assert.NotNil(t, block.Annotations)
}

func TestBlockSourceTarget(t *testing.T) {
	block := model.NewBlock("tu1", "Hello")

	// Initially no targets
	assert.False(t, block.HasTarget(model.LocaleFrench))
	assert.Equal(t, "", block.TargetText(model.LocaleFrench))

	// Set target
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	assert.True(t, block.HasTarget(model.LocaleFrench))
	assert.Equal(t, "Bonjour", block.TargetText(model.LocaleFrench))

	// Set another target
	block.SetTargetText(model.LocaleGerman, "Hallo")
	assert.True(t, block.HasTarget(model.LocaleGerman))
	assert.Equal(t, "Hallo", block.TargetText(model.LocaleGerman))

	// Source unchanged
	assert.Equal(t, "Hello", block.SourceText())
}

func TestBlockSetSourceText(t *testing.T) {
	block := model.NewBlock("tu1", "Original")
	block.SetSourceText("Updated")
	assert.Equal(t, "Updated", block.SourceText())
	assert.Len(t, block.Source, 1)
	assert.Equal(t, "s1", block.Source[0].ID)
}

func TestBlockFirstSegment(t *testing.T) {
	block := model.NewBlock("tu1", "Hello")
	seg := block.FirstSegment()
	require.NotNil(t, seg)
	assert.Equal(t, "Hello", model.RunsPlainText(seg.Runs))

	// Empty block
	emptyBlock := &model.Block{}
	assert.Nil(t, emptyBlock.FirstSegment())
}

func TestBlockMultipleSegments(t *testing.T) {
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source: []*model.Segment{
			{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Hello "}}}},
			{ID: "s2", Runs: []model.Run{{Text: &model.TextRun{Text: "world"}}}},
		},
		Targets: make(map[model.LocaleID][]*model.Segment),
	}

	assert.Equal(t, "Hello world", block.SourceText())
}

// The inline content model is Run-based (RFC 0001); its behaviour is
// exercised in run_test.go. The legacy Fragment / Span / coded-text bridge
// has been removed.

func TestLayerRoot(t *testing.T) {
	layer := &model.Layer{
		ID:     "doc1",
		Name:   "document.html",
		Format: "html",
		Locale: model.LocaleEnglish,
	}

	assert.True(t, layer.IsRoot())
	assert.False(t, layer.IsEmbedded())
	assert.Equal(t, "doc1", layer.ResourceID())
}

func TestLayerNesting(t *testing.T) {
	root := &model.Layer{
		ID:     "doc1",
		Name:   "document.json",
		Format: "json",
		Locale: model.LocaleEnglish,
	}

	child := &model.Layer{
		ID:       "sub1",
		Name:     "body",
		Format:   "html",
		ParentID: root.ID,
	}

	assert.True(t, root.IsRoot())
	assert.False(t, root.IsEmbedded())
	assert.False(t, child.IsRoot())
	assert.True(t, child.IsEmbedded())
}

func TestLayerChildSameFormat(t *testing.T) {
	child := &model.Layer{
		ID:       "section1",
		ParentID: "doc1",
		Format:   "", // same as parent
	}
	assert.False(t, child.IsRoot())
	assert.False(t, child.IsEmbedded()) // not embedded because format is empty
}

func TestDataResource(t *testing.T) {
	data := &model.Data{
		ID:   "d1",
		Name: "header",
		Properties: map[string]string{
			"key": "value",
		},
	}
	assert.Equal(t, "d1", data.ResourceID())
}

func TestMediaResource(t *testing.T) {
	media := &model.Media{
		ID:       "m1",
		MimeType: "image/png",
		Data:     []byte{0x89, 0x50, 0x4E, 0x47},
		AltText:  "Logo",
	}
	assert.Equal(t, "m1", media.ResourceID())
}

func TestGroupMarkers(t *testing.T) {
	gs := &model.GroupStart{ID: "g1", Name: "table", Type: "x-table"}
	ge := &model.GroupEnd{ID: "g1"}

	assert.Equal(t, "g1", gs.ResourceID())
	assert.Equal(t, "g1", ge.ResourceID())
}

func TestRawDocumentResource(t *testing.T) {
	rd := &model.RawDocument{
		URI:          "file:///input.html",
		Encoding:     "UTF-8",
		SourceLocale: model.LocaleEnglish,
		MimeType:     "text/html",
		FormatID:     "html",
	}
	assert.Equal(t, "file:///input.html", rd.ResourceID())
}

func TestSkeletonFragmentBased(t *testing.T) {
	skel := &model.Skeleton{
		Strategy: model.SkeletonFragmentBased,
		Parts: []model.SkeletonPart{
			&model.SkeletonText{Text: "<p>"},
			&model.SkeletonRef{ResourceID: "tu1", Property: "target"},
			&model.SkeletonText{Text: "</p>"},
		},
	}
	assert.Equal(t, model.SkeletonFragmentBased, skel.Strategy)
	assert.Len(t, skel.Parts, 3)
}

func TestSkeletonReparse(t *testing.T) {
	skel := &model.Skeleton{
		Strategy:  model.SkeletonReparse,
		SourceURI: "file:///input.txt",
	}
	assert.Equal(t, model.SkeletonReparse, skel.Strategy)
	assert.Equal(t, "file:///input.txt", skel.SourceURI)
}

func TestLocaleID(t *testing.T) {
	locale := model.LocaleEnglish
	assert.Equal(t, "en", locale.String())
	assert.False(t, locale.IsEmpty())

	empty := model.LocaleID("")
	assert.True(t, empty.IsEmpty())
}

func TestAltTranslation(t *testing.T) {
	alt := &model.AltTranslation{
		Source:    []model.Run{{Text: &model.TextRun{Text: "Hello"}}},
		Target:    []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}},
		Locale:    model.LocaleFrench,
		Origin:    "tm",
		Score:     0.95,
		MatchType: model.MatchFuzzy,
	}
	assert.Equal(t, "alt-translation", alt.AnnotationType())
	assert.Equal(t, "Bonjour", model.FlattenRuns(alt.Target))
}

func TestPartResult(t *testing.T) {
	// Success result
	part := &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
	result := model.PartResult{Part: part}
	require.NoError(t, result.Error)
	assert.NotNil(t, result.Part)

	// Error result
	errResult := model.PartResult{Error: assert.AnError}
	require.Error(t, errResult.Error)
	assert.Nil(t, errResult.Part)
}

func TestBlockJSONSerialization(t *testing.T) {
	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	block.Properties["context"] = "greeting"

	data, err := json.Marshal(block)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Hello")

	var decoded model.Block
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "tu1", decoded.ID)
}

func TestLayerJSONSerialization(t *testing.T) {
	layer := &model.Layer{
		ID:       "doc1",
		Name:     "test.html",
		Format:   "html",
		Locale:   model.LocaleEnglish,
		Encoding: "UTF-8",
		Properties: map[string]string{
			"version": "1.0",
		},
	}

	data, err := json.Marshal(layer)
	require.NoError(t, err)

	var decoded model.Layer
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "doc1", decoded.ID)
	assert.Equal(t, "html", decoded.Format)
	assert.Equal(t, model.LocaleEnglish, decoded.Locale)
}

// --- Block uniform locale access (Framework AD-006) ---

func TestBlockText_SourceLocale(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en-US"
	assert.Equal(t, "Hello", b.Text("en-US"))
}

func TestBlockText_TargetLocale(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en-US"
	b.SetTargetText("fr-FR", "Bonjour")
	assert.Equal(t, "Bonjour", b.Text("fr-FR"))
}

func TestBlockText_MissingLocale(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en-US"
	assert.Equal(t, "", b.Text("de-DE"))
}

func TestBlockText_NoSourceLocaleSet(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	// SourceLocale not set — Text("en-US") looks in targets, finds nothing.
	assert.Equal(t, "", b.Text("en-US"))
	// SourceText still works via the direct method.
	assert.Equal(t, "Hello", b.SourceText())
}

func TestBlockSetText_Source(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en-US"
	b.SetText("en-US", "Hi")
	assert.Equal(t, "Hi", b.SourceText())
}

func TestBlockSetText_Target(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en-US"
	b.SetText("fr-FR", "Bonjour")
	assert.Equal(t, "Bonjour", b.TargetText("fr-FR"))
}

func TestBlockHasLocale_Source(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en-US"
	assert.True(t, b.HasLocale("en-US"))
	assert.False(t, b.HasLocale("fr-FR"))
}

func TestBlockHasLocale_Target(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en-US"
	b.SetTargetText("de-DE", "Hallo")
	assert.True(t, b.HasLocale("de-DE"))
}

func TestBlockText_BilingualPeerComparison(t *testing.T) {
	// Two target locales compared as peers — no source involved.
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en-US"
	b.SetTargetText("de-DE", "Hallo")
	b.SetTargetText("fr-FR", "Bonjour")

	// A bilingual tool comparing [de-DE, fr-FR] uses Text() uniformly.
	assert.Equal(t, "Hallo", b.Text("de-DE"))
	assert.Equal(t, "Bonjour", b.Text("fr-FR"))
}
