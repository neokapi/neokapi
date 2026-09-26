package tools_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProfileResolver implements coreprofile.ProfileResolver for testing.
type mockProfileResolver struct {
	profile *coreprofile.VoiceProfile
}

func (m *mockProfileResolver) ResolveProfile(_ context.Context, _ coreprofile.ResolveContext) (*coreprofile.VoiceProfile, error) {
	return m.profile, nil
}

// fakeTerminology serves a fixed set of LookupAll matches and is otherwise an inert
// terms.Terminology. It lets the voice-vocab tool's terms branch run with a
// concept-bearing match under full test control, without a SQLite store.
type fakeTerminology struct {
	matches []terms.TermMatch
}

func (f *fakeTerminology) LookupAll(context.Context, string, terms.LookupOptions) ([]terms.TermMatch, error) {
	return f.matches, nil
}
func (f *fakeTerminology) Lookup(context.Context, string, terms.LookupOptions) ([]terms.TermMatch, error) {
	return f.matches, nil
}
func (f *fakeTerminology) AddConcept(context.Context, terms.Concept) error { return nil }
func (f *fakeTerminology) GetConcept(context.Context, string) (terms.Concept, bool, error) {
	return terms.Concept{}, false, nil
}
func (f *fakeTerminology) DeleteConcept(context.Context, string) error { return nil }
func (f *fakeTerminology) Search(context.Context, string, model.LocaleID, model.LocaleID, int, int) ([]terms.Concept, int, error) {
	return nil, 0, nil
}
func (f *fakeTerminology) Count(context.Context) (int, error)                       { return 0, nil }
func (f *fakeTerminology) Concepts(context.Context) ([]terms.Concept, error)        { return nil, nil }
func (f *fakeTerminology) AddRelation(context.Context, terms.ConceptRelation) error { return nil }
func (f *fakeTerminology) DeleteRelation(context.Context, string) error             { return nil }
func (f *fakeTerminology) RelationsOf(context.Context, string, *graph.Scope) ([]terms.ConceptRelation, error) {
	return nil, nil
}
func (f *fakeTerminology) ListRelations(context.Context, *graph.Scope) ([]terms.ConceptRelation, error) {
	return nil, nil
}
func (f *fakeTerminology) Close() error { return nil }

func TestVoiceVocabCheckForbiddenTerms(t *testing.T) {
	t.Parallel()
	profile := (&coreprofile.VoiceProfile{
		ID: "test-profile",
	}).Carry("test", []coreprofile.TermRule{
		{Term: "cheap", Replacement: "affordable", Note: "avoid negative connotation"},
	})

	tool := tools.NewVoiceVocabCheckTool(profile, nil)

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

	bvAnn, bvOK := model.AnnoAs[*coreprofile.VoiceAnnotation](resultBlock, "voice")
	require.True(t, bvOK)
	findings := bvAnn.Findings
	require.Len(t, findings, 1)

	assert.Equal(t, string(coreprofile.DimensionVocabulary), findings[0].Category)
	assert.True(t, findings[0].Fails)
	assert.Contains(t, findings[0].Message, "cheap")
	assert.Contains(t, findings[0].Suggestion, "affordable")
	assert.Equal(t, 0, findings[0].Position.Start.Run)
	assert.Equal(t, 10, findings[0].Position.Start.Offset)
	assert.Equal(t, 15, findings[0].Position.End.Offset)
}

func TestVoiceVocabCheckCompetitorTerms(t *testing.T) {
	t.Parallel()
	profile := (&coreprofile.VoiceProfile{}).Carry("test", []coreprofile.TermRule{{Term: "Acme Corp", Replacement: "our platform", Competitor: true}})

	tool := tools.NewVoiceVocabCheckTool(profile, nil)

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

	bvAnn, bvOK := model.AnnoAs[*coreprofile.VoiceAnnotation](resultBlock, "voice")
	require.True(t, bvOK)
	findings := bvAnn.Findings
	require.Len(t, findings, 1)

	assert.True(t, findings[0].Fails)
	assert.Contains(t, findings[0].Message, "Competitor term")
	assert.Contains(t, findings[0].Suggestion, "our platform")
}

func TestVoiceVocabCheckPreferredTermSuggestion(t *testing.T) {
	t.Parallel()
	profile := (&coreprofile.VoiceProfile{}).Carry("test", []coreprofile.TermRule{{Term: "users", Replacement: "customers"}})

	tool := tools.NewVoiceVocabCheckTool(profile, nil)

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

	bvAnn, bvOK := model.AnnoAs[*coreprofile.VoiceAnnotation](resultBlock, "voice")
	require.True(t, bvOK)
	findings := bvAnn.Findings
	require.Len(t, findings, 1)

	assert.Contains(t, findings[0].Suggestion, "customers")
}

func TestVoiceVocabCheckEmitsConceptIDMetadata(t *testing.T) {
	t.Parallel()
	profile := (&coreprofile.VoiceProfile{}).Carry("test", []coreprofile.TermRule{{Term: "cheap", Replacement: "affordable", ConceptID: "concept-affordable"}})

	tool := tools.NewVoiceVocabCheckTool(profile, nil)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "This is a cheap product")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	require.NoError(t, tool.Process(ctx, in, out))

	resultBlock := (<-out).Resource.(*model.Block)
	bvAnn, bvOK := model.AnnoAs[*coreprofile.VoiceAnnotation](resultBlock, "voice")
	require.True(t, bvOK)
	require.Len(t, bvAnn.Findings, 1)

	// The concept-backed rule links its finding to the concept story, alongside
	// the existing structured replacement.
	assert.Equal(t, "concept-affordable", bvAnn.Findings[0].Metadata["concept_id"])
	assert.Equal(t, "affordable", bvAnn.Findings[0].Metadata["replacement"])
}

func TestVoiceVocabCheckStandaloneOmitsConceptID(t *testing.T) {
	t.Parallel()
	// A standalone profile (no concept on the rule) emits findings without a
	// concept_id metadata key.
	profile := (&coreprofile.VoiceProfile{}).Carry("test", []coreprofile.TermRule{{Term: "cheap", Replacement: "affordable"}})

	tool := tools.NewVoiceVocabCheckTool(profile, nil)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "This is a cheap product")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	require.NoError(t, tool.Process(ctx, in, out))

	resultBlock := (<-out).Resource.(*model.Block)
	bvAnn, bvOK := model.AnnoAs[*coreprofile.VoiceAnnotation](resultBlock, "voice")
	require.True(t, bvOK)
	require.Len(t, bvAnn.Findings, 1)

	_, hasConcept := bvAnn.Findings[0].Metadata["concept_id"]
	assert.False(t, hasConcept, "standalone finding must not carry a concept_id")
	// The structured replacement is still present.
	assert.Equal(t, "affordable", bvAnn.Findings[0].Metadata["replacement"])
}

func TestVoiceVocabCheckTermsConceptID(t *testing.T) {
	t.Parallel()
	// A forbidden voice-vocabulary term found via the terms store carries its
	// knowledge-graph concept; when the concept holds a preferred term in the
	// source locale, that surfaces as the structured replacement — symmetric with
	// the profile path.
	tb := &fakeTerminology{matches: []terms.TermMatch{{
		Concept: terms.Concept{
			ID:     "concept-cheap",
			Source: terms.TermSourceBrandVocabulary,
			Terms: []terms.Term{
				{Text: "cheap", Locale: "en", Status: model.TermForbidden},
				{Text: "affordable", Locale: "en", Status: model.TermPreferred},
			},
		},
		Term:     terms.Term{Text: "cheap", Status: model.TermForbidden},
		Position: model.TextRange{Start: 10, End: 15},
	}}}

	tool := tools.NewVoiceVocabCheckTool(nil, tb).InSourceLocale("en")

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "This is a cheap product")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	require.NoError(t, tool.Process(ctx, in, out))

	resultBlock := (<-out).Resource.(*model.Block)
	bvAnn, bvOK := model.AnnoAs[*coreprofile.VoiceAnnotation](resultBlock, "voice")
	require.True(t, bvOK)
	require.Len(t, bvAnn.Findings, 1)

	f := bvAnn.Findings[0]
	assert.Contains(t, f.Message, "cheap")
	assert.Equal(t, "concept-cheap", f.Metadata["concept_id"],
		"a terms store-sourced finding must carry its concept id, like the profile path")
	assert.Equal(t, "affordable", f.Metadata["replacement"])
	assert.Contains(t, f.Suggestion, "affordable")
}

func TestVoiceVocabCheckTermsStandaloneConcept(t *testing.T) {
	t.Parallel()
	// A terms store match whose concept carries no ID (a degenerate / store that does
	// not populate it) yields a finding without a concept_id key.
	tb := &fakeTerminology{matches: []terms.TermMatch{{
		Concept:  terms.Concept{Source: terms.TermSourceBrandVocabulary},
		Term:     terms.Term{Text: "cheap", Status: model.TermForbidden},
		Position: model.TextRange{Start: 10, End: 15},
	}}}

	tool := tools.NewVoiceVocabCheckTool(nil, tb).InSourceLocale("en")

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "This is a cheap product")}
	close(in)

	require.NoError(t, tool.Process(ctx, in, out))
	resultBlock := (<-out).Resource.(*model.Block)
	bvAnn, bvOK := model.AnnoAs[*coreprofile.VoiceAnnotation](resultBlock, "voice")
	require.True(t, bvOK)
	require.Len(t, bvAnn.Findings, 1)
	_, hasConcept := bvAnn.Findings[0].Metadata["concept_id"]
	assert.False(t, hasConcept, "a concept-less terms match must not carry a concept_id")
}

func TestVoiceVocabCheckNoViolations(t *testing.T) {
	t.Parallel()
	profile := (&coreprofile.VoiceProfile{}).Carry("test", []coreprofile.TermRule{{Term: "cheap"}, {Term: "Acme Corp", Competitor: true}})

	tool := tools.NewVoiceVocabCheckTool(profile, nil)

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

	_, bvOK := model.AnnoAs[*coreprofile.VoiceAnnotation](resultBlock, "voice")
	assert.False(t, bvOK)
}

func TestVoiceVocabCheckSkipsEmptyText(t *testing.T) {
	t.Parallel()
	profile := (&coreprofile.VoiceProfile{}).Carry("test", []coreprofile.TermRule{{Term: "cheap"}})

	tool := tools.NewVoiceVocabCheckTool(profile, nil)

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
	_, bvOK := model.AnnoAs[*coreprofile.VoiceAnnotation](resultBlock, "voice")
	assert.False(t, bvOK)
}

func TestVoiceVocabCheckCaseInsensitive(t *testing.T) {
	t.Parallel()
	profile := (&coreprofile.VoiceProfile{}).Carry("test", []coreprofile.TermRule{{Term: "cheap"}})

	tool := tools.NewVoiceVocabCheckTool(profile, nil)

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

	bvAnn, bvOK := model.AnnoAs[*coreprofile.VoiceAnnotation](resultBlock, "voice")
	require.True(t, bvOK)
	findings := bvAnn.Findings
	require.Len(t, findings, 1)
	assert.Equal(t, "CHEAP", findings[0].OriginalText)
}

func TestVoiceVocabCheckWithResolver(t *testing.T) {
	t.Parallel()
	profile := (&coreprofile.VoiceProfile{
		ID: "resolved-vocab",
	}).Carry("test", []coreprofile.TermRule{{Term: "cheap", Replacement: "affordable"}})

	resolver := &mockProfileResolver{profile: profile}
	rc := coreprofile.ResolveContext{ExplicitProfileID: "resolved-vocab"}

	tool := tools.NewVoiceVocabCheckToolWithResolver(resolver, rc, nil)

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

	ann, ok := resultBlock.Anno("voice")
	require.True(t, ok)
	bva := ann.(*coreprofile.VoiceAnnotation)
	assert.Equal(t, "resolved-vocab", bva.ProfileID)
	assert.Len(t, bva.Findings, 1)
}

func TestVoiceVocabCheckWithResolverNilProfile(t *testing.T) {
	t.Parallel()
	resolver := &mockProfileResolver{profile: nil}
	rc := coreprofile.ResolveContext{}

	tool := tools.NewVoiceVocabCheckToolWithResolver(resolver, rc, nil)

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
	_, bvOK := model.AnnoAs[*coreprofile.VoiceAnnotation](resultBlock, "voice")
	assert.False(t, bvOK)
}

func TestVoiceVocabCheckAddsAnnotation(t *testing.T) {
	t.Parallel()
	profile := (&coreprofile.VoiceProfile{
		ID: "voice-1",
	}).Carry("test", []coreprofile.TermRule{{Term: "stuff"}})

	tool := tools.NewVoiceVocabCheckTool(profile, nil)

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

	ann, ok := resultBlock.Anno("voice")
	require.True(t, ok)

	bva := ann.(*coreprofile.VoiceAnnotation)
	assert.Equal(t, "voice-1", bva.ProfileID)
	assert.Less(t, bva.Score, 100)
	assert.Len(t, bva.Findings, 1)
}

// TestVoiceVocabCheckTermsStatuses: the vocabulary the project decided is
// enforced from whichever source recorded it, and a retired term is a finding
// carrying the preferred word as its fix. A voice-vocabulary-only filter
// enforced a source `kapi apply` never writes to, so a term decision was visible
// to retrieval and invisible to every gate.
func TestVoiceVocabCheckTermsStatuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		status      model.TermStatus
		source      terms.TermSource
		competitor  bool
		wantFinding bool
		wantFails   bool
		wantMessage string
	}{
		{
			name: "forbidden voice term", status: model.TermForbidden,
			source:      terms.TermSourceBrandVocabulary,
			wantFinding: true, wantFails: true, wantMessage: "Forbidden term",
		},
		{
			name: "retired terminology term", status: model.TermDeprecated,
			source:      terms.TermSourceTerminology,
			wantFinding: true, wantFails: false, wantMessage: "Retired term",
		},
		{
			name: "competitor term", status: model.TermAdmitted,
			source:      terms.TermSourceTerminology,
			competitor:  true,
			wantFinding: true, wantFails: true, wantMessage: "Competitor term",
		},
		{
			name: "admitted term is not a finding", status: model.TermAdmitted,
			source: terms.TermSourceTerminology,
		},
		{
			name: "preferred term is not a finding", status: model.TermPreferred,
			source: terms.TermSourceTerminology,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tb := &fakeTerminology{matches: []terms.TermMatch{{
				Concept: terms.Concept{
					ID:     "c-mooring",
					Source: tt.source,
					Terms: []terms.Term{
						{Text: "mooring", Locale: "en", Status: tt.status},
						{Text: "berth", Locale: "en", Status: model.TermPreferred},
					},
				},
				Term:     terms.Term{Text: "mooring", Locale: "en", Status: tt.status, CompetitorTerm: tt.competitor},
				Position: model.TextRange{Start: 6, End: 13},
			}}}

			tool := tools.NewVoiceVocabCheckTool(nil, tb).InSourceLocale("en")
			in := make(chan *model.Part, 1)
			out := make(chan *model.Part, 1)
			in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Every mooring is allocated")}
			close(in)
			require.NoError(t, tool.Process(t.Context(), in, out))

			block := (<-out).Resource.(*model.Block)
			ann, ok := model.AnnoAs[*coreprofile.VoiceAnnotation](block, "voice")
			if !tt.wantFinding {
				assert.False(t, ok, "a term in good standing is not a finding")
				return
			}
			require.True(t, ok)
			require.Len(t, ann.Findings, 1)
			assert.Equal(t, tt.wantFails, ann.Findings[0].Fails)
			assert.Contains(t, ann.Findings[0].Message, tt.wantMessage)
			assert.Equal(t, "berth", ann.Findings[0].Metadata["replacement"],
				"the preferred term in the same language is the fix")
		})
	}
}

// TestVoiceVocabCheckLooksUpTheSourceLanguage: the lookup matches a locale
// exactly, so the tool asks in the language its content is written in — and in
// the bare language beneath a regional one, since a vocabulary recorded as `en`
// governs en-GB text just as much.
func TestVoiceVocabCheckLooksUpTheSourceLanguage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		locale model.LocaleID
		want   []model.LocaleID
	}{
		{name: "regional locale asks in both", locale: "en-GB", want: []model.LocaleID{"en-GB", "en"}},
		{name: "bare language asks once", locale: "nb", want: []model.LocaleID{"nb"}},
		{name: "posix spelling asks in canonical form", locale: "pt_BR", want: []model.LocaleID{"pt-BR", "pt"}},
		{name: "no locale asks nothing", locale: "", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tb := &recordingTerminology{}
			tool := tools.NewVoiceVocabCheckTool(nil, tb).InSourceLocale(tt.locale)
			in := make(chan *model.Part, 1)
			out := make(chan *model.Part, 1)
			in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Every mooring is allocated")}
			close(in)
			require.NoError(t, tool.Process(t.Context(), in, out))
			<-out
			assert.Equal(t, tt.want, tb.asked)
		})
	}
}

// recordingTerminology records the locales a lookup asked in, and finds nothing.
type recordingTerminology struct {
	fakeTerminology
	asked []model.LocaleID
}

func (r *recordingTerminology) LookupAll(_ context.Context, _ string, opts terms.LookupOptions) ([]terms.TermMatch, error) {
	r.asked = append(r.asked, opts.SourceLocale)
	return nil, nil
}

// TestVoiceVocabCheckNamesWhereTheDecisionLives pins where a word rule is
// held. Every word rule is a term and reads the same way; a rule a voice file
// or starter pack carries names that source in the finding's `from`
// metadata, so a writer knows where to argue with the decision, and a rule in
// the terms store names none.
func TestVoiceVocabCheckNamesWhereTheDecisionLives(t *testing.T) {
	t.Parallel()

	findingFor := func(t *testing.T, tool *tools.VoiceVocabCheckTool, text string) coreprofile.VoiceFinding {
		t.Helper()
		in := make(chan *model.Part, 1)
		out := make(chan *model.Part, 1)
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", text)}
		close(in)
		require.NoError(t, tool.Process(t.Context(), in, out))
		ann, ok := model.AnnoAs[*coreprofile.VoiceAnnotation]((<-out).Resource.(*model.Block), "voice")
		require.True(t, ok)
		require.Len(t, ann.Findings, 1)
		return ann.Findings[0]
	}

	t.Run("a carried rule names its source", func(t *testing.T) {
		t.Parallel()
		p := (&coreprofile.VoiceProfile{
			ID: "p1",
		}).Carry("pack technical-docs", []coreprofile.TermRule{{Term: "cheap", Replacement: "affordable"}})
		f := findingFor(t, tools.NewVoiceVocabCheckTool(p, nil), "This is a cheap product")
		assert.Equal(t, `Forbidden term "cheap" found`, f.Message)
		assert.Equal(t, "pack technical-docs", f.Metadata["from"])
		assert.True(t, f.Fails)
	})

	t.Run("a terms store concept names no other source", func(t *testing.T) {
		t.Parallel()
		tb := &fakeTerminology{matches: []terms.TermMatch{{
			Concept:  terms.Concept{ID: "c1", Source: terms.TermSourceBrandVocabulary},
			Term:     terms.Term{Text: "cheap", Status: model.TermForbidden},
			Position: model.TextRange{Start: 10, End: 15},
		}}}
		f := findingFor(t, tools.NewVoiceVocabCheckTool(nil, tb).InSourceLocale("en"), "This is a cheap product")
		assert.Equal(t, `Forbidden term "cheap" found`, f.Message)
		assert.NotContains(t, f.Metadata, "from")
		assert.True(t, f.Fails)
	})

	t.Run("a retired term reads as the softer complaint and reports", func(t *testing.T) {
		t.Parallel()
		tb := &fakeTerminology{matches: []terms.TermMatch{{
			Concept:  terms.Concept{ID: "c2", Source: terms.TermSourceBrandVocabulary},
			Term:     terms.Term{Text: "cheap", Status: model.TermDeprecated},
			Position: model.TextRange{Start: 10, End: 15},
		}}}
		f := findingFor(t, tools.NewVoiceVocabCheckTool(nil, tb).InSourceLocale("en"), "This is a cheap product")
		assert.Equal(t, `Retired term "cheap" found`, f.Message)
		assert.False(t, f.Fails, "a retired term reports without failing")
	})

	t.Run("an advisory concept reports without failing", func(t *testing.T) {
		t.Parallel()
		tb := &fakeTerminology{matches: []terms.TermMatch{{
			Concept:  terms.Concept{ID: "c3", Source: terms.TermSourceBrandVocabulary, Advisory: true},
			Term:     terms.Term{Text: "cheap", Status: model.TermForbidden},
			Position: model.TextRange{Start: 10, End: 15},
		}}}
		f := findingFor(t, tools.NewVoiceVocabCheckTool(nil, tb).InSourceLocale("en"), "This is a cheap product")
		assert.Equal(t, `Forbidden term "cheap" found`, f.Message)
		assert.False(t, f.Fails, "an advisory concept reports without failing")
	})
}

func TestVoiceVocabCheckSharedConstraint(t *testing.T) {
	p := &coreprofile.VoiceProfile{Constraints: []coreprofile.Constraint{{
		ID: "assurance", Version: 1, Source: "facts.md", Statement: "No risk-free promises",
		Kind: coreprofile.ConstraintProhibitedPattern, Regex: "risk-free",
	}}}
	checkTool := tools.NewVoiceVocabCheckTool(p, nil)
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("p", "This is risk-free.")}
	close(in)
	require.NoError(t, checkTool.Process(t.Context(), in, out))
	ann, ok := model.AnnoAs[*coreprofile.VoiceAnnotation]((<-out).Resource.(*model.Block), "voice")
	require.True(t, ok)
	require.Len(t, ann.Findings, 1)
	assert.Equal(t, "assurance", ann.Findings[0].Metadata["constraint_id"])
	assert.True(t, ann.Findings[0].Fails)
}
