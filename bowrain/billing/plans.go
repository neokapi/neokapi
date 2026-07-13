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
	// FeatureBravo gates the entire @bravo agent surface (chat panel, settings,
	// routes). It is dark by default — false on every plan — because @bravo's
	// self-hostable runtime is not launch-safe (see epic 015 / AD-016). The only
	// way to enable it is a per-workspace FeatureOverride via the control plane,
	// which lets the founder dogfood it without shipping it to customers. Flip a
	// plan's entry to true once the runtime + billing guardrails are hardened.
	FeatureBravo            Feature = "bravo"
	FeatureBravoCodeExec    Feature = "bravo-code-exec"
	FeatureConnectorsGit    Feature = "connectors-git"
	FeatureConnectorsCustom Feature = "connectors-custom"
	FeatureAPIAccess        Feature = "api-access"
	FeatureSSOSAML          Feature = "sso-saml"
)

// PlanFeatures defines which features are available on each plan.
// This is the source of truth for feature gating.
var PlanFeatures = map[Plan]map[Feature]bool{
	PlanFree: {
		FeatureBravo:            false,
		FeatureBravoCodeExec:    false,
		FeatureConnectorsGit:    false,
		FeatureConnectorsCustom: false,
		FeatureAPIAccess:        false,
		FeatureSSOSAML:          false,
	},
	PlanPro: {
		FeatureBravo:            false,
		FeatureBravoCodeExec:    false,
		FeatureConnectorsGit:    true,
		FeatureConnectorsCustom: false,
		FeatureAPIAccess:        true,
		FeatureSSOSAML:          false,
	},
	PlanTeam: {
		FeatureBravo:            false,
		FeatureBravoCodeExec:    true,
		FeatureConnectorsGit:    true,
		FeatureConnectorsCustom: true,
		FeatureAPIAccess:        true,
		FeatureSSOSAML:          false,
	},
	PlanEnterprise: {
		FeatureBravo:            false,
		FeatureBravoCodeExec:    true,
		FeatureConnectorsGit:    true,
		FeatureConnectorsCustom: true,
		FeatureAPIAccess:        true,
		FeatureSSOSAML:          true,
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

// CreditPackCredits is the credit grant for one purchased top-up pack. The pack
// is $5 (the dollar amount lives in Stripe, DECISIONS L4); this is the credit
// side of that same SKU, so it must stay in step with the price the provisioning
// tool creates and with what the pricing page advertises. Packs do not expire —
// they are drawn from only after the weekly plan allowance (Epic 004).
const CreditPackCredits = 200_000

// PerSeatPlans are the plans priced per seat: the Stripe subscription quantity
// is the seat count, so checkout must send it and seat changes re-price.
var PerSeatPlans = map[Plan]bool{
	PlanTeam: true,
}

// SelfServePlans are the plans a workspace owner can subscribe to from the app.
// Free is the downgrade target rather than a purchase, and Enterprise is sold
// by hand, so neither is self-serve.
var SelfServePlans = []Plan{PlanPro, PlanTeam}

// PlanDisplayNames are the human-readable plan names shown in the UI.
var PlanDisplayNames = map[Plan]string{
	PlanFree:       "Free",
	PlanPro:        "Pro",
	PlanTeam:       "Team",
	PlanEnterprise: "Enterprise",
}

// PlanInfo describes a plan to the client. It deliberately carries no dollar
// amounts: prices live in Stripe (DECISIONS L4), and the client's only job is to
// know which plans it may ask to check out.
type PlanInfo struct {
	ID            Plan   `json:"id"`
	Name          string `json:"name"`
	WeeklyCredits int64  `json:"weekly_credits"` // -1 = unlimited
	MaxProjects   int    `json:"max_projects"`   // -1 = unlimited
	MaxSeats      int    `json:"max_seats"`      // -1 = unlimited
	PerSeat       bool   `json:"per_seat"`
	Purchasable   bool   `json:"purchasable"`
	Current       bool   `json:"current"`
}

// DescribePlan builds the client-facing description of a plan. purchasable is
// decided by the caller, which knows whether the plan's Stripe price is
// configured in this deployment.
func DescribePlan(plan Plan, purchasable, current bool) PlanInfo {
	return PlanInfo{
		ID:            plan,
		Name:          PlanDisplayNames[plan],
		WeeklyCredits: CreditsForPlan(plan),
		MaxProjects:   GetLimit(plan, "max-projects"),
		MaxSeats:      GetLimit(plan, "max-seats"),
		PerSeat:       PerSeatPlans[plan],
		Purchasable:   purchasable,
		Current:       current,
	}
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
