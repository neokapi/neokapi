package host

import (
	"testing"

	"github.com/hashicorp/go-plugin"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/shared"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandshakeConfig(t *testing.T) {
	// Verify handshake values match between host and what plugin servers expect.
	assert.Equal(t, uint(1), HandshakeConfig.ProtocolVersion)
	assert.Equal(t, "NEOKAPI_PLUGIN", HandshakeConfig.MagicCookieKey)
	assert.Equal(t, "neokapi-v1", HandshakeConfig.MagicCookieValue)
}

func TestPluginMap(t *testing.T) {
	pm := PluginMap()
	assert.Contains(t, pm, FormatReaderPluginName)
	assert.Contains(t, pm, FormatWriterPluginName)
	assert.Contains(t, pm, ToolPluginName)

	// Verify the types are correct go-plugin.Plugin implementations.
	var _ plugin.Plugin = pm[FormatReaderPluginName]
	var _ plugin.Plugin = pm[FormatWriterPluginName]
	var _ plugin.Plugin = pm[ToolPluginName]
}

func TestPluginManagerCreation(t *testing.T) {
	// Test creating a manager with nil logger.
	m := NewPluginManager(nil)
	assert.NotNil(t, m)
	assert.NotNil(t, m.logger)
	assert.Equal(t, 0, m.PluginCount())
}

func TestPluginManagerDiscoverEmptyDir(t *testing.T) {
	m := NewPluginManager(nil)
	reg := registry.NewFormatRegistry()

	// Discovering in a non-existent directory should not error
	// (filepath.Glob returns nil for no matches).
	err := m.DiscoverAndRegister(t.TempDir(), reg)
	require.NoError(t, err)
	assert.Equal(t, 0, m.PluginCount())
}

func TestPluginManagerShutdownEmpty(t *testing.T) {
	m := NewPluginManager(nil)
	// Shutdown with no plugins should be a no-op.
	m.Shutdown()
	assert.Equal(t, 0, m.PluginCount())
}

func TestPluginManagerShutdownPlugin(t *testing.T) {
	m := NewPluginManager(nil)
	// ShutdownPlugin for a non-existent plugin should return error.
	err := m.ShutdownPlugin("/nonexistent")
	require.Error(t, err)
}

func TestPluginManagerIsLoaded(t *testing.T) {
	m := NewPluginManager(nil)
	assert.False(t, m.IsLoaded("/some/path"))
}

func TestPluginBaseName(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/usr/local/bin/neokapi-plugin-csv", "csv"},
		{"/opt/plugins/neokapi-plugin-xliff-reader", "xliff-reader"},
		{"neokapi-plugin-json", "json"},
		{"other-binary", "other-binary"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.expected, pluginBaseName(tt.path))
		})
	}
}

// TestPartDTORoundTrip verifies that model types survive serialization round-trip.
func TestPartDTORoundTrip(t *testing.T) {
	// Create a Block part.
	block := model.NewBlock("b1", "Hello World")
	block.Name = "greeting"
	block.Properties["context"] = "ui"
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")

	original := &model.Part{
		Type:     model.PartBlock,
		Resource: block,
	}

	// Convert to DTO and back.
	dto := shared.PartToDTO(original)
	restored := shared.DTOToPart(dto)

	require.NotNil(t, restored)
	assert.Equal(t, model.PartBlock, restored.Type)

	restoredBlock, ok := restored.Resource.(*model.Block)
	require.True(t, ok)
	assert.Equal(t, "b1", restoredBlock.ID)
	assert.Equal(t, "greeting", restoredBlock.Name)
	assert.Equal(t, "Hello World", restoredBlock.SourceText())
	assert.Equal(t, "ui", restoredBlock.Properties["context"])
	assert.True(t, restoredBlock.HasTarget(model.LocaleFrench))
	assert.Equal(t, "Bonjour le monde", restoredBlock.TargetText(model.LocaleFrench))
}

// TestLayerDTORoundTrip verifies Layer serialization.
func TestLayerDTORoundTrip(t *testing.T) {
	layer := &model.Layer{
		ID:             "l1",
		Name:           "test.csv",
		Format:         "csv",
		Locale:         model.LocaleEnglish,
		Encoding:       "UTF-8",
		MimeType:       "text/csv",
		IsMultilingual: false,
		Properties:     map[string]string{"key": "value"},
	}

	original := &model.Part{
		Type:     model.PartLayerStart,
		Resource: layer,
	}

	dto := shared.PartToDTO(original)
	restored := shared.DTOToPart(dto)

	require.NotNil(t, restored)
	assert.Equal(t, model.PartLayerStart, restored.Type)

	restoredLayer, ok := restored.Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "l1", restoredLayer.ID)
	assert.Equal(t, "test.csv", restoredLayer.Name)
	assert.Equal(t, "csv", restoredLayer.Format)
	assert.Equal(t, model.LocaleEnglish, restoredLayer.Locale)
	assert.Equal(t, "UTF-8", restoredLayer.Encoding)
	assert.Equal(t, "value", restoredLayer.Properties["key"])
}

// TestDataDTORoundTrip verifies Data serialization.
func TestDataDTORoundTrip(t *testing.T) {
	data := &model.Data{
		ID:         "d1",
		Name:       "header",
		Properties: map[string]string{"type": "structural"},
	}

	original := &model.Part{
		Type:     model.PartData,
		Resource: data,
	}

	dto := shared.PartToDTO(original)
	restored := shared.DTOToPart(dto)

	require.NotNil(t, restored)
	assert.Equal(t, model.PartData, restored.Type)

	restoredData, ok := restored.Resource.(*model.Data)
	require.True(t, ok)
	assert.Equal(t, "d1", restoredData.ID)
	assert.Equal(t, "header", restoredData.Name)
	assert.Equal(t, "structural", restoredData.Properties["type"])
}

// TestGroupDTORoundTrip verifies GroupStart/GroupEnd serialization.
func TestGroupDTORoundTrip(t *testing.T) {
	gs := &model.GroupStart{
		ID:   "g1",
		Name: "Row 1",
		Type: "row",
	}
	ge := &model.GroupEnd{ID: "g1"}

	startPart := &model.Part{Type: model.PartGroupStart, Resource: gs}
	endPart := &model.Part{Type: model.PartGroupEnd, Resource: ge}

	startDTO := shared.PartToDTO(startPart)
	endDTO := shared.PartToDTO(endPart)

	restoredStart := shared.DTOToPart(startDTO)
	restoredEnd := shared.DTOToPart(endDTO)

	rgs, ok := restoredStart.Resource.(*model.GroupStart)
	require.True(t, ok)
	assert.Equal(t, "g1", rgs.ID)
	assert.Equal(t, "Row 1", rgs.Name)
	assert.Equal(t, "row", rgs.Type)

	rge, ok := restoredEnd.Resource.(*model.GroupEnd)
	require.True(t, ok)
	assert.Equal(t, "g1", rge.ID)
}

// TestMediaDTORoundTrip verifies Media serialization.
func TestMediaDTORoundTrip(t *testing.T) {
	media := &model.Media{
		ID:         "m1",
		MimeType:   "image/png",
		Data:       []byte{0x89, 0x50, 0x4E, 0x47},
		URI:        "logo.png",
		AltText:    "Logo",
		Properties: map[string]string{"width": "100"},
	}

	original := &model.Part{
		Type:     model.PartMedia,
		Resource: media,
	}

	dto := shared.PartToDTO(original)
	restored := shared.DTOToPart(dto)

	require.NotNil(t, restored)
	assert.Equal(t, model.PartMedia, restored.Type)

	restoredMedia, ok := restored.Resource.(*model.Media)
	require.True(t, ok)
	assert.Equal(t, "m1", restoredMedia.ID)
	assert.Equal(t, "image/png", restoredMedia.MimeType)
	assert.Equal(t, []byte{0x89, 0x50, 0x4E, 0x47}, restoredMedia.Data)
	assert.Equal(t, "Logo", restoredMedia.AltText)
}

// TestPartsToDTO verifies batch conversion.
func TestPartsToDTO(t *testing.T) {
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "l1"}},
		{Type: model.PartBlock, Resource: model.NewBlock("b1", "text")},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "l1"}},
	}

	dtos := shared.PartsToDTO(parts)
	assert.Len(t, dtos, 3)

	restored := shared.DTOToParts(dtos)
	assert.Len(t, restored, 3)
	assert.Equal(t, model.PartLayerStart, restored[0].Type)
	assert.Equal(t, model.PartBlock, restored[1].Type)
	assert.Equal(t, model.PartLayerEnd, restored[2].Type)
}

// TestFragmentWithSpansRoundTrip verifies Fragment with Span serialization.
func TestFragmentWithSpansRoundTrip(t *testing.T) {
	frag := model.NewFragment("")
	frag.AppendSpan(&model.Span{
		SpanType: model.SpanOpening,
		Type:     "bold",
		ID:       "s1",
		Data:     "<b>",
	})
	frag.AppendText("Hello")
	frag.AppendSpan(&model.Span{
		SpanType: model.SpanClosing,
		Type:     "bold",
		ID:       "s1",
		Data:     "</b>",
	})

	dto := shared.FragmentToDTO(frag)
	assert.Equal(t, frag.CodedText, dto.CodedText)
	assert.Len(t, dto.Spans, 2)
	assert.Equal(t, int(model.SpanOpening), dto.Spans[0].SpanType)
	assert.Equal(t, "bold", dto.Spans[0].Type)

	restored := shared.DTOToFragment(dto)
	assert.Equal(t, frag.CodedText, restored.CodedText)
	assert.Len(t, restored.Spans, 2)
	assert.Equal(t, model.SpanOpening, restored.Spans[0].SpanType)
	assert.Equal(t, "bold", restored.Spans[0].Type)
	assert.Equal(t, "<b>", restored.Spans[0].Data)
}

// TestFormatReaderClientInterface verifies that the client satisfies the interface.
func TestFormatReaderClientInterface(t *testing.T) {
	var _ format.DataFormatReader = (*FormatReaderRPCClient)(nil)
}

// TestFormatWriterClientInterface verifies that the client satisfies the interface.
func TestFormatWriterClientInterface(t *testing.T) {
	var _ format.DataFormatWriter = (*FormatWriterRPCClient)(nil)
}

// TestInfoResultFields verifies InfoResult field structure.
func TestInfoResultFields(t *testing.T) {
	info := shared.InfoResult{
		Name:        "csv",
		DisplayName: "CSV Reader",
		MIMETypes:   []string{"text/csv"},
		Extensions:  []string{".csv"},
	}
	assert.Equal(t, "csv", info.Name)
	assert.Equal(t, "CSV Reader", info.DisplayName)
	assert.Equal(t, []string{"text/csv"}, info.MIMETypes)
	assert.Equal(t, []string{".csv"}, info.Extensions)
}
