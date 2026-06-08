package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/ai/ner"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockNERProvider implements ner.Provider for testing.
type mockNERProvider struct {
	entities map[string][]ner.DetectedEntity // keyed by text prefix
}

func (m *mockNERProvider) Name() string                       { return "mock-ner" }
func (m *mockNERProvider) Close() error                       { return nil }
func (m *mockNERProvider) SupportedLocales() []model.LocaleID { return nil }

func (m *mockNERProvider) DetectEntities(_ context.Context, req ner.Request) (*ner.Response, error) {
	if m.entities != nil {
		for prefix, ents := range m.entities {
			if len(req.Text) >= len(prefix) && req.Text[:len(prefix)] == prefix {
				return &ner.Response{Entities: ents}, nil
			}
		}
	}
	return &ner.Response{}, nil
}

func (m *mockNERProvider) DetectEntitiesBatch(_ context.Context, reqs []ner.Request) ([]ner.Response, error) {
	results := make([]ner.Response, len(reqs))
	for i, req := range reqs {
		resp, err := m.DetectEntities(context.Background(), req)
		if err != nil {
			return nil, err
		}
		results[i] = *resp
	}
	return results, nil
}

// extractionResponse builds a mock LLM extraction response JSON.
func extractionResponse(blocks []extractionBlock) string {
	result := struct {
		Blocks []extractionBlock `json:"blocks"`
	}{Blocks: blocks}
	b, _ := json.Marshal(result)
	return string(b)
}

type extractionBlock struct {
	BlockID        string              `json:"block_id"`
	Entities       []extractionEntity  `json:"entities"`
	TermCandidates []extractionTermCan `json:"term_candidates"`
}

type extractionEntity struct {
	Text       string  `json:"text"`
	Type       string  `json:"type"`
	DNT        bool    `json:"dnt"`
	Offset     int     `json:"offset"`
	Length     int     `json:"length"`
	Confidence float64 `json:"confidence"`
}

type extractionTermCan struct {
	Text            string  `json:"text"`
	Definition      string  `json:"definition"`
	Category        string  `json:"category"`
	Translatability string  `json:"translatability"`
	Confidence      float64 `json:"confidence"`
	Offset          int     `json:"offset"`
	Length          int     `json:"length"`
}

func TestAIEntityExtractTool_LLMOnly(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(_ context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		assert.Equal(t, "extraction_result", schema.Name)
		return &aiprovider.ChatResponse{
			Content: extractionResponse([]extractionBlock{
				{
					BlockID: "tu1",
					Entities: []extractionEntity{
						{Text: "John Smith", Type: "person", DNT: true, Offset: 0, Length: 10, Confidence: 0.95},
					},
					TermCandidates: []extractionTermCan{
						{Text: "Dashboard", Definition: "Main overview screen", Category: "ui", Translatability: "consistent", Confidence: 0.88, Offset: 20, Length: 9},
					},
				},
			}),
			Model: "test",
		}, nil
	}

	tool := tools.NewAIEntityExtractTool(mock, nil, tools.AIEntityExtractConfig{
		Locale: "en-US",
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "John Smith uses the Dashboard daily")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	// Check entity annotation.
	entitySpan := resultBlock.FacetSpan(model.FacetEntity, "entity:0")
	require.NotNil(t, entitySpan, "should have entity:0 span")
	entity := entitySpan.Value.(*model.EntityAnnotation)
	assert.Equal(t, "John Smith", entity.Text)
	assert.Equal(t, model.EntityPerson, entity.Type)
	assert.True(t, entity.DNT)
	assert.Equal(t, model.ExtractionSourceLLM, entity.Source)

	// Check term candidate annotation.
	tcSpan := resultBlock.FacetSpan(model.FacetTermCandidate, "term-candidate:0")
	require.NotNil(t, tcSpan, "should have term-candidate:0 span")
	tc := tcSpan.Value.(*model.TermCandidateAnnotation)
	assert.Equal(t, "Dashboard", tc.Text)
	assert.Equal(t, "Main overview screen", tc.Definition)
	assert.Equal(t, model.TermCategoryUI, tc.Category)
	assert.Equal(t, model.TranslatabilityConsistent, tc.Translatability)
	assert.Equal(t, model.CandidateStatusPending, tc.Status)
	assert.Equal(t, model.ExtractionSourceLLM, tc.Source)
}

func TestAIEntityExtractTool_WithNER(t *testing.T) {
	nerProv := &mockNERProvider{
		entities: map[string][]ner.DetectedEntity{
			"John": {
				{Text: "John Smith", Type: model.EntityPerson, Confidence: 0.95, Offset: 0, Length: 10},
				{Text: "March 15", Type: model.EntityDate, Confidence: 0.99, Offset: 25, Length: 8},
			},
		},
	}

	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(_ context.Context, _ []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: extractionResponse([]extractionBlock{
				{
					BlockID: "tu1",
					Entities: []extractionEntity{
						// LLM also finds John Smith — should prefer LLM classification.
						{Text: "John Smith", Type: "person", DNT: true, Offset: 0, Length: 10, Confidence: 0.97},
					},
					TermCandidates: []extractionTermCan{
						{Text: "Sprint", Definition: "Agile iteration", Category: "technical", Translatability: "consistent", Confidence: 0.85, Offset: 40, Length: 6},
					},
				},
			}),
			Model: "test",
		}, nil
	}

	tool := tools.NewAIEntityExtractTool(mock, nerProv, tools.AIEntityExtractConfig{
		Locale: "en-US",
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "John Smith ordered on March 15 during Sprint 5")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	// entity:0 should be LLM's "John Smith" (preferred over NER for same position).
	span0 := resultBlock.FacetSpan(model.FacetEntity, "entity:0")
	require.NotNil(t, span0)
	entityAnn0, _ := any(span0.Value).(*model.EntityAnnotation)
	assert.Equal(t, "John Smith", entityAnn0.Text)
	assert.Equal(t, model.ExtractionSourceLLM, entityAnn0.Source)

	// entity:1 should be NER's "March 15" (not in LLM result, so NER fills in).
	span1 := resultBlock.FacetSpan(model.FacetEntity, "entity:1")
	require.NotNil(t, span1)
	entityAnn1, _ := any(span1.Value).(*model.EntityAnnotation)
	assert.Equal(t, "March 15", entityAnn1.Text)
	assert.Equal(t, model.EntityDate, entityAnn1.Type)
	assert.Equal(t, model.ExtractionSourceNER, entityAnn1.Source)

	// Term candidate should be present.
	tcSpan := resultBlock.FacetSpan(model.FacetTermCandidate, "term-candidate:0")
	require.NotNil(t, tcSpan)
	termAnn, _ := any(tcSpan.Value).(*model.TermCandidateAnnotation)
	assert.Equal(t, "Sprint", termAnn.Text)
}

func TestAIEntityExtractTool_SkipsKnownTerms(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(_ context.Context, _ []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: extractionResponse([]extractionBlock{
				{
					BlockID:  "tu1",
					Entities: []extractionEntity{},
					TermCandidates: []extractionTermCan{
						{Text: "Dashboard", Definition: "Known term", Category: "ui", Translatability: "consistent", Confidence: 0.9, Offset: 0, Length: 9},
						{Text: "Workflow", Definition: "New term", Category: "technical", Translatability: "consistent", Confidence: 0.85, Offset: 15, Length: 8},
					},
				},
			}),
			Model: "test",
		}, nil
	}

	tool := tools.NewAIEntityExtractTool(mock, nil, tools.AIEntityExtractConfig{
		Locale:     "en-US",
		KnownTerms: []string{"Dashboard"}, // Already in termbase.
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Dashboard and Workflow settings")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	// Only "Workflow" should be a candidate (Dashboard is known).
	tcSpan := resultBlock.FacetSpan(model.FacetTermCandidate, "term-candidate:0")
	require.NotNil(t, tcSpan)
	tc, _ := any(tcSpan.Value).(*model.TermCandidateAnnotation)
	assert.Equal(t, "Workflow", tc.Text, "Dashboard should be filtered, leaving only Workflow")
}

func TestAIEntityExtractTool_SkipsEmptyBlocks(t *testing.T) {
	mock := aiprovider.NewMockProvider()

	tool := tools.NewAIEntityExtractTool(mock, nil, tools.AIEntityExtractConfig{
		Locale: "en-US",
	})

	ctx := t.Context()
	in := make(chan *model.Part, 2)
	out := make(chan *model.Part, 2)

	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "")}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu2", "   ")}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	// Both blocks should pass through without LLM calls.
	<-out
	<-out
	assert.Empty(t, mock.ChatStructuredCalls)
}

func TestAIEntityExtractTool_PassesThroughNonBlocks(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(_ context.Context, _ []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: extractionResponse([]extractionBlock{{BlockID: "tu1", Entities: []extractionEntity{}, TermCandidates: []extractionTermCan{}}}),
			Model:   "test",
		}, nil
	}

	tool := tools.NewAIEntityExtractTool(mock, nil, tools.AIEntityExtractConfig{
		Locale: "en-US",
	})

	ctx := t.Context()
	in := make(chan *model.Part, 5)
	out := make(chan *model.Part, 5)

	layer := &model.Layer{ID: "doc1", Format: "test"}
	in <- &model.Part{Type: model.PartLayerStart, Resource: layer}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
	in <- &model.Part{Type: model.PartData, Resource: &model.Data{ID: "d1"}}
	in <- &model.Part{Type: model.PartLayerEnd, Resource: layer}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	close(out)

	var parts []*model.Part
	for p := range out {
		parts = append(parts, p)
	}
	assert.Len(t, parts, 4)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartBlock, parts[1].Type)
	assert.Equal(t, model.PartData, parts[2].Type)
	assert.Equal(t, model.PartLayerEnd, parts[3].Type)
}

func TestAIEntityExtractTool_BatchMode(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(_ context.Context, _ []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: extractionResponse([]extractionBlock{
				{BlockID: "tu1", Entities: []extractionEntity{{Text: "Alice", Type: "person", DNT: true, Offset: 0, Length: 5, Confidence: 0.9}}, TermCandidates: []extractionTermCan{}},
				{BlockID: "tu2", Entities: []extractionEntity{}, TermCandidates: []extractionTermCan{{Text: "API", Definition: "Application Programming Interface", Category: "technical", Translatability: "dnt", Confidence: 0.95, Offset: 4, Length: 3}}},
				{BlockID: "tu3", Entities: []extractionEntity{}, TermCandidates: []extractionTermCan{}},
			}),
			Model: "test",
		}, nil
	}

	tool := tools.NewAIEntityExtractTool(mock, nil, tools.AIEntityExtractConfig{
		Locale:      "en-US",
		BatchSize:   10,
		Concurrency: 2,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 5)
	out := make(chan *model.Part, 5)

	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Alice said hello")}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu2", "The API is fast")}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu3", "No entities here")}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	close(out)

	var parts []*model.Part
	for p := range out {
		parts = append(parts, p)
	}
	require.Len(t, parts, 3)

	// One ChatStructured call for the batch.
	assert.Len(t, mock.ChatStructuredCalls, 1)

	// Block 1: entity.
	b1 := parts[0].Resource.(*model.Block)
	assert.NotNil(t, b1.FacetSpan(model.FacetEntity, "entity:0"))

	// Block 2: term candidate.
	b2 := parts[1].Resource.(*model.Block)
	assert.NotNil(t, b2.FacetSpan(model.FacetTermCandidate, "term-candidate:0"))
}
