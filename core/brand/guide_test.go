package brand

import (
	"strings"
	"testing"
)

func sampleProfile() *VoiceProfile {
	return &VoiceProfile{
		Name:        "Acme Voice",
		Description: "Friendly but precise.",
		Tone: ToneProfile{
			Personality: []string{"friendly", "direct"},
			Formality:   "neutral",
			Emotion:     "warm",
			Humor:       "light",
		},
		Style: StyleRules{
			ActiveVoice:  true,
			Contractions: "always",
		},
		Vocabulary: VocabularyRules{
			ForbiddenTerms: []TermRule{
				{Term: "utilize", Replacement: "use"},
				{Term: "leverage", Replacement: "use"},
			},
			CompetitorTerms: []TermRule{
				{Term: "Globex", Replacement: "Acme"},
			},
		},
	}
}

func TestRenderVoiceGuideDeterministic(t *testing.T) {
	p := sampleProfile()
	first := RenderVoiceGuide(p)
	for i := 0; i < 5; i++ {
		if got := RenderVoiceGuide(p); got != first {
			t.Fatalf("RenderVoiceGuide not deterministic on run %d", i)
		}
	}
	for _, want := range []string{
		"# Brand Voice Guide: Acme Voice",
		"- Personality: friendly, direct",
		"- Use active voice",
		"~~utilize~~ → use **use**",
		"### Competitor Terms (avoid)",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("guide missing %q\n---\n%s", want, first)
		}
	}
}

func TestRenderVoiceGuideNil(t *testing.T) {
	if got := RenderVoiceGuide(nil); got != "" {
		t.Errorf("expected empty string for nil profile, got %q", got)
	}
}

func TestRenderVoiceGuideCompact(t *testing.T) {
	got := RenderVoiceGuideCompact(sampleProfile())
	for _, want := range []string{
		"personality: friendly, direct",
		"use active voice",
		`"leverage" → "use"`,
		`"utilize" → "use"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("compact guide missing %q\n---\n%s", want, got)
		}
	}
	// term swaps must be sorted: "leverage" before "utilize".
	if strings.Index(got, "leverage") > strings.Index(got, "utilize") {
		t.Errorf("term swaps not sorted: %s", got)
	}
}

func TestLoadProfileYAML(t *testing.T) {
	const doc = `
name: Test Voice
description: A test profile
tone:
  personality: [crisp]
  formality: formal
vocabulary:
  forbidden_terms:
    - term: synergy
      replacement: teamwork
`
	p, err := LoadProfileYAML(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("LoadProfileYAML: %v", err)
	}
	if p.Name != "Test Voice" {
		t.Errorf("name = %q", p.Name)
	}
	if len(p.Vocabulary.ForbiddenTerms) != 1 || p.Vocabulary.ForbiddenTerms[0].Replacement != "teamwork" {
		t.Errorf("forbidden terms not parsed: %+v", p.Vocabulary.ForbiddenTerms)
	}
}

func TestLoadProfileYAMLInvalid(t *testing.T) {
	if _, err := LoadProfileYAML(strings.NewReader("\tnot: [valid")); err == nil {
		t.Error("expected error for invalid YAML")
	}
}
