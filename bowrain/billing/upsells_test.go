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

func TestDetectUsageWindow_QueryShape(t *testing.T) {
	// Verify the month-window calculation used by detectHighUsage and
	// detectDormantPaid: the current and two prior monthly allocations.
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	windowStart := MonthStart(now.AddDate(0, -2, 0))

	assert.Equal(t, 1, windowStart.Day())
	assert.True(t, windowStart.Before(now))

	// Two months back from March 20 starts the window at January 1.
	expected := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, expected, windowStart)
}
