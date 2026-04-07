package registry

import (
	"context"
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
	assert.Contains(t, names, "test-tool")
}

func TestRegisterWithSchema_PropagatesMetadata(t *testing.T) {
	reg := NewToolRegistry()

	s := &schema.ComponentSchema{
		Title:       "Test Tool",
		Description: "A test tool",
		ToolMeta: &schema.ToolMeta{
			ID:       "test-tool",
			Category: "validate",
			Inputs:   []string{"block", "data"},
			Outputs:  []string{"block"},
			Tags:     []string{"quality", "ai-powered"},
			Requires: []string{"target-language", "credentials"},
		},
	}

	reg.RegisterWithSchema("test-tool", func() tool.Tool { return &stubTool{} }, s)

	infos := reg.ListWithSchemas()
	require.Len(t, infos, 1)

	info := infos[0]
	assert.Equal(t, "test-tool", info.Name)
	assert.Equal(t, "A test tool", info.Description)
	assert.Equal(t, "validate", info.Category)
	assert.True(t, info.HasSchema)
	assert.Equal(t, "built-in", info.Source)
	assert.Equal(t, []string{"block", "data"}, info.Inputs)
	assert.Equal(t, []string{"block"}, info.Outputs)
	assert.Equal(t, []string{"quality", "ai-powered"}, info.Tags)
	assert.Equal(t, []string{"target-language", "credentials"}, info.Requires)
}

func TestRegisterWithSchema_NilSchema(t *testing.T) {
	reg := NewToolRegistry()
	reg.RegisterWithSchema("no-schema", func() tool.Tool { return &stubTool{} }, nil)

	infos := reg.ListWithSchemas()
	require.Len(t, infos, 1)

	info := infos[0]
	assert.Equal(t, "no-schema", info.Name)
	assert.False(t, info.HasSchema)
	assert.Empty(t, info.Inputs)
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
	assert.Empty(t, info.Inputs)
	assert.Empty(t, info.Outputs)
	assert.Empty(t, info.Tags)
	assert.Empty(t, info.Requires)
}

func TestGetSchema_ReturnsSchema(t *testing.T) {
	reg := NewToolRegistry()

	s := &schema.ComponentSchema{Title: "Test"}
	reg.RegisterWithSchema("test", func() tool.Tool { return &stubTool{} }, s)

	got := reg.GetSchema("test")
	assert.NotNil(t, got)
	assert.Equal(t, "Test", got.Title)
}

func TestGetSchema_ReturnsNilForUnknown(t *testing.T) {
	reg := NewToolRegistry()
	assert.Nil(t, reg.GetSchema("nonexistent"))
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

func TestGetToolInfo_ReturnsInfo(t *testing.T) {
	reg := NewToolRegistry()
	reg.RegisterWithSchema("test", func() tool.Tool { return &stubTool{} }, &schema.ComponentSchema{
		Title: "Test",
		ToolMeta: &schema.ToolMeta{
			ID:            "test",
			Category:      "validate",
			Cardinality:   schema.Bilingual,
			DefaultLocale: "qps",
			Produces:      []schema.AnnotationType{schema.AnnotationQAIssues},
			SideEffects:   []schema.SideEffect{schema.SideEffectTMRead},
		},
	})

	info := reg.GetToolInfo("test")
	require.NotNil(t, info)
	assert.Equal(t, "test", info.Name)
	assert.Equal(t, schema.Bilingual, info.Cardinality)
	assert.Equal(t, "qps", info.DefaultLocale)
	assert.Equal(t, []schema.AnnotationType{schema.AnnotationQAIssues}, info.Produces)
	assert.Equal(t, []schema.SideEffect{schema.SideEffectTMRead}, info.SideEffects)
}

func TestGetToolInfo_ReturnsNilForUnknown(t *testing.T) {
	reg := NewToolRegistry()
	assert.Nil(t, reg.GetToolInfo("nonexistent"))
}

func TestRegisterWithSchema_PropagatesIOContract(t *testing.T) {
	reg := NewToolRegistry()
	reg.RegisterWithSchema("io-tool", func() tool.Tool { return &stubTool{} }, &schema.ComponentSchema{
		Title: "IO Tool",
		ToolMeta: &schema.ToolMeta{
			ID:            "io-tool",
			Cardinality:   schema.Multilingual,
			Produces:      []schema.AnnotationType{schema.AnnotationComparison},
			SideEffects:   []schema.SideEffect{schema.SideEffectAnalytics},
		},
	})

	infos := reg.ListWithSchemas()
	require.Len(t, infos, 1)
	assert.Equal(t, schema.Multilingual, infos[0].Cardinality)
	assert.Equal(t, []schema.AnnotationType{schema.AnnotationComparison}, infos[0].Produces)
	assert.Equal(t, []schema.SideEffect{schema.SideEffectAnalytics}, infos[0].SideEffects)
}
