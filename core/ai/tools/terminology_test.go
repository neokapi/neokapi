package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// termExtractBlock runs the terminology tool over one block with the given
// source text and scripted structured response, returning the resulting block.
func termExtractBlock(t *testing.T, source string, mock *aiprovider.MockProvider) *model.Block {
	t.Helper()
	tl := tools.NewAITerminologyTool(mock, tools.AITerminologyConfig{
		Locale: model.LocaleEnglish,
		Domain: "technology",
	})

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", source)}
	close(in)

	require.NoError(t, tl.Process(t.Context(), in, out))
	return (<-out).Resource.(*model.Block)
}

// TestAITerminology_HappyPath: valid structured output lands as JSON in the
// "terminology" block property.
func TestAITerminology_HappyPath(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"terms":[{"term":"gRPC","definition":"an RPC framework","domain":"technology"},{"term":"protobuf","definition":"a serialization format","domain":"technology"}]}`,
			Model:   "test",
		}, nil
	}

	block := termExtractBlock(t, "gRPC services exchange protobuf messages", mock)

	var terms []tools.TermEntry
	require.NoError(t, json.Unmarshal([]byte(block.Properties["terminology"]), &terms))
	require.Len(t, terms, 2)
	assert.Equal(t, "gRPC", terms[0].Term)
	assert.Equal(t, "an RPC framework", terms[0].Definition)
	assert.Equal(t, "protobuf", terms[1].Term)
}

// TestAITerminology_MalformedResponseIsResilient: a non-JSON model response
// must not fail the pipeline; the block simply gets no terminology property.
func TestAITerminology_MalformedResponseIsResilient(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{Content: "no terms worth mentioning here", Model: "test"}, nil
	}

	block := termExtractBlock(t, "Some technical text", mock)

	_, has := block.Properties["terminology"]
	assert.False(t, has, "malformed output must not set the terminology property")
}

// TestAITerminology_EmptyTermsSetNoProperty: an explicit empty terms array
// also leaves the property unset.
func TestAITerminology_EmptyTermsSetNoProperty(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{Content: `{"terms":[]}`, Model: "test"}, nil
	}

	block := termExtractBlock(t, "Nothing notable", mock)

	_, has := block.Properties["terminology"]
	assert.False(t, has)
}

// TestAITerminology_SkipsBlankSource: whitespace-only source text never
// reaches the provider.
func TestAITerminology_SkipsBlankSource(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	termExtractBlock(t, "   \n\t", mock)
	assert.Empty(t, mock.ChatStructuredCalls, "blank source must not call the provider")
}

// TestAITerminology_ErrorPropagation: a provider failure surfaces as a tool
// error wrapped with the tool name.
func TestAITerminology_ErrorPropagation(t *testing.T) {
	provErr := errors.New("quota exceeded")
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return nil, provErr
	}

	tl := tools.NewAITerminologyTool(mock, tools.AITerminologyConfig{Locale: model.LocaleEnglish})

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Some text")}
	close(in)

	err := tl.Process(t.Context(), in, out)
	require.Error(t, err)
	assert.ErrorIs(t, err, provErr)
	assert.Contains(t, err.Error(), "term-extract:")
}
