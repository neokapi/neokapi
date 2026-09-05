package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewriteVocabulary(t *testing.T) {
	tests := []struct {
		name        string
		profile     *VoiceProfile
		text        string
		wantText    string
		wantChanges []RewriteChange
		wantSkipped []RewriteSkip
	}{
		{
			name:     "nil profile leaves the text alone",
			profile:  nil,
			text:     "Leverage it",
			wantText: "Leverage it",
		},
		{
			name: "rules without a replacement are reported, the text is unchanged",
			profile: profileWith([]TermRule{
				{Term: "utilize", Severity: "major"},
				{Term: "leverage", Severity: "major"},
				{Term: "cutting-edge", Severity: "major"},
			}, nil),
			text:     "Leverage our cutting-edge workspace to utilize your content.",
			wantText: "Leverage our cutting-edge workspace to utilize your content.",
			wantSkipped: []RewriteSkip{
				{Term: "utilize", List: "forbidden", Severity: SeverityMajor, Matched: []string{"utilize"}, Count: 1, Reason: RewriteSkipNoReplacement},
				{Term: "leverage", List: "forbidden", Severity: SeverityMajor, Matched: []string{"Leverage"}, Count: 1, Reason: RewriteSkipNoReplacement},
				{Term: "cutting-edge", List: "forbidden", Severity: SeverityMajor, Matched: []string{"cutting-edge"}, Count: 1, Reason: RewriteSkipNoReplacement},
			},
		},
		{
			name: "a mixed profile substitutes what it can and reports the rest",
			profile: profileWith(
				[]TermRule{
					{Term: "utilize", Replacement: "use"},
					{Term: "leverage", Severity: "minor", Note: "say what the reader does"},
				},
				[]TermRule{{Term: "Globex"}},
			),
			text:     "Leverage Globex to utilize your content. Utilize it well.",
			wantText: "Leverage Globex to use your content. use it well.",
			wantChanges: []RewriteChange{
				{Term: "utilize", Replacement: "use", List: "forbidden", Count: 2},
			},
			wantSkipped: []RewriteSkip{
				{Term: "leverage", List: "forbidden", Severity: SeverityMinor, Note: "say what the reader does", Matched: []string{"Leverage"}, Count: 1, Reason: RewriteSkipNoReplacement},
				{Term: "Globex", List: "competitor", Severity: SeverityCritical, Matched: []string{"Globex"}, Count: 1, Reason: RewriteSkipNoReplacement},
			},
		},
		{
			name:     "whole words only: use inside user is untouched",
			profile:  profileWith([]TermRule{{Term: "use", Replacement: "apply"}}, nil),
			text:     "The user can use it",
			wantText: "The user can apply it",
			wantChanges: []RewriteChange{
				{Term: "use", Replacement: "apply", List: "forbidden", Count: 1},
			},
		},
		{
			name: "a declared inflected form is reported with the bare replacement, never substituted",
			profile: profileWith([]TermRule{
				{Term: "utilize", Replacement: "use", Forms: []string{"utilizes", "utilizing", "utilized"}},
			}, nil),
			text:     "We are utilizing what they utilized; utilize it.",
			wantText: "We are utilizing what they utilized; use it.",
			wantChanges: []RewriteChange{
				{Term: "utilize", Replacement: "use", List: "forbidden", Count: 1},
			},
			wantSkipped: []RewriteSkip{
				{Term: "utilize", List: "forbidden", Severity: SeverityMajor, Replacement: "use", Matched: []string{"utilizing", "utilized"}, Count: 2, Reason: RewriteSkipInflectedForm},
			},
		},
		{
			name: "a prose-scoped rule leaves code spans alone and says where it applies",
			profile: profileWith([]TermRule{
				{Term: "daemon", Severity: "minor", Scope: ScopeProse},
			}, nil),
			text:     "The daemon starts with `daemon --start`.",
			wantText: "The daemon starts with `daemon --start`.",
			wantSkipped: []RewriteSkip{
				{Term: "daemon", List: "forbidden", Severity: SeverityMinor, Scope: ScopeProse, Matched: []string{"daemon"}, Count: 1, Reason: RewriteSkipNoReplacement},
			},
		},
		{
			name: "a case-sensitive rule substitutes only its own casing",
			profile: profileWith([]TermRule{
				{Term: "Ripgrep", Replacement: "ripgrep", CaseSensitive: true},
			}, nil),
			text:     "Ripgrep is fast; ripgrep stays lowercase.",
			wantText: "ripgrep is fast; ripgrep stays lowercase.",
			wantChanges: []RewriteChange{
				{Term: "Ripgrep", Replacement: "ripgrep", List: "forbidden", Count: 1},
			},
		},
		{
			name: "a hit inside replaced text is neither substituted nor reported",
			profile: profileWith([]TermRule{
				{Term: "cutting-edge", Replacement: "current"},
				{Term: "edge", Severity: "minor"},
			}, nil),
			text:     "A cutting-edge edge case.",
			wantText: "A current edge case.",
			wantChanges: []RewriteChange{
				{Term: "cutting-edge", Replacement: "current", List: "forbidden", Count: 1},
			},
			wantSkipped: []RewriteSkip{
				{Term: "edge", List: "forbidden", Severity: SeverityMinor, Matched: []string{"edge"}, Count: 1, Reason: RewriteSkipNoReplacement},
			},
		},
		{
			name:     "a concept-backed rule carries its concept through",
			profile:  profileWith([]TermRule{{Term: "workspace", ConceptID: "c-42"}}, nil),
			text:     "Open the workspace.",
			wantText: "Open the workspace.",
			wantSkipped: []RewriteSkip{
				{Term: "workspace", List: "forbidden", Severity: SeverityMajor, ConceptID: "c-42", Matched: []string{"workspace"}, Count: 1, Reason: RewriteSkipNoReplacement},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RewriteVocabulary(tt.profile, tt.text)
			assert.Equal(t, tt.wantText, got.Text)
			assert.Equal(t, tt.wantChanges, got.Changes)
			assert.Equal(t, tt.wantSkipped, got.Skipped)
		})
	}
}

// TestRewriteVocabulary_AgreesWithCheck pins the property the report exists
// for: every hit the vocabulary check raises is either substituted or
// reported, so the counts a caller sees add up to what `voice check` finds.
func TestRewriteVocabulary_AgreesWithCheck(t *testing.T) {
	p := profileWith(
		[]TermRule{
			{Term: "utilize", Replacement: "use", Forms: []string{"utilizing"}},
			{Term: "leverage"},
		},
		[]TermRule{{Term: "Globex"}},
	)
	text := "Leverage Globex; utilize and keep utilizing. Leverage again."
	hits := MatchVocabulary(p, text)
	require.NotEmpty(t, hits)

	got := RewriteVocabulary(p, text)
	total := 0
	for _, c := range got.Changes {
		total += c.Count
	}
	for _, s := range got.Skipped {
		total += s.Count
	}
	assert.Equal(t, len(hits), total, "changes + skipped must account for every hit")

	skippedTotal := 0
	for _, s := range got.Skipped {
		skippedTotal += s.Count
	}
	assert.Len(t, MatchVocabulary(p, got.Text), skippedTotal,
		"what the check still finds in the rewritten text is exactly what was reported skipped")
}
