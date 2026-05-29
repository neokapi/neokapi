package brand

import (
	"testing"
	"time"
)

func TestShouldAutoPromote(t *testing.T) {
	tests := []struct {
		name      string
		threshold int
		count     int
		want      bool
	}{
		{"disabled by default", 0, 100, false},
		{"below threshold", 5, 4, false},
		{"at threshold", 5, 5, true},
		{"above threshold", 5, 9, true},
		{"negative threshold disabled", -1, 100, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &VoiceProfile{Autonomy: AutonomyConfig{AutoPromoteAtCount: tt.threshold}}
			if got := p.ShouldAutoPromote(SuggestedRule{CorrectionCount: tt.count}); got != tt.want {
				t.Errorf("ShouldAutoPromote(threshold=%d, count=%d) = %v, want %v", tt.threshold, tt.count, got, tt.want)
			}
		})
	}
	if (*VoiceProfile)(nil).ShouldAutoPromote(SuggestedRule{CorrectionCount: 99}) {
		t.Error("nil profile should never auto-promote")
	}
}

func TestMergeCandidates(t *testing.T) {
	now := time.Now().UTC()
	suggestions := []*SuggestedRule{
		{Term: "utilize", Replacement: "use", CorrectionCount: 4, Dimension: DimensionVocabulary},
		{Term: "leverage", Replacement: "use", CorrectionCount: 3, Dimension: DimensionVocabulary},
		{Term: "synergy", Replacement: "teamwork", CorrectionCount: 6, Dimension: DimensionVocabulary},
		{Term: "Globex", CorrectionCount: 5, Dimension: DimensionVocabulary},
	}
	decisions := []*RuleDecision{
		{Term: "Utilize", Status: RuleDecisionPromoted, PromotedVersion: 3, DecidedBy: "u1", DecidedAt: now},
		{Term: "leverage", Status: RuleDecisionRejected, DecidedBy: "u1", DecidedAt: now},
		{Term: "synergy", Status: RuleDecisionApproved, DecidedBy: "u2", DecidedAt: now},
	}

	t.Run("review view drops rejected and promoted", func(t *testing.T) {
		got := MergeCandidates(suggestions, decisions, false)
		// utilize (promoted) and leverage (rejected) drop; synergy (approved) and
		// Globex (pending) remain.
		if len(got) != 2 {
			t.Fatalf("got %d candidates, want 2: %+v", len(got), got)
		}
		byTerm := map[string]CandidateRule{}
		for _, c := range got {
			byTerm[c.Term] = c
		}
		if byTerm["synergy"].Status != RuleDecisionApproved {
			t.Errorf("synergy status = %q, want approved", byTerm["synergy"].Status)
		}
		if byTerm["Globex"].Status != RuleDecisionPending {
			t.Errorf("Globex status = %q, want pending", byTerm["Globex"].Status)
		}
		if byTerm["Globex"].DecidedAt != nil {
			t.Error("pending candidate should have nil DecidedAt")
		}
	})

	t.Run("full history includes resolved with provenance", func(t *testing.T) {
		got := MergeCandidates(suggestions, decisions, true)
		if len(got) != 4 {
			t.Fatalf("got %d candidates, want 4", len(got))
		}
		byTerm := map[string]CandidateRule{}
		for _, c := range got {
			byTerm[c.Term] = c
		}
		// Case-insensitive match: suggestion "utilize" ↔ decision "Utilize".
		u := byTerm["utilize"]
		if u.Status != RuleDecisionPromoted || u.PromotedVersion != 3 {
			t.Errorf("utilize = %+v, want promoted@v3", u)
		}
		if u.DecidedAt == nil {
			t.Error("promoted candidate should carry DecidedAt")
		}
	})
}
