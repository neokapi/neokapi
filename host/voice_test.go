package host

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/host/output"
)

func testProfile() *profile.VoiceProfile {
	return &profile.VoiceProfile{
		ID:   "test",
		Name: "Test",
		Vocabulary: profile.VocabularyRules{
			ForbiddenTerms:  []profile.TermRule{{Term: "utilize", Replacement: "use"}},
			CompetitorTerms: []profile.TermRule{{Term: "Globex", Replacement: "Acme"}},
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
	got, changes, skipped := RuleRewrite(testProfile(), text)
	want := "We use Acme tools to use growth."
	if got != want {
		t.Errorf("RuleRewrite = %q, want %q", got, want)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %+v", len(changes), changes)
	}
	if len(skipped) != 0 {
		t.Errorf("every rule named a replacement, so nothing is skipped: %+v", skipped)
	}
	// "utilize" appears twice.
	var utilize *output.VoiceChange
	for i := range changes {
		if changes[i].From == "utilize" {
			utilize = &changes[i]
		}
	}
	if utilize == nil || utilize.Count != 2 {
		t.Errorf("expected utilize change count 2, got %+v", utilize)
	}
}

func TestVoiceProfileTemplateParses(t *testing.T) {
	p, err := profile.LoadProfileYAML(strings.NewReader(VoiceProfileTemplate))
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
	tool := coretools.NewVoiceVocabCheckTool(testProfile(), nil)
	findings, err := RunBlockTool(t.Context(), tool, "We utilize Globex.")
	if err != nil {
		t.Fatalf("RunBlockTool: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings (forbidden + competitor), got %d: %+v", len(findings), findings)
	}
	score := profile.CalculateScore(findings)
	// One major (5) + one critical (25) = 30 penalty → 70.
	if score.Overall != 70 {
		t.Errorf("expected score 70, got %d", score.Overall)
	}
}
