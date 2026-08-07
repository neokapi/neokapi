package tools_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// blockIDInPrompt matches the block ids the extraction prompt sends, so a mock
// can answer the batch it was actually given.
var blockIDInPrompt = regexp.MustCompile(`Block \(id: ([^)]+)\):`)

// Extraction reuses the batch machinery translation does, and had the same hole
// at the other end of it: a reply stopped at the output cap arrived as a JSON
// fragment, failed to unmarshal, and took the whole run down as "unmarshal
// extraction result" — a message that blames the model for a batch we chose.
func TestAIEntityExtract_TruncatedBatchSplitsInsteadOfAborting(t *testing.T) {
	var mu sync.Mutex
	var batchSizes []int

	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(_ context.Context, messages []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		matches := blockIDInPrompt.FindAllStringSubmatch(messages[len(messages)-1].Text(), -1)

		mu.Lock()
		batchSizes = append(batchSizes, len(matches))
		mu.Unlock()

		// More than two blocks is more than this model will emit in one reply.
		if len(matches) > 2 {
			return &aiprovider.ChatResponse{
				Content:   `{"blocks":[{"block_id":"tu0","entities":[{"text":"Ali`,
				Model:     "test",
				Truncated: true,
			}, nil
		}

		blocks := make([]extractionBlock, 0, len(matches))
		for _, m := range matches {
			blocks = append(blocks, extractionBlock{
				BlockID:  m[1],
				Entities: []extractionEntity{{Text: "Alice", Type: "person", DNT: true, Offset: 0, Length: 5, Confidence: 0.9}},
			})
		}
		return &aiprovider.ChatResponse{Content: extractionResponse(blocks), Model: "test"}, nil
	}

	tool := tools.NewAIEntityExtractTool(mock, nil, tools.AIEntityExtractConfig{
		Locale:      "en-US",
		BatchSize:   4,
		Concurrency: 1,
	})

	in := make(chan *model.Part, 4)
	out := make(chan *model.Part, 4)
	for i := range 4 {
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock(fmt.Sprintf("tu%d", i), "Alice said hello")}
	}
	close(in)

	require.NoError(t, tool.Process(t.Context(), in, out))
	close(out)

	var parts []*model.Part
	for p := range out {
		parts = append(parts, p)
	}
	require.Len(t, parts, 4)

	// Every block still carries its entity: the split retry produced the whole
	// result, not half of it.
	for _, p := range parts {
		block := p.Resource.(*model.Block)
		assert.NotNil(t, block.OverlaySpan(model.OverlayEntity, "entity:0"),
			"block %s must carry the entity found in the retried half", block.ID)
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int{4, 2, 2}, batchSizes, "the oversized batch is halved and each half retried")
}

// An extraction batch has to ask for what its reply could need. Without a budget
// the call runs on the provider's own default, and a long batch is cut off before
// it has reported on its last blocks.
func TestAIEntityExtract_BatchCarriesAnOutputBudget(t *testing.T) {
	var got int
	var ok bool

	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, _ []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		got, ok = aiprovider.MaxOutputTokensFrom(ctx)
		return &aiprovider.ChatResponse{Content: extractionResponse(nil), Model: "test"}, nil
	}

	tool := tools.NewAIEntityExtractTool(mock, nil, tools.AIEntityExtractConfig{
		Locale: "en-US", BatchSize: 10, Concurrency: 1,
	})

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", strings.Repeat("A long sentence about Alice. ", 1000))}
	close(in)

	require.NoError(t, tool.Process(t.Context(), in, out))
	close(out)
	<-out

	assert.True(t, ok, "an extraction call must carry an output budget")
	assert.Greater(t, got, aiprovider.ConservativeMaxOutputTokens,
		"a batch this long must ask for more than a provider's bare default")
}
