package shared_test

import (
	"testing"

	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/plugin/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpanRoundtrip(t *testing.T) {
	span := &model.Span{
		SpanType:  model.SpanOpening,
		Type:      "bold",
		ID:        "s1",
		Data:      "<b>",
		OuterData: "</b>",
		Deletable: true,
		Cloneable: false,
	}

	dto := shared.SpanToDTO(span)
	assert.Equal(t, int(model.SpanOpening), dto.SpanType)
	assert.Equal(t, "bold", dto.Type)
	assert.Equal(t, "s1", dto.ID)
	assert.Equal(t, "<b>", dto.Data)
	assert.Equal(t, "</b>", dto.OuterData)
	assert.True(t, dto.Deletable)
	assert.False(t, dto.Cloneable)

	result := shared.DTOToSpan(dto)
	assert.Equal(t, span.SpanType, result.SpanType)
	assert.Equal(t, span.Type, result.Type)
	assert.Equal(t, span.ID, result.ID)
	assert.Equal(t, span.Data, result.Data)
	assert.Equal(t, span.OuterData, result.OuterData)
	assert.Equal(t, span.Deletable, result.Deletable)
	assert.Equal(t, span.Cloneable, result.Cloneable)
}

func TestFragmentRoundtrip(t *testing.T) {
	frag := &model.Fragment{
		CodedText: "Hello \uE101world\uE102",
		Spans: []*model.Span{
			{SpanType: model.SpanOpening, ID: "s1", Data: "<b>"},
			{SpanType: model.SpanClosing, ID: "s1", Data: "</b>"},
		},
	}

	dto := shared.FragmentToDTO(frag)
	assert.Equal(t, frag.CodedText, dto.CodedText)
	assert.Len(t, dto.Spans, 2)

	result := shared.DTOToFragment(dto)
	assert.Equal(t, frag.CodedText, result.CodedText)
	require.Len(t, result.Spans, 2)
	assert.Equal(t, model.SpanOpening, result.Spans[0].SpanType)
	assert.Equal(t, model.SpanClosing, result.Spans[1].SpanType)
}

func TestFragmentNilRoundtrip(t *testing.T) {
	dto := shared.FragmentToDTO(nil)
	assert.Empty(t, dto.CodedText)
	assert.Nil(t, dto.Spans)
}

func TestSegmentRoundtrip(t *testing.T) {
	seg := &model.Segment{
		ID:      "seg1",
		Content: model.NewFragment("Hello world"),
	}

	dto := shared.SegmentToDTO(seg)
	assert.Equal(t, "seg1", dto.ID)

	result := shared.DTOToSegment(dto)
	assert.Equal(t, "seg1", result.ID)
	assert.Equal(t, seg.Content.CodedText, result.Content.CodedText)
}

func TestBlockRoundtrip(t *testing.T) {
	block := model.NewBlock("tu1", "Hello world")
	block.Name = "greeting"
	block.Type = "text"
	block.MimeType = "text/plain"
	block.Properties["context"] = "test"
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")

	dto := shared.BlockToDTO(block)
	require.NotNil(t, dto)
	assert.Equal(t, "tu1", dto.ID)
	assert.Equal(t, "greeting", dto.Name)
	assert.Equal(t, "text", dto.Type)
	assert.True(t, dto.Translatable)
	assert.NotEmpty(t, dto.Source)
	assert.NotEmpty(t, dto.Targets)

	result := shared.DTOToBlock(dto)
	require.NotNil(t, result)
	assert.Equal(t, "tu1", result.ID)
	assert.Equal(t, "greeting", result.Name)
	assert.Equal(t, "text", result.Type)
	assert.Equal(t, "text/plain", result.MimeType)
	assert.True(t, result.Translatable)
	assert.Equal(t, "Hello world", result.SourceText())
	assert.Equal(t, "test", result.Properties["context"])
	assert.Equal(t, "Bonjour le monde", result.TargetText(model.LocaleFrench))
}

func TestBlockNilRoundtrip(t *testing.T) {
	assert.Nil(t, shared.BlockToDTO(nil))
	assert.Nil(t, shared.DTOToBlock(nil))
}

func TestLayerRoundtrip(t *testing.T) {
	layer := &model.Layer{
		ID:             "doc1",
		Name:           "main",
		Format:         "html",
		Locale:         model.LocaleEnglish,
		Encoding:       "UTF-8",
		MimeType:       "text/html",
		LineBreak:      "\n",
		IsMultilingual: true,
		ParentID:       "root",
		Properties:     map[string]string{"key": "val"},
	}

	dto := shared.LayerToDTO(layer)
	require.NotNil(t, dto)
	assert.Equal(t, "doc1", dto.ID)
	assert.Equal(t, "html", dto.Format)
	assert.Equal(t, "en", dto.Locale)
	assert.True(t, dto.IsMultilingual)

	result := shared.DTOToLayer(dto)
	require.NotNil(t, result)
	assert.Equal(t, layer.ID, result.ID)
	assert.Equal(t, layer.Name, result.Name)
	assert.Equal(t, layer.Format, result.Format)
	assert.Equal(t, layer.Locale, result.Locale)
	assert.Equal(t, layer.Encoding, result.Encoding)
	assert.Equal(t, layer.MimeType, result.MimeType)
	assert.Equal(t, layer.LineBreak, result.LineBreak)
	assert.Equal(t, layer.IsMultilingual, result.IsMultilingual)
	assert.Equal(t, layer.ParentID, result.ParentID)
	assert.Equal(t, "val", result.Properties["key"])
}

func TestLayerNilRoundtrip(t *testing.T) {
	assert.Nil(t, shared.LayerToDTO(nil))
	assert.Nil(t, shared.DTOToLayer(nil))
}

func TestDataRoundtrip(t *testing.T) {
	data := &model.Data{
		ID:         "d1",
		Name:       "skeleton",
		Properties: map[string]string{"key": "val"},
	}

	dto := shared.DataToDTO(data)
	require.NotNil(t, dto)
	assert.Equal(t, "d1", dto.ID)
	assert.Equal(t, "skeleton", dto.Name)

	result := shared.DTOToData(dto)
	require.NotNil(t, result)
	assert.Equal(t, data.ID, result.ID)
	assert.Equal(t, data.Name, result.Name)
	assert.Equal(t, "val", result.Properties["key"])
}

func TestDataNilRoundtrip(t *testing.T) {
	assert.Nil(t, shared.DataToDTO(nil))
	assert.Nil(t, shared.DTOToData(nil))
}

func TestGroupStartRoundtrip(t *testing.T) {
	gs := &model.GroupStart{ID: "g1", Name: "section", Type: "div"}

	dto := shared.GroupStartToDTO(gs)
	require.NotNil(t, dto)
	assert.Equal(t, "g1", dto.ID)
	assert.Equal(t, "section", dto.Name)
	assert.Equal(t, "div", dto.Type)

	result := shared.DTOToGroupStart(dto)
	require.NotNil(t, result)
	assert.Equal(t, gs.ID, result.ID)
	assert.Equal(t, gs.Name, result.Name)
	assert.Equal(t, gs.Type, result.Type)
}

func TestGroupStartNilRoundtrip(t *testing.T) {
	assert.Nil(t, shared.GroupStartToDTO(nil))
	assert.Nil(t, shared.DTOToGroupStart(nil))
}

func TestGroupEndRoundtrip(t *testing.T) {
	ge := &model.GroupEnd{ID: "g1"}

	dto := shared.GroupEndToDTO(ge)
	require.NotNil(t, dto)
	assert.Equal(t, "g1", dto.ID)

	result := shared.DTOToGroupEnd(dto)
	require.NotNil(t, result)
	assert.Equal(t, ge.ID, result.ID)
}

func TestGroupEndNilRoundtrip(t *testing.T) {
	assert.Nil(t, shared.GroupEndToDTO(nil))
	assert.Nil(t, shared.DTOToGroupEnd(nil))
}

func TestMediaRoundtrip(t *testing.T) {
	media := &model.Media{
		ID:         "m1",
		MimeType:   "image/png",
		Data:       []byte{0x89, 0x50, 0x4E, 0x47},
		URI:        "images/logo.png",
		AltText:    "Company logo",
		Properties: map[string]string{"width": "100"},
	}

	dto := shared.MediaToDTO(media)
	require.NotNil(t, dto)
	assert.Equal(t, "m1", dto.ID)
	assert.Equal(t, "image/png", dto.MimeType)
	assert.Equal(t, media.Data, dto.Data)
	assert.Equal(t, "images/logo.png", dto.URI)
	assert.Equal(t, "Company logo", dto.AltText)

	result := shared.DTOToMedia(dto)
	require.NotNil(t, result)
	assert.Equal(t, media.ID, result.ID)
	assert.Equal(t, media.MimeType, result.MimeType)
	assert.Equal(t, media.Data, result.Data)
	assert.Equal(t, media.URI, result.URI)
	assert.Equal(t, media.AltText, result.AltText)
	assert.Equal(t, "100", result.Properties["width"])
}

func TestMediaNilRoundtrip(t *testing.T) {
	assert.Nil(t, shared.MediaToDTO(nil))
	assert.Nil(t, shared.DTOToMedia(nil))
}

func TestPartBlockRoundtrip(t *testing.T) {
	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	dto := shared.PartToDTO(part)
	assert.Equal(t, int(model.PartBlock), dto.PartType)
	require.NotNil(t, dto.Block)
	assert.Nil(t, dto.Layer)
	assert.Nil(t, dto.Data)

	result := shared.DTOToPart(dto)
	assert.Equal(t, model.PartBlock, result.Type)
	resultBlock, ok := result.Resource.(*model.Block)
	require.True(t, ok)
	assert.Equal(t, "tu1", resultBlock.ID)
	assert.Equal(t, "Hello", resultBlock.SourceText())
}

func TestPartLayerStartRoundtrip(t *testing.T) {
	layer := &model.Layer{ID: "doc1", Format: "html", Locale: model.LocaleEnglish}
	part := &model.Part{Type: model.PartLayerStart, Resource: layer}

	dto := shared.PartToDTO(part)
	assert.Equal(t, int(model.PartLayerStart), dto.PartType)
	require.NotNil(t, dto.Layer)
	assert.Nil(t, dto.Block)

	result := shared.DTOToPart(dto)
	assert.Equal(t, model.PartLayerStart, result.Type)
	resultLayer, ok := result.Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "doc1", resultLayer.ID)
}

func TestPartLayerEndRoundtrip(t *testing.T) {
	layer := &model.Layer{ID: "doc1"}
	part := &model.Part{Type: model.PartLayerEnd, Resource: layer}

	dto := shared.PartToDTO(part)
	assert.Equal(t, int(model.PartLayerEnd), dto.PartType)
	require.NotNil(t, dto.Layer)

	result := shared.DTOToPart(dto)
	assert.Equal(t, model.PartLayerEnd, result.Type)
}

func TestPartDataRoundtrip(t *testing.T) {
	data := &model.Data{ID: "d1", Name: "skel"}
	part := &model.Part{Type: model.PartData, Resource: data}

	dto := shared.PartToDTO(part)
	assert.Equal(t, int(model.PartData), dto.PartType)
	require.NotNil(t, dto.Data)

	result := shared.DTOToPart(dto)
	assert.Equal(t, model.PartData, result.Type)
	resultData, ok := result.Resource.(*model.Data)
	require.True(t, ok)
	assert.Equal(t, "d1", resultData.ID)
}

func TestPartGroupStartRoundtrip(t *testing.T) {
	gs := &model.GroupStart{ID: "g1", Name: "section", Type: "div"}
	part := &model.Part{Type: model.PartGroupStart, Resource: gs}

	dto := shared.PartToDTO(part)
	assert.Equal(t, int(model.PartGroupStart), dto.PartType)
	require.NotNil(t, dto.GroupStart)

	result := shared.DTOToPart(dto)
	assert.Equal(t, model.PartGroupStart, result.Type)
	resultGS, ok := result.Resource.(*model.GroupStart)
	require.True(t, ok)
	assert.Equal(t, "g1", resultGS.ID)
}

func TestPartGroupEndRoundtrip(t *testing.T) {
	ge := &model.GroupEnd{ID: "g1"}
	part := &model.Part{Type: model.PartGroupEnd, Resource: ge}

	dto := shared.PartToDTO(part)
	assert.Equal(t, int(model.PartGroupEnd), dto.PartType)
	require.NotNil(t, dto.GroupEnd)

	result := shared.DTOToPart(dto)
	assert.Equal(t, model.PartGroupEnd, result.Type)
}

func TestPartMediaRoundtrip(t *testing.T) {
	media := &model.Media{ID: "m1", MimeType: "image/png", Data: []byte{0xFF}}
	part := &model.Part{Type: model.PartMedia, Resource: media}

	dto := shared.PartToDTO(part)
	assert.Equal(t, int(model.PartMedia), dto.PartType)
	require.NotNil(t, dto.Media)

	result := shared.DTOToPart(dto)
	assert.Equal(t, model.PartMedia, result.Type)
	resultMedia, ok := result.Resource.(*model.Media)
	require.True(t, ok)
	assert.Equal(t, "m1", resultMedia.ID)
	assert.Equal(t, []byte{0xFF}, resultMedia.Data)
}

func TestPartsToDTO(t *testing.T) {
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")},
		{Type: model.PartData, Resource: &model.Data{ID: "d1"}},
	}

	dtos := shared.PartsToDTO(parts)
	assert.Len(t, dtos, 2)
	assert.Equal(t, int(model.PartBlock), dtos[0].PartType)
	assert.Equal(t, int(model.PartData), dtos[1].PartType)
}

func TestDTOToParts(t *testing.T) {
	dtos := []shared.PartDTO{
		{PartType: int(model.PartBlock), Block: &shared.BlockDTO{ID: "tu1", Translatable: true}},
		{PartType: int(model.PartData), Data: &shared.DataDTO{ID: "d1"}},
	}

	parts := shared.DTOToParts(dtos)
	assert.Len(t, parts, 2)
	assert.Equal(t, model.PartBlock, parts[0].Type)
	assert.Equal(t, model.PartData, parts[1].Type)
}
