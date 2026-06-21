package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// text builds a single TextRun model.Run, mirroring the segment package tests.
func text(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }

// ph builds a placeholder (inline-code) run.
func ph(id string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{ID: id, Equiv: "{" + id + "}"}}
}

// segText renders the runs covered by a span as plain text, for assertions.
func segText(runs []model.Run, sp model.Span) string {
	return model.RunsText(sp.Range.ExtractRuns(runs))
}

// chunkProvider returns a mock LLM provider whose structured chat always
// responds with the given chunk list encoded as {"chunks": [...]}.
func chunkProvider(t *testing.T, chunks ...string) *aiprovider.MockProvider {
	t.Helper()
	p := aiprovider.NewMockProvider()
	p.ChatStructuredFunc = func(_ context.Context, _ []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		b, err := json.Marshal(chunkResult{Chunks: chunks})
		require.NoError(t, err)
		return &aiprovider.ChatResponse{Content: string(b), Model: "mock-model"}, nil
	}
	return p
}

func newSegmenter(p aiprovider.LLMProvider) *llmSegmenter {
	return &llmSegmenter{provider: p}
}

func TestLLMSegment_CleanSplit(t *testing.T) {
	runs := []model.Run{text("One. Two. Three.")}
	p := chunkProvider(t, "One.", "Two.", "Three.")
	s := newSegmenter(p)

	spans, err := s.Segment(context.Background(), runs, "en-US")
	require.NoError(t, err)
	require.Len(t, spans, 3)
	assert.Equal(t, "One.", segText(runs, spans[0]))
	assert.Equal(t, " Two.", segText(runs, spans[1]))
	assert.Equal(t, " Three.", segText(runs, spans[2]))
}

func TestLLMSegment_WhitespaceDrift(t *testing.T) {
	// The model returns chunks with collapsed/extra whitespace and stray
	// leading/trailing spaces. Alignment must still locate each chunk in the
	// original text and produce the correct boundaries.
	runs := []model.Run{text("Hello world.  How are you?\tFine, thanks.")}
	p := chunkProvider(t,
		"Hello   world.",   // collapsed internal double space
		"  How are you?  ", // extra surrounding whitespace
		"Fine, thanks.",    // exact
	)
	s := newSegmenter(p)

	spans, err := s.Segment(context.Background(), runs, "en")
	require.NoError(t, err)
	require.Len(t, spans, 3)
	assert.Equal(t, "Hello world.", segText(runs, spans[0]))
	// Inter-chunk whitespace attaches to the following segment's leading edge.
	assert.Equal(t, "  How are you?", segText(runs, spans[1]))
	assert.Equal(t, "\tFine, thanks.", segText(runs, spans[2]))
}

func TestLLMSegment_SplitAcrossInlineCode(t *testing.T) {
	// Codes are zero-width in the masked text; chunk alignment operates on the
	// masked text and the resulting spans still anchor to the right runs.
	runs := []model.Run{text("Hi there. "), ph("br"), text("Bye now.")}
	fl := segment.Flatten(runs, segment.MaskOptions{})
	require.Equal(t, "Hi there. Bye now.", fl.Text())

	p := chunkProvider(t, "Hi there.", "Bye now.")
	s := newSegmenter(p)

	spans, err := s.Segment(context.Background(), runs, "en")
	require.NoError(t, err)
	require.Len(t, spans, 2)
	assert.Equal(t, "Hi there.", segText(runs, spans[0]))
	// The break lands after "Hi there." so the trailing space and the
	// zero-width placeholder ride along with the second segment. The plain-text
	// flattening drops the code; the run sub-sequence keeps it.
	second := spans[1].Range.ExtractRuns(runs)
	assert.Equal(t, " Bye now.", model.RunsText(second))
	var sawPlaceholder bool
	for _, r := range second {
		if r.Ph != nil && r.Ph.ID == "br" {
			sawPlaceholder = true
		}
	}
	assert.True(t, sawPlaceholder, "placeholder run should ride with the second segment")
}

func TestLLMSegment_GarbageDegradesToWholeBlock(t *testing.T) {
	// The model returns chunks that do not appear in the source. Alignment
	// fails and the engine returns nil (whole block, no segmentation).
	runs := []model.Run{text("One. Two. Three.")}
	p := chunkProvider(t, "completely", "unrelated", "tokens")
	s := newSegmenter(p)

	spans, err := s.Segment(context.Background(), runs, "en")
	require.NoError(t, err)
	assert.Nil(t, spans)
}

func TestLLMSegment_InvalidJSONDegradesToWholeBlock(t *testing.T) {
	runs := []model.Run{text("One. Two.")}
	p := aiprovider.NewMockProvider()
	p.ChatStructuredFunc = func(_ context.Context, _ []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{Content: "not json at all", Model: "mock-model"}, nil
	}
	s := newSegmenter(p)

	spans, err := s.Segment(context.Background(), runs, "en")
	require.NoError(t, err)
	assert.Nil(t, spans)
}

func TestLLMSegment_SingleChunkNoSegmentation(t *testing.T) {
	// A single chunk means "no interior boundary" — whole block.
	runs := []model.Run{text("One. Two. Three.")}
	p := chunkProvider(t, "One. Two. Three.")
	s := newSegmenter(p)

	spans, err := s.Segment(context.Background(), runs, "en")
	require.NoError(t, err)
	assert.Nil(t, spans)
}

func TestLLMSegment_EmptyInput(t *testing.T) {
	// Empty / whitespace-only input returns nil without calling the provider.
	p := aiprovider.NewMockProvider()
	called := false
	p.ChatStructuredFunc = func(_ context.Context, _ []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		called = true
		return &aiprovider.ChatResponse{Content: `{"chunks":[]}`}, nil
	}
	s := newSegmenter(p)

	for _, runs := range [][]model.Run{
		nil,
		{text("")},
		{text("   \t\n ")},
	} {
		spans, err := s.Segment(context.Background(), runs, "en")
		require.NoError(t, err)
		assert.Nil(t, spans)
	}
	assert.False(t, called, "provider must not be called for empty input")
}

func TestLLMSegment_ProviderErrorDegradesToWholeBlock(t *testing.T) {
	runs := []model.Run{text("One. Two.")}
	p := aiprovider.NewMockProvider()
	p.ChatStructuredFunc = func(_ context.Context, _ []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return nil, assert.AnError
	}
	s := newSegmenter(p)

	spans, err := s.Segment(context.Background(), runs, "en")
	require.NoError(t, err)
	assert.Nil(t, spans)
}

func TestLLMSegment_EngineRegistered(t *testing.T) {
	// init() must have registered the "llm" engine and it reports the
	// llm-chunk layer.
	assert.True(t, segment.HasEngine("llm"))

	s := newSegmenter(aiprovider.NewMockProvider())
	assert.Equal(t, segment.LayerLLMChunk, s.Layer())
}

func TestLLMSegment_FactoryRequiresProvider(t *testing.T) {
	// A known provider with no credentials still constructs (validation of
	// credentials happens at call time inside the provider); an unknown
	// provider id is an error.
	seg, err := newLLMSegmenter(segment.BaseConfig{}, &LLMParams{Provider: "definitely-not-a-provider"})
	require.Error(t, err)
	assert.Nil(t, seg)

	seg, err = newLLMSegmenter(segment.BaseConfig{}, &LLMParams{Provider: string(aiprovider.Demo)})
	require.NoError(t, err)
	require.NotNil(t, seg)
	assert.Equal(t, segment.LayerLLMChunk, seg.Layer())
}
