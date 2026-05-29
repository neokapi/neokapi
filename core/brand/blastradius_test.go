package brand

import "testing"

func blocks() []EvalBlock {
	return []EvalBlock{
		{BlockID: "b1", CollectionID: "c1", CollectionName: "Marketing", Text: "Please utilize the dashboard"},
		{BlockID: "b2", CollectionID: "c1", CollectionName: "Marketing", Text: "Utilize it again and utilize more"},
		{BlockID: "b3", CollectionID: "c2", CollectionName: "Docs", Text: "A clean sentence with nothing to flag"},
	}
}

func TestEvaluateBlastRadius_PromotingForbiddenTerm(t *testing.T) {
	baseline := profileWith(nil, nil)
	candidate := CandidateWithRule(baseline, SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 4})

	br := EvaluateBlastRadius(blocks(), baseline, candidate)

	if br.TotalBlocks != 3 {
		t.Errorf("TotalBlocks = %d, want 3", br.TotalBlocks)
	}
	// b1 has one "utilize", b2 has two; b3 none.
	if br.NewViolations != 3 {
		t.Errorf("NewViolations = %d, want 3", br.NewViolations)
	}
	if br.ResolvedViolations != 0 {
		t.Errorf("ResolvedViolations = %d, want 0", br.ResolvedViolations)
	}
	if br.AffectedBlocks != 2 {
		t.Errorf("AffectedBlocks = %d, want 2", br.AffectedBlocks)
	}
	// Forbidden terms are major (not critical).
	if br.CriticalCount != 0 {
		t.Errorf("CriticalCount = %d, want 0", br.CriticalCount)
	}
	// New violations lower the score, so affected blocks degrade.
	if br.DegradedBlocks != 2 {
		t.Errorf("DegradedBlocks = %d, want 2", br.DegradedBlocks)
	}
	if br.ImprovedBlocks != 0 {
		t.Errorf("ImprovedBlocks = %d, want 0", br.ImprovedBlocks)
	}
	// Only c1 is affected (b1, b2); c2 (b3) is clean.
	if len(br.Collections) != 1 {
		t.Fatalf("Collections = %d, want 1: %+v", len(br.Collections), br.Collections)
	}
	c := br.Collections[0]
	if c.CollectionID != "c1" || c.AffectedBlocks != 2 {
		t.Errorf("collection = %+v, want c1 with 2 affected", c)
	}
	if c.AvgScoreDelta >= 0 {
		t.Errorf("AvgScoreDelta = %f, want negative (degradation)", c.AvgScoreDelta)
	}
}

func TestEvaluateBlastRadius_CandidateDoesNotMutateBaseline(t *testing.T) {
	baseline := profileWith([]TermRule{{Term: "existing"}}, nil)
	_ = CandidateWithRule(baseline, SuggestedRule{Term: "utilize"})
	if got := len(baseline.Vocabulary.ForbiddenTerms); got != 1 {
		t.Fatalf("baseline mutated: ForbiddenTerms = %d, want 1", got)
	}
}

func TestEvaluateBlastRadius_NoOpRule(t *testing.T) {
	baseline := profileWith(nil, nil)
	// A term that appears in none of the blocks.
	candidate := CandidateWithRule(baseline, SuggestedRule{Term: "nonexistentword"})
	br := EvaluateBlastRadius(blocks(), baseline, candidate)
	if br.AffectedBlocks != 0 || br.NewViolations != 0 || len(br.Collections) != 0 {
		t.Errorf("expected zero blast radius, got %+v", br)
	}
}

func TestEvaluateBlastRadius_ResolvedAndImproved(t *testing.T) {
	// Baseline flags a competitor term; the candidate drops it — content improves.
	baseline := profileWith(nil, []TermRule{{Term: "Globex"}})
	candidate := profileWith(nil, nil)
	bs := []EvalBlock{
		{BlockID: "b1", CollectionID: "c1", Text: "We beat Globex every day"},
		{BlockID: "b2", CollectionID: "c1", Text: "Nothing to see here"},
	}
	br := EvaluateBlastRadius(bs, baseline, candidate)
	if br.ResolvedViolations != 1 {
		t.Errorf("ResolvedViolations = %d, want 1", br.ResolvedViolations)
	}
	if br.NewViolations != 0 {
		t.Errorf("NewViolations = %d, want 0", br.NewViolations)
	}
	if br.ImprovedBlocks != 1 {
		t.Errorf("ImprovedBlocks = %d, want 1", br.ImprovedBlocks)
	}
	if br.AffectedBlocks != 1 {
		t.Errorf("AffectedBlocks = %d, want 1", br.AffectedBlocks)
	}
}

func TestEvaluateBlastRadius_CriticalCount(t *testing.T) {
	// Promoting a competitor term is critical severity.
	baseline := profileWith(nil, nil)
	candidate := profileWith(nil, []TermRule{{Term: "Globex"}})
	bs := []EvalBlock{{BlockID: "b1", CollectionID: "c1", Text: "Globex is the rival"}}
	br := EvaluateBlastRadius(bs, baseline, candidate)
	if br.NewViolations != 1 {
		t.Errorf("NewViolations = %d, want 1", br.NewViolations)
	}
	if br.CriticalCount != 1 {
		t.Errorf("CriticalCount = %d, want 1", br.CriticalCount)
	}
}

func TestProfileClone_Independent(t *testing.T) {
	p := &VoiceProfile{
		Name:       "base",
		Tone:       ToneProfile{Personality: []string{"warm"}},
		Vocabulary: VocabularyRules{ForbiddenTerms: []TermRule{{Term: "utilize"}}, Abbreviations: map[string]string{"e.g.": "for example"}},
		Locales:    map[string]LocaleOverride{"de": {Formality: "formal"}},
	}
	c := p.Clone()
	c.Vocabulary.ForbiddenTerms = append(c.Vocabulary.ForbiddenTerms, TermRule{Term: "leverage"})
	c.Tone.Personality[0] = "cold"
	c.Vocabulary.Abbreviations["i.e."] = "that is"
	c.Locales["fr"] = LocaleOverride{Formality: "casual"}

	if len(p.Vocabulary.ForbiddenTerms) != 1 {
		t.Errorf("clone leaked into baseline ForbiddenTerms: %+v", p.Vocabulary.ForbiddenTerms)
	}
	if p.Tone.Personality[0] != "warm" {
		t.Errorf("clone leaked into baseline Personality: %v", p.Tone.Personality)
	}
	if _, ok := p.Vocabulary.Abbreviations["i.e."]; ok {
		t.Errorf("clone leaked into baseline Abbreviations: %v", p.Vocabulary.Abbreviations)
	}
	if _, ok := p.Locales["fr"]; ok {
		t.Errorf("clone leaked into baseline Locales: %v", p.Locales)
	}
}
