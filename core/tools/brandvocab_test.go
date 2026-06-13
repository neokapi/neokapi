package tools_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/termbase"
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

// fakeTermBase serves a fixed set of LookupAll matches and is otherwise an inert
// termbase.TermBase. It lets the brand-vocab tool's termbase branch run with a
// concept-bearing match under full test control, without a SQLite store.
type fakeTermBase struct {
	matches []termbase.TermMatch
}

func (f *fakeTermBase) LookupAll(context.Context, string, termbase.LookupOptions) ([]termbase.TermMatch, error) {
	return f.matches, nil
}
func (f *fakeTermBase) Lookup(context.Context, string, termbase.LookupOptions) ([]termbase.TermMatch, error) {
	return f.matches, nil
}
func (f *fakeTermBase) AddConcept(context.Context, termbase.Concept) error { return nil }
func (f *fakeTermBase) GetConcept(context.Context, string) (termbase.Concept, bool, error) {
	return termbase.Concept{}, false, nil
}
func (f *fakeTermBase) DeleteConcept(context.Context, string) error { return nil }
func (f *fakeTermBase) Search(context.Context, string, model.LocaleID, model.LocaleID, int, int) ([]termbase.Concept, int, error) {
	return nil, 0, nil
}
func (f *fakeTermBase) Count(context.Context) (int, error)                          { return 0, nil }
func (f *fakeTermBase) Concepts(context.Context) ([]termbase.Concept, error)        { return nil, nil }
func (f *fakeTermBase) AddRelation(context.Context, termbase.ConceptRelation) error { return nil }
func (f *fakeTermBase) DeleteRelation(context.Context, string) error                { return nil }
func (f *fakeTermBase) RelationsOf(context.Context, string, *graph.Scope) ([]termbase.ConceptRelation, error) {
	return nil, nil
}
func (f *fakeTermBase) ListRelations(context.Context, *graph.Scope) ([]termbase.ConceptRelation, error) {
	return nil, nil
}
func (f *fakeTermBase) Close() error { return nil }

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

	bvAnn, bvOK := model.AnnoAs[*brand.BrandVoiceAnnotation](resultBlock, "brand-voice")
	require.True(t, bvOK)
	findings := bvAnn.Findings
	require.Len(t, findings, 1)

	assert.Equal(t, string(brand.DimensionVocabulary), findings[0].Category)
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

	bvAnn, bvOK := model.AnnoAs[*brand.BrandVoiceAnnotation](resultBlock, "brand-voice")
	require.True(t, bvOK)
	findings := bvAnn.Findings
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

	bvAnn, bvOK := model.AnnoAs[*brand.BrandVoiceAnnotation](resultBlock, "brand-voice")
	require.True(t, bvOK)
	findings := bvAnn.Findings
	require.Len(t, findings, 1)

	assert.Contains(t, findings[0].Suggestion, "customers")
}

func TestBrandVocabCheckEmitsConceptIDMetadata(t *testing.T) {
	t.Parallel()
	profile := &brand.VoiceProfile{
		Vocabulary: brand.VocabularyRules{
			ForbiddenTerms: []brand.TermRule{
				{Term: "cheap", Replacement: "affordable", ConceptID: "concept-affordable"},
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

	require.NoError(t, tool.Process(ctx, in, out))

	resultBlock := (<-out).Resource.(*model.Block)
	bvAnn, bvOK := model.AnnoAs[*brand.BrandVoiceAnnotation](resultBlock, "brand-voice")
	require.True(t, bvOK)
	require.Len(t, bvAnn.Findings, 1)

	// The concept-backed rule links its finding to the concept story, alongside
	// the existing structured replacement.
	assert.Equal(t, "concept-affordable", bvAnn.Findings[0].Metadata["concept_id"])
	assert.Equal(t, "affordable", bvAnn.Findings[0].Metadata["replacement"])
}

func TestBrandVocabCheckStandaloneOmitsConceptID(t *testing.T) {
	t.Parallel()
	// A standalone profile (no concept on the rule) emits findings without a
	// concept_id metadata key.
	profile := &brand.VoiceProfile{
		Vocabulary: brand.VocabularyRules{
			ForbiddenTerms: []brand.TermRule{
				{Term: "cheap", Replacement: "affordable"},
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

	require.NoError(t, tool.Process(ctx, in, out))

	resultBlock := (<-out).Resource.(*model.Block)
	bvAnn, bvOK := model.AnnoAs[*brand.BrandVoiceAnnotation](resultBlock, "brand-voice")
	require.True(t, bvOK)
	require.Len(t, bvAnn.Findings, 1)

	_, hasConcept := bvAnn.Findings[0].Metadata["concept_id"]
	assert.False(t, hasConcept, "standalone finding must not carry a concept_id")
	// The structured replacement is still present.
	assert.Equal(t, "affordable", bvAnn.Findings[0].Metadata["replacement"])
}

func TestBrandVocabCheckTermbaseConceptID(t *testing.T) {
	t.Parallel()
	// A forbidden brand-vocabulary term found via the termbase carries its
	// knowledge-graph concept; when the concept holds a preferred term in the
	// source locale, that surfaces as the structured replacement — symmetric with
	// the profile path.
	tb := &fakeTermBase{matches: []termbase.TermMatch{{
		Concept: termbase.Concept{
			ID:     "concept-cheap",
			Source: termbase.TermSourceBrandVocabulary,
			Terms: []termbase.Term{
				{Text: "cheap", Locale: "", Status: model.TermForbidden},
				{Text: "affordable", Locale: "", Status: model.TermPreferred},
			},
		},
		Term:     termbase.Term{Text: "cheap", Status: model.TermForbidden},
		Position: model.TextRange{Start: 10, End: 15},
	}}}

	tool := tools.NewBrandVocabCheckTool(nil, tb)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "This is a cheap product")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	require.NoError(t, tool.Process(ctx, in, out))

	resultBlock := (<-out).Resource.(*model.Block)
	bvAnn, bvOK := model.AnnoAs[*brand.BrandVoiceAnnotation](resultBlock, "brand-voice")
	require.True(t, bvOK)
	require.Len(t, bvAnn.Findings, 1)

	f := bvAnn.Findings[0]
	assert.Contains(t, f.Message, "cheap")
	assert.Equal(t, "concept-cheap", f.Metadata["concept_id"],
		"a termbase-sourced finding must carry its concept id, like the profile path")
	assert.Equal(t, "affordable", f.Metadata["replacement"])
	assert.Contains(t, f.Suggestion, "affordable")
}

func TestBrandVocabCheckTermbaseStandaloneConcept(t *testing.T) {
	t.Parallel()
	// A termbase match whose concept carries no ID (a degenerate / store that does
	// not populate it) yields a finding without a concept_id key.
	tb := &fakeTermBase{matches: []termbase.TermMatch{{
		Concept:  termbase.Concept{Source: termbase.TermSourceBrandVocabulary},
		Term:     termbase.Term{Text: "cheap", Status: model.TermForbidden},
		Position: model.TextRange{Start: 10, End: 15},
	}}}

	tool := tools.NewBrandVocabCheckTool(nil, tb)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "This is a cheap product")}
	close(in)

	require.NoError(t, tool.Process(ctx, in, out))
	resultBlock := (<-out).Resource.(*model.Block)
	bvAnn, bvOK := model.AnnoAs[*brand.BrandVoiceAnnotation](resultBlock, "brand-voice")
	require.True(t, bvOK)
	require.Len(t, bvAnn.Findings, 1)
	_, hasConcept := bvAnn.Findings[0].Metadata["concept_id"]
	assert.False(t, hasConcept, "a concept-less termbase match must not carry a concept_id")
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

	_, bvOK := model.AnnoAs[*brand.BrandVoiceAnnotation](resultBlock, "brand-voice")
	assert.False(t, bvOK)
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
	_, bvOK := model.AnnoAs[*brand.BrandVoiceAnnotation](resultBlock, "brand-voice")
	assert.False(t, bvOK)
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

	bvAnn, bvOK := model.AnnoAs[*brand.BrandVoiceAnnotation](resultBlock, "brand-voice")
	require.True(t, bvOK)
	findings := bvAnn.Findings
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

	ann, ok := resultBlock.Anno("brand-voice")
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
	_, bvOK := model.AnnoAs[*brand.BrandVoiceAnnotation](resultBlock, "brand-voice")
	assert.False(t, bvOK)
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

	ann, ok := resultBlock.Anno("brand-voice")
	require.True(t, ok)

	bva := ann.(*brand.BrandVoiceAnnotation)
	assert.Equal(t, "voice-1", bva.ProfileID)
	assert.Less(t, bva.Score, 100)
	assert.Len(t, bva.Findings, 1)
}
