package profile

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
	for i := range 5 {
		if got := RenderVoiceGuide(p); got != first {
			t.Fatalf("RenderVoiceGuide not deterministic on run %d", i)
		}
	}
	for _, want := range []string{
		"# Voice Guide: Acme Voice",
		"- Personality: friendly, direct",
		"- Use active voice",
		"~~utilize~~ → use **use**",
		"### Competitor Terms",
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

// TestRenderVoiceGuideCompactAllFields pins the no-dead-context contract: every
// populated constraint of a VoiceProfile that the full guide renders must
// materially appear in the compact form too — tone guidelines, humor,
// sentence-length ceilings, point of view, prohibited patterns, and forbidden/
// competitor terms even when they carry no replacement.
func TestRenderVoiceGuideCompactAllFields(t *testing.T) {
	p := &VoiceProfile{
		Name:        "Full Voice",
		Description: "Everything populated.",
		Tone: ToneProfile{
			Personality: []string{"calm", "expert"},
			Formality:   "formal",
			Emotion:     "authoritative",
			Humor:       "none",
			Guidelines:  "Address the reader as a peer",
		},
		Style: StyleRules{
			ActiveVoice:    true,
			SentenceLength: "short",
			PersonPOV:      "second",
			Contractions:   "never",
			ProhibitedPatterns: []Pattern{
				{Regex: `!{2,}`, Description: "no stacked exclamation marks", Severity: "major"},
				{Regex: `\bvery\b`, Severity: "minor"}, // description-less: the regex must surface
			},
		},
		Vocabulary: VocabularyRules{
			ForbiddenTerms: []TermRule{
				{Term: "synergy", Replacement: "teamwork"},
				{Term: "world-class"}, // no replacement: must still render as a ban
			},
			CompetitorTerms: []TermRule{
				{Term: "Globex"}, // no replacement: must still render as a ban
			},
		},
	}

	got := RenderVoiceGuideCompact(p)
	for _, want := range []string{
		"personality: calm, expert",
		"formality: formal",
		"emotion: authoritative",
		"humor: none",
		"use active voice",
		"sentence length: short",
		"point of view: second",
		"contractions: never",
		"Address the reader as a peer",
		"no stacked exclamation marks",
		`\bvery\b`,
		`"synergy" → "teamwork"`,
		`"world-class"`,
		`"Globex"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("compact guide missing %q\n---\n%s", want, got)
		}
	}

	// Deterministic across renders.
	for i := range 3 {
		if again := RenderVoiceGuideCompact(p); again != got {
			t.Fatalf("RenderVoiceGuideCompact not deterministic on run %d", i)
		}
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

// TestGuideDoesNotBanAWordInFavourOfItself.
//
// A rule whose replacement IS its term states a convention — use this word, and
// here is how — not a ban. Rendered as a swap it produced, from a profile kapi
// inferred from ripgrep's own documentation:
//
//   - ~~grep~~ → use **grep**
//
// which tells a model to avoid a word in favour of that word. The note carried
// the real rule and was rendered for preferred terms only, so the section where
// it was the entire meaning was the one that dropped it.
func TestGuideDoesNotBanAWordInFavourOfItself(t *testing.T) {
	p := &VoiceProfile{
		Name: "ripgrep",
		Vocabulary: VocabularyRules{
			CompetitorTerms: []TermRule{
				{Term: "grep", Replacement: "grep", Note: "Named plainly; no put-downs."},
				{Term: "ag", Replacement: "The Silver Searcher"},
			},
			ForbiddenTerms: []TermRule{
				{Term: "Ripgrep", Replacement: "ripgrep", Note: "Lowercase in every occurrence."},
			},
		},
	}
	got := RenderVoiceGuide(p)

	assert.NotContains(t, got, "~~grep~~ → use **grep**", "a word is never banned in favour of itself")
	assert.Contains(t, got, "- **grep**: Named plainly; no put-downs.",
		"and the note says what the rule actually is")
	assert.Contains(t, got, "- ~~ag~~ → use **The Silver Searcher**", "a real swap still renders")

	// A replacement differing only in case is a real rule about capitalisation,
	// so the comparison must be exact rather than case-folded.
	assert.Contains(t, got, "- ~~Ripgrep~~ → use **ripgrep**: Lowercase in every occurrence.")
}

// TestCompactGuideDropsTheNoOpSwapToo: the translation path renders its own
// term list, and had the same defect.
func TestCompactGuideDropsTheNoOpSwapToo(t *testing.T) {
	p := &VoiceProfile{
		Vocabulary: VocabularyRules{
			CompetitorTerms: []TermRule{
				{Term: "grep", Replacement: "grep"},
				{Term: "ag", Replacement: "The Silver Searcher"},
			},
		},
	}
	got := RenderVoiceGuideCompact(p)
	assert.NotContains(t, got, `"grep" → "grep"`)
	assert.Contains(t, got, `"ag" → "The Silver Searcher"`)
}

// TestPatternReachesTheModelWithItsWords is issue #2240.
//
// The description said WHY a pattern is banned and the regex says WHAT is
// banned, and the description won. A rule with a careful description reached
// the model naming none of the words it forbids, so the document written under
// it used them — each one a violation the check would then flag. A user who
// documented their rule made the guide less actionable than one who did not.
func TestPatternReachesTheModelWithItsWords(t *testing.T) {
	p := &VoiceProfile{
		Style: StyleRules{ProhibitedPatterns: []Pattern{{
			Regex:       `(?i)\b(?:endpoint|payload|webhook|HMAC|API)\b`,
			Description: "implementation vocabulary, which this reader does not have",
		}}},
	}
	got := RenderVoiceGuideCompact(p)

	assert.Contains(t, got, "implementation vocabulary", "the reason survives")
	for _, w := range []string{"endpoint", "payload", "webhook"} {
		assert.Contains(t, got, w, "the model is told which word to avoid: %s", w)
	}
}

// TestPatternWithoutWordsKeepsItsDescription: a pattern built from character
// classes has no words to extract, and inventing some would be worse than the
// description alone.
func TestPatternWithoutWordsKeepsItsDescription(t *testing.T) {
	p := &VoiceProfile{
		Style: StyleRules{ProhibitedPatterns: []Pattern{{
			Regex:       `\s+\w+(?:ed|en)\b`,
			Description: "passive construction",
		}}},
	}
	got := RenderVoiceGuideCompact(p)
	assert.Contains(t, got, "passive construction")
	assert.NotContains(t, got, "such as", "nothing extractable, so nothing claimed")
}

// TestDuplicatePatternHintsAreDropped: two patterns sharing a description
// rendered the same sentence twice and spent context on nothing.
func TestDuplicatePatternHintsAreDropped(t *testing.T) {
	p := &VoiceProfile{
		Style: StyleRules{ProhibitedPatterns: []Pattern{
			{Regex: `\x{2014}`, Description: "em dashes"},
			{Regex: `\x{2013}`, Description: "em dashes"},
		}},
	}
	got := RenderVoiceGuideCompact(p)
	assert.Equal(t, 1, strings.Count(got, "em dashes"))
}

// TestCompactGuideCarriesTheExamples.
//
// A before/after pair is the strongest steering a profile has — describing a
// register has been measured not to move a model, and showing one has. The
// compact form dropped them entirely, and it is what the translation path uses:
// the one place kapi writes prose on a user's behalf at scale.
func TestCompactGuideCarriesTheExamples(t *testing.T) {
	p := &VoiceProfile{Examples: []VoiceExample{
		{Before: "RipGrep is blazingly fast", After: "ripgrep uses parallelism to search"},
	}}
	got := RenderVoiceGuideCompact(p)
	assert.Contains(t, got, "RipGrep is blazingly fast")
	assert.Contains(t, got, "ripgrep uses parallelism to search")
}
