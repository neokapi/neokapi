package brand

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

func profileWith(forbidden, competitor []TermRule) *VoiceProfile {
	return &VoiceProfile{Vocabulary: VocabularyRules{ForbiddenTerms: forbidden, CompetitorTerms: competitor}}
}

func TestMatchVocabulary(t *testing.T) {
	tests := []struct {
		name      string
		profile   *VoiceProfile
		text      string
		wantTerms []string   // matched terms in order
		wantSev   []Severity // parallel to wantTerms
		wantKind  []VocabKind
	}{
		{
			name:      "forbidden term defaults to major",
			profile:   profileWith([]TermRule{{Term: "utilize", Replacement: "use"}}, nil),
			text:      "Please utilize the dashboard",
			wantTerms: []string{"utilize"},
			wantSev:   []Severity{SeverityMajor},
			wantKind:  []VocabKind{VocabForbidden},
		},
		{
			name:      "competitor term defaults to critical",
			profile:   profileWith(nil, []TermRule{{Term: "Globex"}}),
			text:      "Unlike Globex, we ship faithfully",
			wantTerms: []string{"Globex"},
			wantSev:   []Severity{SeverityCritical},
			wantKind:  []VocabKind{VocabCompetitor},
		},
		{
			name:      "rule severity overrides the default",
			profile:   profileWith([]TermRule{{Term: "synergy", Severity: "minor"}}, nil),
			text:      "We love synergy here",
			wantTerms: []string{"synergy"},
			wantSev:   []Severity{SeverityMinor},
			wantKind:  []VocabKind{VocabForbidden},
		},
		{
			name:      "whole-word: Go does not match inside going",
			profile:   profileWith([]TermRule{{Term: "Go"}}, nil),
			text:      "We are going home",
			wantTerms: nil,
		},
		{
			name:      "case-insensitive match",
			profile:   profileWith([]TermRule{{Term: "utilize"}}, nil),
			text:      "Utilize it now",
			wantTerms: []string{"utilize"},
			wantSev:   []Severity{SeverityMajor},
			wantKind:  []VocabKind{VocabForbidden},
		},
		{
			name:      "multiple occurrences each report",
			profile:   profileWith([]TermRule{{Term: "utilize"}}, nil),
			text:      "utilize and utilize again",
			wantTerms: []string{"utilize", "utilize"},
			wantSev:   []Severity{SeverityMajor, SeverityMajor},
			wantKind:  []VocabKind{VocabForbidden, VocabForbidden},
		},
		{
			name:      "nil profile yields no hits",
			profile:   nil,
			text:      "anything",
			wantTerms: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := MatchVocabulary(tt.profile, tt.text)
			if len(hits) != len(tt.wantTerms) {
				t.Fatalf("got %d hits, want %d: %+v", len(hits), len(tt.wantTerms), hits)
			}
			for i, h := range hits {
				if h.Term != tt.wantTerms[i] {
					t.Errorf("hit %d: term = %q, want %q", i, h.Term, tt.wantTerms[i])
				}
				if h.Severity != tt.wantSev[i] {
					t.Errorf("hit %d: severity = %v, want %v", i, h.Severity, tt.wantSev[i])
				}
				if h.Kind != tt.wantKind[i] {
					t.Errorf("hit %d: kind = %v, want %v", i, h.Kind, tt.wantKind[i])
				}
				// The byte range must point back at a non-empty span of the text.
				if h.Start < 0 || h.End > len(tt.text) || h.Start >= h.End {
					t.Errorf("hit %d: bad range [%d,%d) for text len %d", i, h.Start, h.End, len(tt.text))
				}
			}
		})
	}
}

func TestMatchVocabulary_PropagatesConceptID(t *testing.T) {
	profile := profileWith(
		[]TermRule{{Term: "utilize", Replacement: "use", ConceptID: "concept-use"}},
		[]TermRule{{Term: "Globex", ConceptID: "concept-globex"}},
	)
	hits := MatchVocabulary(profile, "Please utilize unlike Globex")
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2: %+v", len(hits), hits)
	}
	byTerm := map[string]VocabHit{}
	for _, h := range hits {
		byTerm[h.Term] = h
	}
	if got := byTerm["utilize"].ConceptID; got != "concept-use" {
		t.Errorf("forbidden hit ConceptID = %q, want %q", got, "concept-use")
	}
	if got := byTerm["Globex"].ConceptID; got != "concept-globex" {
		t.Errorf("competitor hit ConceptID = %q, want %q", got, "concept-globex")
	}

	// A concept-less rule (standalone profile) yields an empty ConceptID.
	standalone := MatchVocabulary(profileWith([]TermRule{{Term: "utilize"}}, nil), "utilize it")
	if len(standalone) != 1 {
		t.Fatalf("got %d hits, want 1", len(standalone))
	}
	if standalone[0].ConceptID != "" {
		t.Errorf("standalone hit ConceptID = %q, want empty", standalone[0].ConceptID)
	}
}

func TestHitsToFindings(t *testing.T) {
	text := "Please utilize the Globex dashboard"
	runs := []model.Run{{Text: &model.TextRun{Text: text}}}
	profile := profileWith(
		[]TermRule{{Term: "utilize", Replacement: "use", Note: "prefer plain words", ConceptID: "c-use"}},
		[]TermRule{{Term: "Globex"}},
	)

	findings := HitsToFindings(MatchVocabulary(profile, text), text, runs)
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(findings), findings)
	}

	// Forbidden hit: note-bearing message, structured replacement + concept link,
	// and a run-anchored position computed from the byte range.
	f := findings[0]
	if f.OriginalText != "utilize" {
		t.Errorf("original text = %q, want %q", f.OriginalText, "utilize")
	}
	if want := `Forbidden term "utilize" found: prefer plain words`; f.Message != want {
		t.Errorf("message = %q, want %q", f.Message, want)
	}
	if want := `Use "use" instead`; f.Suggestion != want {
		t.Errorf("suggestion = %q, want %q", f.Suggestion, want)
	}
	if f.Metadata["replacement"] != "use" {
		t.Errorf("replacement metadata = %q, want %q", f.Metadata["replacement"], "use")
	}
	if f.Metadata["concept_id"] != "c-use" {
		t.Errorf("concept_id metadata = %q, want %q", f.Metadata["concept_id"], "c-use")
	}
	if f.Position.StartRun != 0 || f.Position.StartOffset != 7 || f.Position.EndOffset != 14 {
		t.Errorf("position = %+v, want run 0 [7,14)", f.Position)
	}

	// Competitor hit: no replacement and no concept on the rule, so the metadata
	// map stays nil (the keys are simply absent).
	comp := findings[1]
	if want := `Competitor term "Globex" found`; comp.Message != want {
		t.Errorf("competitor message = %q, want %q", comp.Message, want)
	}
	if comp.Metadata != nil {
		t.Errorf("concept-less, replacement-less competitor metadata = %+v, want nil", comp.Metadata)
	}

	// No hits → nil findings.
	if got := HitsToFindings(nil, text, runs); got != nil {
		t.Errorf("HitsToFindings(nil hits) = %+v, want nil", got)
	}

	// Nil runs (the run-less /check + MCP path): the position is left zero but the
	// message and concept metadata still flow through.
	noRuns := HitsToFindings(MatchVocabulary(profile, text), text, nil)
	if len(noRuns) != 2 {
		t.Fatalf("got %d findings without runs, want 2", len(noRuns))
	}
	if noRuns[0].Position != (model.RunRange{}) {
		t.Errorf("nil runs should yield a zero position, got %+v", noRuns[0].Position)
	}
	if noRuns[0].Metadata["concept_id"] != "c-use" {
		t.Errorf("concept_id must survive a run-less mapping, got %q", noRuns[0].Metadata["concept_id"])
	}
}

func TestSeverityForRule(t *testing.T) {
	cases := []struct {
		in   string
		want Severity
	}{
		{"", SeverityMajor}, // falls back to default
		{"minor", SeverityMinor},
		{"MAJOR", SeverityMajor},
		{" critical ", SeverityCritical}, // surrounding whitespace is trimmed
		{"neutral", SeverityNeutral},
		{"bogus", SeverityMajor}, // unknown → default
	}
	for _, c := range cases {
		if got := severityForRule(c.in, SeverityMajor); got != c.want {
			t.Errorf("severityForRule(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
