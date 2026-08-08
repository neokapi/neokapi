package billing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasFeature(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		plan      Plan
		feature   Feature
		overrides map[Feature]bool
		want      bool
	}{
		{"free has no git connectors", PlanFree, FeatureConnectorsGit, nil, false},
		{"pro has git connectors", PlanPro, FeatureConnectorsGit, nil, true},
		{"pro has no bravo code exec", PlanPro, FeatureBravoCodeExec, nil, false},
		{"team has bravo code exec", PlanTeam, FeatureBravoCodeExec, nil, true},
		// FeatureBravo is dark by default: false on every plan, override-only.
		{"free has no bravo", PlanFree, FeatureBravo, nil, false},
		{"pro has no bravo", PlanPro, FeatureBravo, nil, false},
		{"team has no bravo", PlanTeam, FeatureBravo, nil, false},
		{"enterprise has no bravo", PlanEnterprise, FeatureBravo, nil, false},
		{"override enables bravo for dogfooding", PlanFree, FeatureBravo, map[Feature]bool{FeatureBravo: true}, true},
		{"team has no sso", PlanTeam, FeatureSSOSAML, nil, false},
		{"enterprise has sso", PlanEnterprise, FeatureSSOSAML, nil, true},
		{"enterprise has all features", PlanEnterprise, FeatureSSOSAML, nil, true},
		{"free has api access — tokens are the front door to the kapi loop", PlanFree, FeatureAPIAccess, nil, true},
		{"pro has api access", PlanPro, FeatureAPIAccess, nil, true},
		{"override grants feature to free", PlanFree, FeatureConnectorsGit, map[Feature]bool{FeatureConnectorsGit: true}, true},
		{"override revokes feature from pro", PlanPro, FeatureConnectorsGit, map[Feature]bool{FeatureConnectorsGit: false}, false},
		{"override only affects specified feature", PlanFree, FeatureSSOSAML, map[Feature]bool{FeatureConnectorsGit: true}, false},
		{"unknown plan returns false", Plan("unknown"), FeatureAPIAccess, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := HasFeature(tt.plan, tt.feature, tt.overrides)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMinimumPlanFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		feature Feature
		want    Plan
	}{
		{FeatureConnectorsGit, PlanPro},
		{FeatureAPIAccess, PlanFree},
		{FeatureBravoCodeExec, PlanTeam},
		{FeatureConnectorsCustom, PlanTeam},
		{FeatureSSOSAML, PlanEnterprise},
		{FeatureBravo, Plan("")}, // dark on every plan → no minimum plan
	}

	for _, tt := range tests {
		t.Run(string(tt.feature), func(t *testing.T) {
			t.Parallel()
			got := MinimumPlanFor(tt.feature)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMinimumPlanFor_UnknownFeature(t *testing.T) {
	t.Parallel()
	got := MinimumPlanFor(Feature("nonexistent"))
	assert.Equal(t, Plan(""), got)
}

func TestPlanLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		plan      Plan
		limitName string
		want      int
	}{
		{"free max projects", PlanFree, "max-projects", 1},
		{"free max seats", PlanFree, "max-seats", 1},
		{"pro max projects", PlanPro, "max-projects", 10},
		{"pro max seats", PlanPro, "max-seats", 3},
		{"team unlimited projects", PlanTeam, "max-projects", -1},
		{"team unlimited seats", PlanTeam, "max-seats", -1},
		{"enterprise unlimited projects", PlanEnterprise, "max-projects", -1},
		{"unknown limit returns -1", PlanFree, "unknown-limit", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := GetLimit(tt.plan, tt.limitName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCreditsForPlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		plan Plan
		want int64
	}{
		// Free has NO recurring allowance — its credits are the one-time
		// FreeTrialGrantCredits grant, not a monthly allocation.
		{PlanFree, 0},
		{PlanPro, ProMonthlyCredits},
		{PlanTeam, TeamMonthlyCredits},
		{PlanEnterprise, -1},
	}

	for _, tt := range tests {
		t.Run(string(tt.plan), func(t *testing.T) {
			t.Parallel()
			got := CreditsForPlan(tt.plan)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCreditsForPlan_Unknown(t *testing.T) {
	t.Parallel()
	got := CreditsForPlan(Plan("unknown"))
	assert.Equal(t, int64(0), got, "unknown plan should default to free (no recurring allowance) — the safe default is to grant nothing")
}

// The provisional monthly numbers: Pro/Team are the former weekly allowances
// ×4, and the Free trial grant matches the credit pack. These pins exist so a
// retune is a conscious edit here plus the pricing surfaces, never an
// accident.
func TestMonthlyCreditConstants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, int64(2_000_000), int64(ProMonthlyCredits))
	assert.Equal(t, int64(8_000_000), int64(TeamMonthlyCredits))
	assert.Equal(t, int64(200_000), int64(FreeTrialGrantCredits))
}

func TestValidPlans(t *testing.T) {
	t.Parallel()
	assert.True(t, ValidPlans[PlanFree])
	assert.True(t, ValidPlans[PlanPro])
	assert.True(t, ValidPlans[PlanTeam])
	assert.True(t, ValidPlans[PlanEnterprise])
	assert.False(t, ValidPlans[Plan("unknown")])
}
