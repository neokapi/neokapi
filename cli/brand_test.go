package cli

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/brand"
	coretools "github.com/neokapi/neokapi/core/tools"
)

func testProfile() *brand.VoiceProfile {
	return &brand.VoiceProfile{
		ID:   "test",
		Name: "Test",
		Vocabulary: brand.VocabularyRules{
			ForbiddenTerms:  []brand.TermRule{{Term: "utilize", Replacement: "use"}},
			CompetitorTerms: []brand.TermRule{{Term: "Globex", Replacement: "Acme"}},
		},
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Friendly DTC":            "friendly-dtc",
		"  Tech Docs!! ":          "tech-docs",
		"Already-slug":            "already-slug",
		"Multiple   spaces  here": "multiple-spaces-here",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRuleRewrite(t *testing.T) {
	text := "We utilize Globex tools to utilize growth."
	got, changes := ruleRewrite(testProfile(), text)
	want := "We use Acme tools to use growth."
	if got != want {
		t.Errorf("ruleRewrite = %q, want %q", got, want)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %+v", len(changes), changes)
	}
	// Competitor terms apply before forbidden; "utilize" appears twice.
	var utilize *output.BrandChange
	for i := range changes {
		if changes[i].From == "utilize" {
			utilize = &changes[i]
		}
	}
	if utilize == nil || utilize.Count != 2 {
		t.Errorf("expected utilize change count 2, got %+v", utilize)
	}
}

func TestBrandProfileTemplateParses(t *testing.T) {
	p, err := brand.LoadProfileYAML(strings.NewReader(brandProfileTemplate))
	if err != nil {
		t.Fatalf("brand new template must parse as a VoiceProfile: %v", err)
	}
	if p.Name == "" {
		t.Error("template profile has no name")
	}
	// The template's forbidden-term example must round-trip into a usable rule.
	var hasUtilize bool
	for _, r := range p.Vocabulary.ForbiddenTerms {
		if r.Term == "utilize" && r.Replacement == "use" {
			hasUtilize = true
		}
	}
	if !hasUtilize {
		t.Errorf("template forbidden terms missing utilize→use: %+v", p.Vocabulary.ForbiddenTerms)
	}
}

func TestRunBlockToolFindings(t *testing.T) {
	tool := coretools.NewBrandVocabCheckTool(testProfile(), nil)
	findings, err := runBlockTool(t.Context(), tool, "We utilize Globex.")
	if err != nil {
		t.Fatalf("runBlockTool: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings (forbidden + competitor), got %d: %+v", len(findings), findings)
	}
	score := brand.CalculateScore(findings)
	// One major (5) + one critical (25) = 30 penalty → 70.
	if score.Overall != 70 {
		t.Errorf("expected score 70, got %d", score.Overall)
	}
}
