package reviewdeck

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/memory"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// fixBlock is an English source with one placeholder and its Norwegian draft.
func fixBlock(draft ...model.Run) *model.Block {
	b := &model.Block{ID: "b1", Translatable: true, SourceLocale: "en"}
	b.SetSourceRuns([]model.Run{model.TextR("Save the workspace "), model.PhR(model.PlaceholderRun{ID: "1", Type: "var", Data: "{name}", Equiv: "name"})})
	b.SetTargetRuns("nb", draft)
	return b
}

func namePh() model.Run {
	return model.PhR(model.PlaceholderRun{ID: "1", Type: "var", Data: "{name}", Equiv: "name"})
}

func nbRules() []profile.TermRuleSet {
	return []profile.TermRuleSet{{Kind: profile.VocabForbidden, Rules: []profile.TermRule{
		{Term: "arbeidsrommet", Replacement: "arbeidsområdet"},
		{Term: "lagre", Replacement: ""},
	}}}
}

func TestFixCandidates_TermFixKeepsThePlaceholder(t *testing.T) {
	b := fixBlock(model.TextR("Lagre arbeidsrommet "), namePh())
	res, err := FixCandidates(context.Background(), FixCandidatesRequest{
		Block: b, Source: "en", Target: "nb", Reason: FixReasonWrongTerm, TermRules: nbRules(),
	})
	require.NoError(t, err)
	require.Len(t, res.Candidates, 1)
	c := res.Candidates[0]
	assert.Equal(t, FixTerm, c.Kind)
	assert.Equal(t, "Lagre arbeidsområdet {name}", c.Text)
	assert.Equal(t, []FixChange{{From: "arbeidsrommet", To: "arbeidsområdet", Count: 1}}, c.Changes)
	assert.True(t, model.DiffRunCodes(b.SourceRuns(), c.Runs).Balanced(), "the placeholder stays a run")
	assert.Equal(t, "Lagre arbeidsrommet {name}", model.RenderRunsWithData(b.TargetRuns("nb")), "the block is left as it was")

	var removed, added []string
	for _, s := range c.Diff.Prior {
		if s.Op == "removed" {
			removed = append(removed, s.Text)
		}
	}
	for _, s := range c.Diff.Current {
		if s.Op == "added" {
			added = append(added, s.Text)
		}
	}
	assert.Equal(t, []string{"arbeidsrommet"}, removed)
	assert.Equal(t, []string{"arbeidsområdet"}, added)
}

func TestFixCandidates_NoTermFixWithoutAViolation(t *testing.T) {
	b := fixBlock(model.TextR("Lagre arbeidsområdet "), namePh())
	res, err := FixCandidates(context.Background(), FixCandidatesRequest{Block: b, Source: "en", Target: "nb", TermRules: nbRules()})
	require.NoError(t, err)
	assert.Empty(t, res.Candidates)
}

func TestFixCandidates_MemoryAlternatives(t *testing.T) {
	ctx := context.Background()
	b := fixBlock(model.TextR("Lagre arbeidsrommet "), namePh())
	tm := memory.NewInMemoryStore()
	add := func(id string, target ...model.Run) {
		require.NoError(t, tm.Add(ctx, memory.Entry{ID: id, Variants: map[model.LocaleID][]model.Run{
			"en": b.SourceRuns(),
			"nb": target,
		}}))
	}
	add("good", model.TextR("Lagre arbeidsområdet "), namePh())
	add("same", model.TextR("Lagre arbeidsrommet "), namePh())
	add("lost", model.TextR("Lagre arbeidsområdet"))

	res, err := FixCandidates(ctx, FixCandidatesRequest{Block: b, Source: "en", Target: "nb", Memory: tm, Reason: FixReasonMeaning})
	require.NoError(t, err)
	var texts []string
	for _, c := range res.Candidates {
		require.Equal(t, FixMemory, c.Kind)
		require.NotNil(t, c.Score)
		texts = append(texts, c.Text)
	}
	assert.Contains(t, texts, "Lagre arbeidsområdet {name}")
	assert.NotContains(t, texts, "Lagre arbeidsrommet {name}", "a match identical to the draft is no fix")
	for _, c := range res.Candidates {
		assert.True(t, model.DiffRunCodes(b.SourceRuns(), c.Runs).Balanced(), "no candidate loses the placeholder: %s", c.Text)
	}
}

func TestFixCandidates_DropsACandidateThatBreaksAPlaceholder(t *testing.T) {
	ctx := context.Background()
	b := fixBlock(model.TextR("Lagre arbeidsrommet "), namePh())
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(ctx, memory.Entry{ID: "lost", Variants: map[model.LocaleID][]model.Run{
		"en": b.SourceRuns(),
		"nb": {model.TextR("Lagre arbeidsområdet")},
	}}))
	res, err := FixCandidates(ctx, FixCandidatesRequest{Block: b, Source: "en", Target: "nb", Memory: tm})
	require.NoError(t, err)
	assert.Empty(t, res.Candidates)
	require.NotEmpty(t, res.Dropped)
	assert.Equal(t, FixMemory, res.Dropped[0].Kind)
	assert.Contains(t, res.Dropped[0].Why, "placeholder")
}

// The AI rewrite runs a one-block translate through the demo provider, a
// deterministic stub, with the instruction FixInstruction builds.
func TestFixCandidates_AIRewriteOnRequest(t *testing.T) {
	b := fixBlock(model.TextR("Lagre arbeidsrommet "), namePh())
	var asked string
	chain := func(_ context.Context, instruction string) ([]tool.Tool, func(), error) {
		asked = instruction
		tl := aitools.NewAITranslateTool(aiprovider.NewDemoProvider(aiprovider.Config{}), aitools.AITranslateConfig{
			SourceLocale: "en", TargetLocale: "nb", Instruction: instruction, BatchSize: 1,
		})
		return []tool.Tool{tl}, func() {}, nil
	}
	res, err := FixCandidates(context.Background(), FixCandidatesRequest{
		Block: b, Source: "en", Target: "nb", Reason: FixReasonTone, AI: chain,
	})
	require.NoError(t, err)
	assert.Contains(t, asked, "Lagre arbeidsrommet {name}", "the instruction carries the draft")
	assert.Contains(t, asked, "tone")
	require.Len(t, res.Candidates, 1, "%+v %+v", res.Dropped, res.Notes)
	c := res.Candidates[0]
	assert.Equal(t, FixAI, c.Kind)
	assert.Equal(t, []FixReason{FixReasonTone}, c.Reasons)
	assert.True(t, model.DiffRunCodes(b.SourceRuns(), c.Runs).Balanced())
	assert.Equal(t, "Lagre arbeidsrommet {name}", model.RenderRunsWithData(b.TargetRuns("nb")), "the block is left as it was")
}

func TestFixCandidates_AIUnavailableIsANote(t *testing.T) {
	b := fixBlock(model.TextR("Lagre arbeidsrommet "), namePh())
	res, err := FixCandidates(context.Background(), FixCandidatesRequest{
		Block: b, Source: "en", Target: "nb",
		AI: func(context.Context, string) ([]tool.Tool, func(), error) {
			return nil, nil, errors.New("no AI provider is configured")
		},
	})
	require.NoError(t, err)
	assert.Empty(t, res.Candidates)
	assert.Equal(t, []string{"no AI rewrite: no AI provider is configured"}, res.Notes)
}

func TestFixCandidates_ReasonOrdersTheKinds(t *testing.T) {
	assert.Equal(t, []FixKind{FixTerm, FixMemory, FixAI}, fixOrder(FixReasonWrongTerm))
	assert.Equal(t, []FixKind{FixMemory, FixAI, FixTerm}, fixOrder(FixReasonMeaning))
	assert.Equal(t, []FixKind{FixAI, FixMemory, FixTerm}, fixOrder(FixReasonGrammar))
	_, err := ParseFixReason("rude")
	assert.Error(t, err)
}

func TestClassifyDeckCard(t *testing.T) {
	score := func(n int) *int { return &n }
	tests := []struct {
		name  string
		facts DeckFacts
		want  DeckGroup
	}{
		{"clean and high", DeckFacts{Score: score(92), SourceChars: 40}, DeckFast},
		{"at the bar", DeckFacts{Score: score(80)}, DeckFast},
		{"low score", DeckFacts{Score: score(60)}, DeckCloser},
		{"no score", DeckFacts{}, DeckCloser},
		{"a finding that does not fail", DeckFacts{Findings: 1, Score: score(95)}, DeckCloser},
		{"a failing finding", DeckFacts{Findings: 1, FailingFindings: 1, Score: score(95)}, DeckDesk},
		{"stale", DeckFacts{Stale: true, Score: score(95)}, DeckDesk},
		{"redraft", DeckFacts{RejectedRedraft: true, Score: score(95)}, DeckDesk},
		{"rejected as it stands", DeckFacts{Rejected: true, Score: score(95)}, DeckAside},
		{"rejected with a failing finding", DeckFacts{Rejected: true, FailingFindings: 1}, DeckAside},
		{"long source", DeckFacts{SourceChars: 281, Score: score(95)}, DeckDesk},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := ClassifyDeckCard(tc.facts, DeckPolicy{})
			assert.Equal(t, tc.want, got)
			assert.NotEmpty(t, reason)
		})
	}
	got, _ := ClassifyDeckCard(DeckFacts{SourceChars: 100, Score: score(95)}, DeckPolicy{LongSourceChars: 50})
	assert.Equal(t, DeckDesk, got, "the policy's limit applies")
}
