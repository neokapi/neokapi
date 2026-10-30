package profile

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// A no-op / empty blast radius must serialize Collections as `[]`, never `null`:
// the web blast-radius preview indexes `.length`/`.map` on it and a null crashes
// the whole page (full-screen error boundary).
func TestEvaluateBlastRadius_CollectionsNeverNullJSON(t *testing.T) {
	baseline := CarriedRuleSets(profileWith(nil, nil))
	candidate := CandidateWithRule(baseline, SuggestedRule{Term: "nonexistentword"})

	// No blocks at all — the case most likely to leave Collections nil.
	br := EvaluateBlastRadius(nil, baseline, candidate)
	if br.Collections == nil {
		t.Fatal("Collections is nil; want non-nil empty slice")
	}
	out, err := json.Marshal(br)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"collections":[]`) {
		t.Errorf(`JSON should contain "collections":[], got %s`, out)
	}
}

func blocks() []EvalBlock {
	return []EvalBlock{
		{BlockID: "b1", CollectionID: "c1", CollectionName: "Marketing", Text: "Please utilize the dashboard"},
		{BlockID: "b2", CollectionID: "c1", CollectionName: "Marketing", Text: "Utilize it again and utilize more"},
		{BlockID: "b3", CollectionID: "c2", CollectionName: "Docs", Text: "A clean sentence with nothing to flag"},
	}
}

func TestEvaluateBlastRadius_PromotingForbiddenTerm(t *testing.T) {
	baseline := CarriedRuleSets(profileWith(nil, nil))
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
	// Forbidden terms fail.
	if br.FailingCount != 3 {
		t.Errorf("FailingCount = %d, want 3", br.FailingCount)
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
	baseline := CarriedRuleSets(profileWith([]TermRule{{Term: "existing"}}, nil))
	_ = CandidateWithRule(baseline, SuggestedRule{Term: "utilize"})
	if got := len(baseline[0].Rules); got != 1 {
		t.Fatalf("baseline mutated: rules = %d, want 1", got)
	}
}

func TestEvaluateBlastRadius_NoOpRule(t *testing.T) {
	baseline := CarriedRuleSets(profileWith(nil, nil))
	// A term that appears in none of the blocks.
	candidate := CandidateWithRule(baseline, SuggestedRule{Term: "nonexistentword"})
	br := EvaluateBlastRadius(blocks(), baseline, candidate)
	if br.AffectedBlocks != 0 || br.NewViolations != 0 || len(br.Collections) != 0 {
		t.Errorf("expected zero blast radius, got %+v", br)
	}
}

func TestEvaluateBlastRadius_ResolvedAndImproved(t *testing.T) {
	// Baseline flags a competitor term; the candidate drops it — content improves.
	baseline := CarriedRuleSets(profileWith(nil, []TermRule{{Term: "Globex"}}))
	candidate := CarriedRuleSets(profileWith(nil, nil))
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

func TestEvaluateBlastRadius_FailingCount(t *testing.T) {
	// Promoting a competitor term is critical severity.
	baseline := CarriedRuleSets(profileWith(nil, nil))
	candidate := CarriedRuleSets(profileWith(nil, []TermRule{{Term: "Globex"}}))
	bs := []EvalBlock{{BlockID: "b1", CollectionID: "c1", Text: "Globex is the rival"}}
	br := EvaluateBlastRadius(bs, baseline, candidate)
	if br.NewViolations != 1 {
		t.Errorf("NewViolations = %d, want 1", br.NewViolations)
	}
	if br.FailingCount != 1 {
		t.Errorf("FailingCount = %d, want 1", br.FailingCount)
	}
}

func TestProfileClone_Independent(t *testing.T) {
	p := (&VoiceProfile{
		Name:    "base",
		Tone:    ToneProfile{Personality: []string{"warm"}},
		Locales: map[model.LocaleID]LocaleOverride{"de": {Formality: "formal"}},
	}).Carry("pack test", []TermRule{{Term: "utilize", Forms: []string{"utilizes"}}})
	c := p.Clone()
	c.CarriedTerms().Rules[0].Forms[0] = "utilizing"
	c.Tone.Personality[0] = "cold"
	c.Locales["fr"] = LocaleOverride{Formality: "casual"}

	if got := p.CarriedTerms().Rules[0].Forms[0]; got != "utilizes" {
		t.Errorf("clone leaked into baseline carried terms: %q", got)
	}
	if p.Tone.Personality[0] != "warm" {
		t.Errorf("clone leaked into baseline Personality: %v", p.Tone.Personality)
	}
	if _, ok := p.Locales["fr"]; ok {
		t.Errorf("clone leaked into baseline Locales: %v", p.Locales)
	}
}

// The blast radius matches each block through its runs, so a term beside a
// character reference is a new violation with a prescribed replacement, and a
// term a reference spells an edge of is a new violation that prescribes no
// edit (check.Fix makes none).
func TestEvaluateBlastRadiusBesideCharacterReferences(t *testing.T) {
	cases := []struct {
		name       string
		runs       []model.Run
		violations int
		prescribed int
	}{
		{"before a reference", []model.Run{model.TextR("utilize"), entity("1", "&amp;"), model.TextR("more")}, 1, 1},
		{"after a reference", []model.Run{model.TextR("word"), entity("1", "&nbsp;"), model.TextR("next")}, 1, 1},
		{"between references", []model.Run{entity("1", "&amp;"), model.TextR("utilize"), entity("2", "&nbsp;")}, 1, 1},
		{"a reference spells the last letter", []model.Run{model.TextR("caf"), entity("1", "&eacute;")}, 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := model.RunsText(tc.runs)
			stored := &model.Block{ID: "b1"}
			stored.SetSourceRuns(tc.runs)
			block := EvalBlockFor(stored, "c1", "c1")
			if block.Text != text || block.Runs == nil {
				t.Fatalf("EvalBlockFor = %+v, want the text and the runs that hold a reference", block)
			}
			br := EvaluateBlastRadius([]EvalBlock{block}, nil, referenceRules)
			if want := len(MatchTermRulesInRuns(referenceRules, text, tc.runs)); want != tc.violations {
				t.Fatalf("core/profile finds %d hits, the case expects %d", want, tc.violations)
			}
			if br.NewViolations != tc.violations {
				t.Errorf("NewViolations = %d, want %d", br.NewViolations, tc.violations)
			}
			if br.PrescribedBlocks != tc.prescribed {
				t.Errorf("PrescribedBlocks = %d, want %d", br.PrescribedBlocks, tc.prescribed)
			}
		})
	}

	// Runs that hold no reference change nothing a search finds, so the
	// projection keeps the text alone.
	plain := &model.Block{ID: "b2"}
	plain.SetSourceRuns([]model.Run{model.TextR("We "), model.TextR("utilize it")})
	if got := EvalBlockFor(plain, "c1", "c1"); got.Runs != nil || got.Text != "We utilize it" {
		t.Errorf("EvalBlockFor = %+v, want the text and no runs", got)
	}
}
