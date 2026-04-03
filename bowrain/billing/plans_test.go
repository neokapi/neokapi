package billing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasFeature(t *testing.T) {
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
		{"team has no sso", PlanTeam, FeatureSSOSAML, nil, false},
		{"enterprise has sso", PlanEnterprise, FeatureSSOSAML, nil, true},
		{"enterprise has all features", PlanEnterprise, FeatureCustomMT, nil, true},
		{"free has no api access", PlanFree, FeatureAPIAccess, nil, false},
		{"pro has api access", PlanPro, FeatureAPIAccess, nil, true},
		{"override grants feature to free", PlanFree, FeatureConnectorsGit, map[Feature]bool{FeatureConnectorsGit: true}, true},
		{"override revokes feature from pro", PlanPro, FeatureConnectorsGit, map[Feature]bool{FeatureConnectorsGit: false}, false},
		{"override only affects specified feature", PlanFree, FeatureAPIAccess, map[Feature]bool{FeatureConnectorsGit: true}, false},
		{"unknown plan returns false", Plan("unknown"), FeatureAPIAccess, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasFeature(tt.plan, tt.feature, tt.overrides)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMinimumPlanFor(t *testing.T) {
	tests := []struct {
		feature Feature
		want    Plan
	}{
		{FeatureConnectorsGit, PlanPro},
		{FeatureAPIAccess, PlanPro},
		{FeatureCustomMT, PlanPro},
		{FeatureBravoCodeExec, PlanTeam},
		{FeatureConnectorsCustom, PlanTeam},
		{FeatureSSOSAML, PlanEnterprise},
	}

	for _, tt := range tests {
		t.Run(string(tt.feature), func(t *testing.T) {
			got := MinimumPlanFor(tt.feature)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMinimumPlanFor_UnknownFeature(t *testing.T) {
	got := MinimumPlanFor(Feature("nonexistent"))
	assert.Equal(t, Plan(""), got)
}

func TestPlanLimits(t *testing.T) {
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
			got := GetLimit(tt.plan, tt.limitName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCreditsForPlan(t *testing.T) {
	tests := []struct {
		plan Plan
		want int64
	}{
		{PlanFree, 50_000},
		{PlanPro, 500_000},
		{PlanTeam, 2_000_000},
		{PlanEnterprise, -1},
	}

	for _, tt := range tests {
		t.Run(string(tt.plan), func(t *testing.T) {
			got := CreditsForPlan(tt.plan)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCreditsForPlan_Unknown(t *testing.T) {
	got := CreditsForPlan(Plan("unknown"))
	assert.Equal(t, int64(50_000), got, "unknown plan should default to free credits")
}

func TestValidPlans(t *testing.T) {
	assert.True(t, ValidPlans[PlanFree])
	assert.True(t, ValidPlans[PlanPro])
	assert.True(t, ValidPlans[PlanTeam])
	assert.True(t, ValidPlans[PlanEnterprise])
	assert.False(t, ValidPlans[Plan("unknown")])
}
