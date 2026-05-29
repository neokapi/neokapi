package brand

import "testing"

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
