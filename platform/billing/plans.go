// Package billing implements plan management, usage credits, Stripe integration,
// and feature gating for the Bowrain platform.
package billing

// Plan represents a subscription tier.
type Plan string

const (
	PlanFree       Plan = "free"
	PlanPro        Plan = "pro"
	PlanTeam       Plan = "team"
	PlanEnterprise Plan = "enterprise"
)

// ValidPlans is the set of valid Plan values.
var ValidPlans = map[Plan]bool{
	PlanFree:       true,
	PlanPro:        true,
	PlanTeam:       true,
	PlanEnterprise: true,
}

// Feature represents a gated platform capability.
type Feature string

const (
	FeatureBravoCodeExec    Feature = "bravo-code-exec"
	FeatureConnectorsGit    Feature = "connectors-git"
	FeatureConnectorsCustom Feature = "connectors-custom"
	FeatureAPIAccess        Feature = "api-access"
	FeatureSSOSAML          Feature = "sso-saml"
	FeatureCustomMT         Feature = "custom-mt-providers"
)

// PlanFeatures defines which features are available on each plan.
// This is the source of truth for feature gating.
var PlanFeatures = map[Plan]map[Feature]bool{
	PlanFree: {
		FeatureBravoCodeExec:    false,
		FeatureConnectorsGit:    false,
		FeatureConnectorsCustom: false,
		FeatureAPIAccess:        false,
		FeatureSSOSAML:          false,
		FeatureCustomMT:         false,
	},
	PlanPro: {
		FeatureBravoCodeExec:    false,
		FeatureConnectorsGit:    true,
		FeatureConnectorsCustom: false,
		FeatureAPIAccess:        true,
		FeatureSSOSAML:          false,
		FeatureCustomMT:         true,
	},
	PlanTeam: {
		FeatureBravoCodeExec:    true,
		FeatureConnectorsGit:    true,
		FeatureConnectorsCustom: true,
		FeatureAPIAccess:        true,
		FeatureSSOSAML:          false,
		FeatureCustomMT:         true,
	},
	PlanEnterprise: {
		FeatureBravoCodeExec:    true,
		FeatureConnectorsGit:    true,
		FeatureConnectorsCustom: true,
		FeatureAPIAccess:        true,
		FeatureSSOSAML:          true,
		FeatureCustomMT:         true,
	},
}

// PlanLimits defines numeric limits per plan.
// A value of -1 means unlimited.
var PlanLimits = map[Plan]map[string]int{
	PlanFree:       {"max-projects": 1, "max-seats": 1},
	PlanPro:        {"max-projects": 10, "max-seats": 3},
	PlanTeam:       {"max-projects": -1, "max-seats": -1},
	PlanEnterprise: {"max-projects": -1, "max-seats": -1},
}

// WeeklyCredits defines the weekly AI credit allocation per plan.
// A value of -1 means unlimited (Enterprise).
var WeeklyCredits = map[Plan]int64{
	PlanFree:       50_000,
	PlanPro:        500_000,
	PlanTeam:       2_000_000,
	PlanEnterprise: -1,
}

// planOrder defines the hierarchy from lowest to highest tier.
var planOrder = []Plan{PlanFree, PlanPro, PlanTeam, PlanEnterprise}

// HasFeature checks whether a plan grants access to a feature,
// considering optional per-workspace overrides. Overrides take
// precedence over the plan matrix.
func HasFeature(plan Plan, feature Feature, overrides ...map[Feature]bool) bool {
	// Check per-workspace override first.
	if len(overrides) > 0 && overrides[0] != nil {
		if enabled, ok := overrides[0][feature]; ok {
			return enabled
		}
	}
	// Fall back to plan matrix.
	if features, ok := PlanFeatures[plan]; ok {
		return features[feature]
	}
	return false
}

// MinimumPlanFor returns the lowest plan that includes the given feature.
// Returns an empty string if no plan includes it.
func MinimumPlanFor(feature Feature) Plan {
	for _, p := range planOrder {
		if features, ok := PlanFeatures[p]; ok {
			if features[feature] {
				return p
			}
		}
	}
	return ""
}

// CreditsForPlan returns the weekly credit allocation for a plan.
// Returns -1 for unlimited (Enterprise).
func CreditsForPlan(plan Plan) int64 {
	if c, ok := WeeklyCredits[plan]; ok {
		return c
	}
	return WeeklyCredits[PlanFree]
}

// GetLimit returns a numeric limit for a plan. Returns -1 if the limit
// is not defined (treated as unlimited).
func GetLimit(plan Plan, limitName string) int {
	if limits, ok := PlanLimits[plan]; ok {
		if v, ok := limits[limitName]; ok {
			return v
		}
	}
	return -1
}
