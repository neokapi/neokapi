package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/terms"
)

func findingsVia(t tool.Tool, read func(*model.Block) []check.Finding) func(*model.Block) ([]check.Finding, error) {
	return func(b *model.Block) ([]check.Finding, error) {
		in := make(chan *model.Part, 1)
		out := make(chan *model.Part, 1)
		in <- &model.Part{Type: model.PartBlock, Resource: b}
		close(in)
		errc := make(chan error, 1)
		go func() {
			defer close(out)
			errc <- t.Process(context.Background(), in, out)
		}()
		for range out { //nolint:revive // drain
		}
		if err := <-errc; err != nil {
			return nil, err
		}
		return read(b), nil
	}
}

func voiceFindings(b *model.Block) []check.Finding {
	if ann, ok := model.AnnoAs[*coreprofile.VoiceAnnotation](b, "voice"); ok {
		return ann.Findings
	}
	return nil
}

func unifiedFindings(b *model.Block) []check.Finding {
	if ann, ok := model.AnnoAs[*check.FindingsAnnotation](b, check.AnnotationKey); ok {
		return ann.Findings
	}
	return nil
}

func inertChecker(*model.Block) ([]check.Finding, error) { return nil, nil }

func TestVoiceVocabCanaries(t *testing.T) {
	ctx := context.Background()
	forbidden := &coreprofile.VoiceProfile{ID: "p"}
	forbidden.Vocabulary.ForbiddenTerms = []coreprofile.TermRule{{Term: "risk-free", Severity: "critical"}}
	forbidden.Vocabulary.CompetitorTerms = []coreprofile.TermRule{{Term: "Acme"}}
	forbidden.Style.ProhibitedPatterns = []coreprofile.Pattern{{Regex: `(?i)\bsimply\b`}}

	store := terms.NewInMemoryStore(terms.WithMaxConcepts(0))
	require.NoError(t, store.AddConcept(ctx, terms.Concept{ID: "c1", Terms: []terms.Term{
		{Text: "whitelist", Locale: "en", Status: model.TermForbidden},
		{Text: "allowlist", Locale: "en", Status: model.TermPreferred},
	}}))

	tests := []struct {
		name    string
		profile *coreprofile.VoiceProfile
		store   terms.Terminology
		probes  int
	}{
		{"profile vocabulary and patterns", forbidden, nil, 3},
		{"terms store only", nil, store, 1},
		{"both", forbidden, store, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewVoiceVocabCheckTool(tt.profile, tt.store).InSourceLocale("en")
			canaries, uncheckable, err := tool.Canaries(ctx)
			require.NoError(t, err)
			require.Empty(t, uncheckable)
			require.Len(t, canaries, tt.probes)

			outcome, err := check.Probe(canaries, uncheckable, findingsVia(tool, voiceFindings))
			require.NoError(t, err)
			assert.Equal(t, check.CanaryCaught, outcome.Status)

			outcome, err = check.Probe(canaries, uncheckable, inertChecker)
			require.NoError(t, err)
			assert.Equal(t, check.CanaryMissed, outcome.Status)
		})
	}
}

func TestVoiceVocabCanaries_NothingToCatch(t *testing.T) {
	ctx := context.Background()
	toneOnly := &coreprofile.VoiceProfile{ID: "tone"}
	toneOnly.Tone.Personality = []string{"plain"}
	onlyPreferred := terms.NewInMemoryStore(terms.WithMaxConcepts(0))
	require.NoError(t, onlyPreferred.AddConcept(ctx, terms.Concept{ID: "c", Terms: []terms.Term{{Text: "allowlist", Locale: "en", Status: model.TermPreferred}}}))
	otherLanguage := terms.NewInMemoryStore(terms.WithMaxConcepts(0))
	require.NoError(t, otherLanguage.AddConcept(ctx, terms.Concept{ID: "c", Terms: []terms.Term{{Text: "svarteliste", Locale: "nb", Status: model.TermForbidden}}}))

	for name, tool := range map[string]*VoiceVocabCheckTool{
		"a tone-only profile":           NewVoiceVocabCheckTool(toneOnly, nil),
		"a store of preferred terms":    NewVoiceVocabCheckTool(nil, onlyPreferred).InSourceLocale("en"),
		"a store forbidding in another": NewVoiceVocabCheckTool(nil, otherLanguage).InSourceLocale("en"),
	} {
		t.Run(name, func(t *testing.T) {
			canaries, uncheckable, err := tool.Canaries(ctx)
			require.NoError(t, err)
			assert.Empty(t, canaries)
			assert.NotEmpty(t, uncheckable)
		})
	}
}

func TestRequiredPatternCanaries(t *testing.T) {
	p := &coreprofile.VoiceProfile{ID: "p"}
	p.Style.RequiredPatterns = []coreprofile.Pattern{{Regex: `©`}}
	canaries, uncheckable := RequiredPatternCanaries(p)
	require.Empty(t, uncheckable)
	require.Len(t, canaries, 1)
	assert.NotEmpty(t, coreprofile.DocumentFindings(p, canaries[0].Block.SourceText()))

	_, uncheckable = RequiredPatternCanaries(&coreprofile.VoiceProfile{ID: "none"})
	assert.NotEmpty(t, uncheckable)
}

func TestBilingualCanaries(t *testing.T) {
	placeholder := NewPlaceholderCheckTool(NewPlaceholderCheckConfig("de"))
	outcome, err := check.Probe(PlaceholderCanaries("de"), "", findingsVia(placeholder, unifiedFindings))
	require.NoError(t, err)
	assert.Equal(t, check.CanaryCaught, outcome.Status)

	cfg := NewDNTCheckConfig("de")
	cfg.Terms = []string{"kapi"}
	canaries, uncheckable := DNTCanaries(cfg)
	require.Empty(t, uncheckable)
	outcome, err = check.Probe(canaries, "", findingsVia(NewDNTCheckTool(cfg), unifiedFindings))
	require.NoError(t, err)
	assert.Equal(t, check.CanaryCaught, outcome.Status)

	outcome, err = check.Probe(canaries, "", inertChecker)
	require.NoError(t, err)
	assert.Equal(t, check.CanaryMissed, outcome.Status)

	_, uncheckable = DNTCanaries(NewDNTCheckConfig("de"))
	assert.NotEmpty(t, uncheckable)
}
