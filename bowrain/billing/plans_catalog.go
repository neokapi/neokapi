package billing

import (
	"bytes"
	"encoding/json"
)

// LandingPlan is one plan's Go-sourced FACTS, serialized into the committed
// landing-page catalog (bowrain/web/landing/src/generated/plans.json). It
// deliberately carries no dollar price — that lives in Stripe (DECISIONS L4) and
// is hand-authored marketing copy on the landing page. The landing pairs these
// facts with the copy (price, description, feature phrasing, icons) keyed by ID.
type LandingPlan struct {
	ID             Plan   `json:"id"`
	Name           string `json:"name"`
	MonthlyCredits int64  `json:"monthlyCredits"` // -1 = unlimited, 0 = no recurring allowance
	MaxMarkets     int    `json:"maxMarkets"`     // -1 = unlimited
	MaxBrands      int    `json:"maxBrands"`      // -1 = unlimited
	MaxCustodians  int    `json:"maxCustodians"`  // 0 on Free, -1 = unlimited
	PerSeat        bool   `json:"perSeat"`
	SelfServe      bool   `json:"selfServe"`
}

// LandingPlanCatalog is the whole committed catalog: the per-plan facts plus the
// one-time trial-grant credit figure the pricing copy quotes.
type LandingPlanCatalog struct {
	Note              string        `json:"_note"`
	TrialGrantCredits int64         `json:"trialGrantCredits"`
	Plans             []LandingPlan `json:"plans"`
}

const landingCatalogNote = "GENERATED from bowrain/billing/plans.go by `go generate ./...` (directive in billing/gen.go). Do not edit by hand: change the facts in plans.go and regenerate. Facts only (ids, names, credits, limits, flags). Dollar prices are marketing copy in Plans.tsx / Stripe."

// BuildLandingPlanCatalog assembles the landing catalog from the plan tables in
// plans.go, in tier order. It is the single source the generator writes and the
// drift test regenerates, so the committed file can never silently disagree with
// plans.go.
func BuildLandingPlanCatalog() LandingPlanCatalog {
	selfServe := make(map[Plan]bool, len(SelfServePlans))
	for _, p := range SelfServePlans {
		selfServe[p] = true
	}
	plans := make([]LandingPlan, 0, len(planOrder))
	for _, p := range planOrder {
		plans = append(plans, LandingPlan{
			ID:             p,
			Name:           PlanDisplayNames[p],
			MonthlyCredits: CreditsForPlan(p),
			MaxMarkets:     GetLimit(p, LimitMaxMarkets),
			MaxBrands:      GetLimit(p, LimitMaxBrands),
			MaxCustodians:  GetLimit(p, LimitMaxCustodians),
			PerSeat:        PerSeatPlans[p],
			SelfServe:      selfServe[p],
		})
	}
	return LandingPlanCatalog{
		Note:              landingCatalogNote,
		TrialGrantCredits: FreeTrialGrantCredits,
		Plans:             plans,
	}
}

// MarshalLandingPlanCatalog renders the catalog as the exact bytes committed to
// plans.json: 2-space-indented JSON with a trailing newline, HTML escaping off,
// so the file is diff-clean and byte-identical between the generator and the
// drift test.
func MarshalLandingPlanCatalog() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(BuildLandingPlanCatalog()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
