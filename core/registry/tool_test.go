package registry

import (
	"context"
	"maps"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTool struct{}

func (s *stubTool) Name() string                      { return "stub" }
func (s *stubTool) Description() string               { return "stub tool" }
func (s *stubTool) Config() tool.ToolConfig           { return nil }
func (s *stubTool) SetConfig(_ tool.ToolConfig) error { return nil }
func (s *stubTool) Process(_ context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for p := range in {
		out <- p
	}
	return nil
}

func TestRegister_BasicTool(t *testing.T) {
	reg := NewToolRegistry()
	reg.Register("test-tool", func() tool.Tool { return &stubTool{} })

	assert.True(t, reg.Has("test-tool"))
	assert.False(t, reg.Has("nonexistent"))

	names := reg.Names()
	assert.Contains(t, names, ToolID("test-tool"))
}

func TestRegisterWithSchema_PropagatesMetadata(t *testing.T) {
	reg := NewToolRegistry()

	s := &schema.ComponentSchema{
		Title:       "Test Tool",
		Description: "A test tool",
		ToolMeta: &schema.ToolMeta{
			ID:       "test-tool",
			Category: "validate",
			Consumes: []schema.IOPort{{Type: schema.PortTarget, Side: model.SideTarget}},
			Produces: []schema.IOPort{schema.Port(model.OverlayQA, model.SideTarget)},
			Tags:     []string{"quality", "ai-powered"},
			Requires: []string{"target-language", "credentials"},
		},
	}

	reg.RegisterWithSchema("test-tool", func() tool.Tool { return &stubTool{} }, s)

	infos := reg.ListWithSchemas()
	require.Len(t, infos, 1)

	info := infos[0]
	assert.Equal(t, ToolID("test-tool"), info.Name)
	assert.Equal(t, "A test tool", info.Description)
	assert.Equal(t, "validate", info.Category)
	assert.True(t, info.HasSchema)
	assert.Equal(t, SourceBuiltIn, info.Source)
	assert.Equal(t, []schema.IOPort{{Type: schema.PortTarget, Side: model.SideTarget}}, info.Consumes)
	assert.Equal(t, []schema.IOPort{schema.Port(model.OverlayQA, model.SideTarget)}, info.Produces)
	assert.Equal(t, []string{"quality", "ai-powered"}, info.Tags)
	assert.Equal(t, []string{"target-language", "credentials"}, info.Requires)
}

func TestRegisterWithSchema_NilSchema(t *testing.T) {
	reg := NewToolRegistry()
	reg.RegisterWithSchema("no-schema", func() tool.Tool { return &stubTool{} }, nil)

	infos := reg.ListWithSchemas()
	require.Len(t, infos, 1)

	info := infos[0]
	assert.Equal(t, ToolID("no-schema"), info.Name)
	assert.False(t, info.HasSchema)
	assert.Empty(t, info.Consumes)
	assert.Empty(t, info.Tags)
	assert.Empty(t, info.Requires)
}

func TestRegisterWithSchema_EmptyMeta(t *testing.T) {
	reg := NewToolRegistry()

	s := &schema.ComponentSchema{
		Title:    "Minimal",
		ToolMeta: &schema.ToolMeta{ID: "minimal"},
	}

	reg.RegisterWithSchema("minimal", func() tool.Tool { return &stubTool{} }, s)

	info := reg.ListWithSchemas()[0]
	assert.Empty(t, info.Category)
	assert.Empty(t, info.Consumes)
	assert.Empty(t, info.Produces)
	assert.Empty(t, info.Tags)
	assert.Empty(t, info.Requires)
}

func TestSchema_ReturnsSchema(t *testing.T) {
	reg := NewToolRegistry()

	s := &schema.ComponentSchema{Title: "Test"}
	reg.RegisterWithSchema("test", func() tool.Tool { return &stubTool{} }, s)

	got := reg.Schema("test")
	assert.NotNil(t, got)
	assert.Equal(t, "Test", got.Title)
}

func TestSchema_ReturnsNilForUnknown(t *testing.T) {
	reg := NewToolRegistry()
	assert.Nil(t, reg.Schema("nonexistent"))
}

func TestNewTool_CreatesInstance(t *testing.T) {
	reg := NewToolRegistry()
	reg.Register("test", func() tool.Tool { return &stubTool{} })

	got, err := reg.NewTool("test")
	require.NoError(t, err)
	assert.NotNil(t, got)
}

func TestNewTool_ErrorForUnknown(t *testing.T) {
	reg := NewToolRegistry()
	_, err := reg.NewTool("nonexistent")
	require.Error(t, err)
}

func TestToolInfo_ReturnsInfo(t *testing.T) {
	reg := NewToolRegistry()
	reg.RegisterWithSchema("test", func() tool.Tool { return &stubTool{} }, &schema.ComponentSchema{
		Title: "Test",
		ToolMeta: &schema.ToolMeta{
			ID:            "test",
			Category:      "validate",
			Cardinality:   schema.Bilingual,
			DefaultLocale: "qps",
			Produces:      []schema.IOPort{schema.Port(model.OverlayQA, model.SideTarget)},
			SideEffects:   []schema.SideEffect{schema.SideEffectTMRead},
		},
	})

	info := reg.ToolInfo("test")
	require.NotNil(t, info)
	assert.Equal(t, ToolID("test"), info.Name)
	assert.Equal(t, schema.Bilingual, info.Cardinality)
	assert.Equal(t, model.LocaleID("qps"), info.DefaultLocale)
	assert.Equal(t, []schema.IOPort{schema.Port(model.OverlayQA, model.SideTarget)}, info.Produces)
	assert.Equal(t, []schema.SideEffect{schema.SideEffectTMRead}, info.SideEffects)
}

func TestToolInfo_ReturnsNilForUnknown(t *testing.T) {
	reg := NewToolRegistry()
	assert.Nil(t, reg.ToolInfo("nonexistent"))
}

func TestSetConfigPreprocessor_TransformsConfig(t *testing.T) {
	reg := NewToolRegistry()
	reg.RegisterWithSchema("ai-tool", func() tool.Tool { return &stubTool{} }, &schema.ComponentSchema{
		Title: "AI Tool",
		ToolMeta: &schema.ToolMeta{
			ID:       "ai-tool",
			Requires: []string{"credentials"},
		},
	})
	reg.SetConfigFactory("ai-tool", func(config map[string]any, targetLang string) (tool.Tool, error) {
		// Verify the preprocessor ran before the factory.
		assert.Equal(t, "injected-key", config["apiKey"])
		assert.Equal(t, "anthropic", config["provider"])
		return &stubTool{}, nil
	})

	// Preprocessor injects credentials into the config.
	reg.SetConfigPreprocessor(func(toolName string, requires []string, config map[string]any) (map[string]any, error) {
		assert.Equal(t, "ai-tool", toolName)
		assert.Contains(t, requires, "credentials")
		result := make(map[string]any)
		maps.Copy(result, config)
		result["apiKey"] = "injected-key"
		result["provider"] = "anthropic"
		return result, nil
	})

	_, err := reg.NewToolWithConfig("ai-tool", map[string]any{"batchSize": 10}, "fr")
	require.NoError(t, err)
}

func TestSetConfigPreprocessor_ErrorPropagates(t *testing.T) {
	reg := NewToolRegistry()
	reg.RegisterWithSchema("ai-tool", func() tool.Tool { return &stubTool{} }, &schema.ComponentSchema{
		Title:    "AI Tool",
		ToolMeta: &schema.ToolMeta{ID: "ai-tool"},
	})
	reg.SetConfigFactory("ai-tool", func(config map[string]any, targetLang string) (tool.Tool, error) {
		t.Fatal("factory should not be called when preprocessor errors")
		return nil, nil
	})

	reg.SetConfigPreprocessor(func(toolName string, requires []string, config map[string]any) (map[string]any, error) {
		return nil, assert.AnError
	})

	_, err := reg.NewToolWithConfig("ai-tool", map[string]any{}, "fr")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ai-tool config")
}

func TestSetConfigPreprocessor_SkippedWithoutConfigFactory(t *testing.T) {
	reg := NewToolRegistry()
	called := false
	reg.Register("simple-tool", func() tool.Tool { return &stubTool{} })

	reg.SetConfigPreprocessor(func(toolName string, requires []string, config map[string]any) (map[string]any, error) {
		called = true
		return config, nil
	})

	// Tool without ConfigFactory should fall back to zero-arg Factory.
	got, err := reg.NewToolWithConfig("simple-tool", map[string]any{}, "")
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.False(t, called, "preprocessor should not run when no ConfigFactory is set")
}

func TestNewToolWithConfig_UsesConfigFactory(t *testing.T) {
	reg := NewToolRegistry()
	reg.Register("cfg-tool", func() tool.Tool { return &stubTool{} })
	reg.SetConfigFactory("cfg-tool", func(config map[string]any, targetLang string) (tool.Tool, error) {
		assert.Equal(t, "value", config["key"])
		assert.Equal(t, "de", targetLang)
		return &stubTool{}, nil
	})

	_, err := reg.NewToolWithConfig("cfg-tool", map[string]any{"key": "value"}, "de")
	require.NoError(t, err)
}

func TestRegisterWithSchema_PropagatesIOContract(t *testing.T) {
	reg := NewToolRegistry()
	reg.RegisterWithSchema("io-tool", func() tool.Tool { return &stubTool{} }, &schema.ComponentSchema{
		Title: "IO Tool",
		ToolMeta: &schema.ToolMeta{
			ID:          "io-tool",
			Cardinality: schema.Multilingual,
			Produces:    []schema.IOPort{{Type: model.AnnoComparison, Side: model.SideTarget}},
			SideEffects: []schema.SideEffect{schema.SideEffectAnalytics},
		},
	})

	infos := reg.ListWithSchemas()
	require.Len(t, infos, 1)
	assert.Equal(t, schema.Multilingual, infos[0].Cardinality)
	assert.Equal(t, []schema.IOPort{{Type: model.AnnoComparison, Side: model.SideTarget}}, infos[0].Produces)
	assert.Equal(t, []schema.SideEffect{schema.SideEffectAnalytics}, infos[0].SideEffects)
}
