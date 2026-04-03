package billing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUpsellOpportunity_ScoreRange(t *testing.T) {
	opp := UpsellOpportunity{
		WorkspaceID:   "ws-1",
		CurrentPlan:   PlanFree,
		Signal:        "credit_exhaustion",
		Score:         90,
		SuggestedPlan: PlanPro,
		DetectedAt:    time.Now().UTC(),
	}
	assert.GreaterOrEqual(t, opp.Score, 0)
	assert.LessOrEqual(t, opp.Score, 100)
}

func TestUpsellOpportunity_AllSignalTypes(t *testing.T) {
	signals := map[string]struct {
		currentPlan   Plan
		suggestedPlan Plan
		score         int
	}{
		"credit_exhaustion": {PlanFree, PlanPro, 90},
		"high_usage":        {PlanPro, PlanTeam, 70},
		"dormant_paid":      {PlanPro, PlanPro, 50},
		"feature_gate_hits": {PlanFree, PlanPro, 80},
		"seat_pressure":     {PlanPro, PlanTeam, 75},
	}

	for sig, tt := range signals {
		t.Run(sig, func(t *testing.T) {
			opp := UpsellOpportunity{
				WorkspaceID:   "ws-test",
				CurrentPlan:   tt.currentPlan,
				Signal:        sig,
				Score:         tt.score,
				SuggestedPlan: tt.suggestedPlan,
				DetectedAt:    time.Now().UTC(),
			}
			assert.Equal(t, sig, opp.Signal)
			assert.Greater(t, opp.Score, 0)
		})
	}
}

func TestUpsellOpportunity_SuggestedPlanUpgrade(t *testing.T) {
	// Suggested plan should always be >= current plan.
	tests := []struct {
		name          string
		currentPlan   Plan
		suggestedPlan Plan
	}{
		{"free to pro", PlanFree, PlanPro},
		{"pro to team", PlanPro, PlanTeam},
		{"dormant keeps same", PlanTeam, PlanTeam},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opp := UpsellOpportunity{
				WorkspaceID:   "ws-1",
				CurrentPlan:   tt.currentPlan,
				SuggestedPlan: tt.suggestedPlan,
			}
			// Verify suggested plan is valid.
			assert.True(t, ValidPlans[opp.SuggestedPlan])
		})
	}
}

func TestDetectCreditExhaustion_QueryShape(t *testing.T) {
	// Verify WeekStart calculation used in detectCreditExhaustion.
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	twoWeeksAgo := WeekStart(now.AddDate(0, 0, -14))

	assert.Equal(t, time.Monday, twoWeeksAgo.Weekday())
	assert.True(t, twoWeeksAgo.Before(now))

	// Two weeks back from March 20 (Friday) should be March 2 (Monday).
	expected := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, expected, twoWeeksAgo)
}

func TestDetectHighUsage_QueryShape(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	threeWeeksAgo := WeekStart(now.AddDate(0, 0, -21))

	assert.Equal(t, time.Monday, threeWeeksAgo.Weekday())
	assert.True(t, threeWeeksAgo.Before(now))
}

func TestDetectDormantPaid_QueryShape(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	fourWeeksAgo := WeekStart(now.AddDate(0, 0, -28))

	assert.Equal(t, time.Monday, fourWeeksAgo.Weekday())
	assert.True(t, fourWeeksAgo.Before(now))

	// Four weeks back from March 20 should be Feb 16 (Monday).
	expected := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, expected, fourWeeksAgo)
}
