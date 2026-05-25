package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProfileResolver implements brand.ProfileResolver for testing.
type mockProfileResolver struct {
	profile *brand.VoiceProfile
}

func (m *mockProfileResolver) ResolveProfile(_ context.Context, _ brand.ResolveContext) (*brand.VoiceProfile, error) {
	return m.profile, nil
}

func TestBrandVocabCheckForbiddenTerms(t *testing.T) {
	t.Parallel()
	profile := &brand.VoiceProfile{
		ID: "test-profile",
		Vocabulary: brand.VocabularyRules{
			ForbiddenTerms: []brand.TermRule{
				{Term: "cheap", Replacement: "affordable", Note: "avoid negative connotation"},
			},
		},
	}

	tool := tools.NewBrandVocabCheckTool(profile, nil)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "This is a cheap product")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	findingsStr := resultBlock.Properties["brand-vocab-findings"]
	require.NotEmpty(t, findingsStr)

	var findings []brand.BrandVoiceFinding
	err = json.Unmarshal([]byte(findingsStr), &findings)
	require.NoError(t, err)
	require.Len(t, findings, 1)

	assert.Equal(t, brand.DimensionVocabulary, findings[0].Dimension)
	assert.Equal(t, brand.SeverityMajor, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "cheap")
	assert.Contains(t, findings[0].Suggestion, "affordable")
	assert.Equal(t, 0, findings[0].Position.StartRun)
	assert.Equal(t, 10, findings[0].Position.StartOffset)
	assert.Equal(t, 15, findings[0].Position.EndOffset)
}

func TestBrandVocabCheckCompetitorTerms(t *testing.T) {
	t.Parallel()
	profile := &brand.VoiceProfile{
		Vocabulary: brand.VocabularyRules{
			CompetitorTerms: []brand.TermRule{
				{Term: "Acme Corp", Replacement: "our platform"},
			},
		},
	}

	tool := tools.NewBrandVocabCheckTool(profile, nil)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Unlike Acme Corp, we deliver")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	findingsStr := resultBlock.Properties["brand-vocab-findings"]
	require.NotEmpty(t, findingsStr)

	var findings []brand.BrandVoiceFinding
	err = json.Unmarshal([]byte(findingsStr), &findings)
	require.NoError(t, err)
	require.Len(t, findings, 1)

	assert.Equal(t, brand.SeverityCritical, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "Competitor term")
	assert.Contains(t, findings[0].Suggestion, "our platform")
}

func TestBrandVocabCheckPreferredTermSuggestion(t *testing.T) {
	t.Parallel()
	profile := &brand.VoiceProfile{
		Vocabulary: brand.VocabularyRules{
			ForbiddenTerms: []brand.TermRule{
				{Term: "users", Replacement: "customers"},
			},
		},
	}

	tool := tools.NewBrandVocabCheckTool(profile, nil)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Our users love this feature")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	var findings []brand.BrandVoiceFinding
	err = json.Unmarshal([]byte(resultBlock.Properties["brand-vocab-findings"]), &findings)
	require.NoError(t, err)
	require.Len(t, findings, 1)

	assert.Contains(t, findings[0].Suggestion, "customers")
}

func TestBrandVocabCheckNoViolations(t *testing.T) {
	t.Parallel()
	profile := &brand.VoiceProfile{
		Vocabulary: brand.VocabularyRules{
			ForbiddenTerms: []brand.TermRule{
				{Term: "cheap"},
			},
			CompetitorTerms: []brand.TermRule{
				{Term: "Acme Corp"},
			},
		},
	}

	tool := tools.NewBrandVocabCheckTool(profile, nil)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "This is an affordable and quality product")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	assert.Empty(t, resultBlock.Properties["brand-vocab-findings"])
}

func TestBrandVocabCheckSkipsEmptyText(t *testing.T) {
	t.Parallel()
	profile := &brand.VoiceProfile{
		Vocabulary: brand.VocabularyRules{
			ForbiddenTerms: []brand.TermRule{
				{Term: "cheap"},
			},
		},
	}

	tool := tools.NewBrandVocabCheckTool(profile, nil)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "   ")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)
	assert.Empty(t, resultBlock.Properties["brand-vocab-findings"])
}

func TestBrandVocabCheckCaseInsensitive(t *testing.T) {
	t.Parallel()
	profile := &brand.VoiceProfile{
		Vocabulary: brand.VocabularyRules{
			ForbiddenTerms: []brand.TermRule{
				{Term: "cheap"},
			},
		},
	}

	tool := tools.NewBrandVocabCheckTool(profile, nil)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "This is CHEAP stuff")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	var findings []brand.BrandVoiceFinding
	err = json.Unmarshal([]byte(resultBlock.Properties["brand-vocab-findings"]), &findings)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "CHEAP", findings[0].OriginalText)
}

func TestBrandVocabCheckWithResolver(t *testing.T) {
	t.Parallel()
	profile := &brand.VoiceProfile{
		ID: "resolved-vocab",
		Vocabulary: brand.VocabularyRules{
			ForbiddenTerms: []brand.TermRule{
				{Term: "cheap", Replacement: "affordable"},
			},
		},
	}

	resolver := &mockProfileResolver{profile: profile}
	rc := brand.ResolveContext{ExplicitProfileID: "resolved-vocab"}

	tool := tools.NewBrandVocabCheckToolWithResolver(resolver, rc, nil)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "This is a cheap product")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	ann, ok := resultBlock.Annotations["brand-voice"]
	require.True(t, ok)
	bva := ann.(*brand.BrandVoiceAnnotation)
	assert.Equal(t, "resolved-vocab", bva.ProfileID)
	assert.Len(t, bva.Findings, 1)
}

func TestBrandVocabCheckWithResolverNilProfile(t *testing.T) {
	t.Parallel()
	resolver := &mockProfileResolver{profile: nil}
	rc := brand.ResolveContext{}

	tool := tools.NewBrandVocabCheckToolWithResolver(resolver, rc, nil)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "This is a cheap product")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	// No findings — profile was nil so no vocab rules to check.
	assert.Empty(t, resultBlock.Properties["brand-vocab-findings"])
}

func TestBrandVocabCheckAddsAnnotation(t *testing.T) {
	t.Parallel()
	profile := &brand.VoiceProfile{
		ID: "voice-1",
		Vocabulary: brand.VocabularyRules{
			ForbiddenTerms: []brand.TermRule{
				{Term: "stuff"},
			},
		},
	}

	tool := tools.NewBrandVocabCheckTool(profile, nil)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Buy our stuff")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	ann, ok := resultBlock.Annotations["brand-voice"]
	require.True(t, ok)

	bva := ann.(*brand.BrandVoiceAnnotation)
	assert.Equal(t, "voice-1", bva.ProfileID)
	assert.Less(t, bva.Score, 100)
	assert.Len(t, bva.Findings, 1)
}
