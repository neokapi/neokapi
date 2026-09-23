package contextop_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_CandidatesAnswerInTheirOwnProject(t *testing.T) {
	here := contextop.Record{
		ID: "1", Seq: 1, Project: "prj_docs", Kind: contextop.KindObserve,
		Subject: termRule("utilise", "use", ""), Status: contextop.StatusSuggested,
		Scope: contextop.Scope{Level: contextop.LevelProject},
	}
	elsewhere := contextop.Record{
		ID: "2", Seq: 2, Project: "prj_web", Kind: contextop.KindObserve,
		Subject: termRule("leverage", "use", ""), Status: contextop.StatusSuggested,
		Scope: contextop.Scope{Level: contextop.LevelProject},
	}
	widened := contextop.Record{
		ID: "3", Seq: 3, Project: "prj_web", Kind: contextop.KindObserve,
		Subject: termRule("synergy", "fit", ""), Status: contextop.StatusSuggested,
		Scope: contextop.Scope{Level: contextop.LevelWorkspace},
	}

	got := contextop.Resolve([]contextop.Record{here, elsewhere, widened}, nil,
		contextop.ResolveRequest{Project: "prj_docs"})
	assert.Equal(t, []string{"synergy", "utilise"}, termsOf(got.Advisory),
		"this project's candidates and the widened ones, and nobody else's")
	assert.Empty(t, got.Binding)
}

func TestResolve_OnlyCandidatesAdvise(t *testing.T) {
	tests := []struct {
		status contextop.Status
		advise bool
	}{
		{contextop.StatusSuggested, true},
		{contextop.StatusEstablished, false},
		{contextop.StatusDropped, false},
		{contextop.StatusReverted, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			r := contextop.Record{
				ID: "1", Seq: 1, Project: "prj_docs", Kind: contextop.KindObserve,
				Subject: termRule("utilise", "use", ""), Status: tt.status,
			}
			got := contextop.Resolve([]contextop.Record{r}, nil, contextop.ResolveRequest{Project: "prj_docs"})
			assert.Equal(t, tt.advise, len(got.Advisory) == 1,
				"a confirmed rule lives in the project's own store, and a discarded or reverted one answers nowhere")
		})
	}
}

func TestResolve_ScopeCoordinatesNarrowWhereARuleAnswers(t *testing.T) {
	scoped := contextop.Record{
		ID: "1", Seq: 1, Project: "prj_docs", Kind: contextop.KindObserve,
		Subject: termRule("utilise", "use", ""), Status: contextop.StatusSuggested,
		Scope: contextop.Scope{
			Level:       contextop.LevelProject,
			Coordinates: map[string]string{"brand": "northsea", "mode": "reference"},
		},
	}
	tests := []struct {
		name  string
		point map[string]string
		want  int
	}{
		{"the same point", map[string]string{"brand": "northsea", "mode": "reference"}, 1},
		{"a finer point under it", map[string]string{"brand": "northsea", "mode": "reference", "channel": "docs"}, 1},
		{"another mode", map[string]string{"brand": "northsea", "mode": "tutorial"}, 0},
		{"another brand", map[string]string{"brand": "acme", "mode": "reference"}, 0},
		{"a point that names neither axis", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contextop.Resolve([]contextop.Record{scoped}, nil,
				contextop.ResolveRequest{Project: "prj_docs", Coordinates: tt.point})
			assert.Len(t, got.Advisory, tt.want)
		})
	}
}

func TestResolve_TheProjectsOwnTermsHideAWidenedRule(t *testing.T) {
	widened := []contextop.WidenedRule{
		{Operation: "1", Subject: termRule("utilise", "use", "major")},
		{Operation: "2", Subject: termRule("leverage", "use", "major")},
	}

	all := contextop.Resolve(nil, widened, contextop.ResolveRequest{Project: "prj_docs"})
	assert.Equal(t, []string{"leverage", "utilise"}, termsOf(all.Binding))

	narrowed := contextop.Resolve(nil, widened, contextop.ResolveRequest{
		Project: "prj_docs", Held: []string{"Utilise"},
	})
	assert.Equal(t, []string{"leverage"}, termsOf(narrowed.Binding),
		"the project has its own decision about that word, and the most specific answer wins")
}

func TestResolve_TheLatestStatementAboutATermAnswers(t *testing.T) {
	older := contextop.Record{
		ID: "1", Seq: 1, Project: "prj_docs", Kind: contextop.KindObserve,
		Subject: termRule("utilise", "use", ""), Status: contextop.StatusSuggested,
	}
	newer := contextop.Record{
		ID: "2", Seq: 2, Project: "prj_docs", Kind: contextop.KindObserve,
		Subject: termRule("utilise", "employ", ""), Status: contextop.StatusSuggested,
	}
	got := contextop.Resolve([]contextop.Record{older, newer}, nil, contextop.ResolveRequest{Project: "prj_docs"})
	require.Len(t, got.Advisory, 1, "one rule per term")
	assert.Equal(t, "employ", got.Advisory[0].Replacement)
}

func TestResolution_RuleSetsMarkCandidatesAdvisory(t *testing.T) {
	resolution := contextop.Resolution{
		Binding:  []profile.TermRule{{Term: "leverage", Replacement: "use", Severity: "critical"}},
		Advisory: []profile.TermRule{{Term: "utilise", Replacement: "use", Severity: "critical"}},
	}
	sets := resolution.RuleSets()
	require.Len(t, sets, 2)
	assert.False(t, sets[0].Advisory, "a widened rule binds")
	assert.True(t, sets[1].Advisory, "a candidate advises")

	hits := profile.MatchTermRules(sets, "we leverage and utilise it")
	require.Len(t, hits, 2)
	byTerm := map[string]profile.VocabHit{}
	for _, h := range hits {
		byTerm[h.Term] = h
	}
	assert.Equal(t, profile.SeverityCritical, byTerm["leverage"].Severity, "a binding rule keeps its severity")
	assert.Equal(t, profile.SeverityNeutral, byTerm["utilise"].Severity,
		"a candidate is held at neutral, which carries no penalty and trips no gate")
	assert.True(t, byTerm["utilise"].Advisory)

	findings := profile.HitsToFindings(hits, "we leverage and utilise it", nil)
	require.Len(t, findings, 2)
	for _, f := range findings {
		if f.Advisory {
			assert.Contains(t, f.Message, "Proposed rule")
			assert.Equal(t, profile.SeverityNeutral, f.Severity)
		}
	}

	assert.True(t, contextop.Resolution{}.Empty())
	assert.False(t, resolution.Empty())
}

func TestWidenAndNarrow(t *testing.T) {
	ctx := t.Context()
	ws := openWorkspace(t)

	r := contextop.Record{
		ID: "7", Seq: 7, Project: "prj_docs", Kind: contextop.KindObserve,
		Subject: termRule("utilise", "use", "major"), Status: contextop.StatusEstablished,
		Scope: contextop.Scope{Level: contextop.LevelWorkspace},
	}
	require.NoError(t, contextop.Widen(ctx, ws, r))

	held, err := contextop.WidenedRules(ctx, ws)
	require.NoError(t, err)
	require.Len(t, held, 1)
	assert.Equal(t, "7", held[0].Operation)
	assert.Equal(t, "prj_docs", string(held[0].Project), "provenance survives widening")
	rule, ok := held[0].Subject.Rule()
	require.True(t, ok)
	assert.Equal(t, "utilise", rule.Term)

	// Another project's operation 7 is a different rule.
	other := r
	other.Project = "prj_web"
	other.Subject = termRule("leverage", "use", "major")
	require.NoError(t, contextop.Widen(ctx, ws, other))
	held, err = contextop.WidenedRules(ctx, ws)
	require.NoError(t, err)
	assert.Len(t, held, 2, "the project goes in the key, so two projects' operation 7 are two rules")

	require.NoError(t, contextop.Narrow(ctx, ws, "prj_docs", "7"))
	held, err = contextop.WidenedRules(ctx, ws)
	require.NoError(t, err)
	require.Len(t, held, 1)
	assert.Equal(t, "prj_web", string(held[0].Project))

	assert.Error(t, contextop.Widen(ctx, ws, contextop.Record{
		ID: "8", Subject: contextop.Subject{Kind: contextop.SubjectNote, Text: "a fact"},
	}), "a note states no rule to widen")
}

func termsOf(rules []profile.TermRule) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.Term)
	}
	return out
}
