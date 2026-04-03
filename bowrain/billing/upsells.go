package billing

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// detectUpsells queries usage patterns to find workspaces ripe for upgrade.
func detectUpsells(ctx context.Context, db *sql.DB) ([]UpsellOpportunity, error) {
	var opportunities []UpsellOpportunity

	now := time.Now().UTC()

	// 1. Credit exhaustion: free workspaces hitting 100% two or more weeks.
	creditExhaustion, err := detectCreditExhaustion(ctx, db, now)
	if err != nil {
		return nil, fmt.Errorf("credit exhaustion: %w", err)
	}
	opportunities = append(opportunities, creditExhaustion...)

	// 2. High usage, low plan: workspaces consistently using >80% of credits.
	highUsage, err := detectHighUsage(ctx, db, now)
	if err != nil {
		return nil, fmt.Errorf("high usage: %w", err)
	}
	opportunities = append(opportunities, highUsage...)

	// 3. Dormant paid: paid workspaces with <10% usage for 4+ weeks.
	dormant, err := detectDormantPaid(ctx, db, now)
	if err != nil {
		return nil, fmt.Errorf("dormant paid: %w", err)
	}
	opportunities = append(opportunities, dormant...)

	// 4. Feature gate hits: workspaces with repeated 403 upgrade_required events.
	gateHits, err := detectFeatureGateHits(ctx, db, now)
	if err != nil {
		return nil, fmt.Errorf("feature gate hits: %w", err)
	}
	opportunities = append(opportunities, gateHits...)

	return opportunities, nil
}

func detectCreditExhaustion(ctx context.Context, db *sql.DB, now time.Time) ([]UpsellOpportunity, error) {
	// Find free workspaces that exhausted credits in 2+ of the last 4 weeks.
	twoWeeksAgo := WeekStart(now.AddDate(0, 0, -14))

	rows, err := db.QueryContext(ctx,
		`SELECT ca.workspace_id, s.plan, COUNT(*) as exhausted_weeks
		 FROM credit_allocations ca
		 JOIN subscriptions s ON s.workspace_id = ca.workspace_id
		 WHERE ca.source = 'plan'
		   AND ca.week_start >= $1
		   AND ca.credits_used >= ca.credits_total
		   AND s.plan = 'free'
		 GROUP BY ca.workspace_id, s.plan
		 HAVING COUNT(*) >= 2`, twoWeeksAgo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []UpsellOpportunity
	for rows.Next() {
		var wsID, plan string
		var weeks int
		if err := rows.Scan(&wsID, &plan, &weeks); err != nil {
			return nil, err
		}
		result = append(result, UpsellOpportunity{
			WorkspaceID:   wsID,
			CurrentPlan:   Plan(plan),
			Signal:        "credit_exhaustion",
			Score:         90,
			Detail:        fmt.Sprintf("Exhausted credits %d weeks in a row", weeks),
			SuggestedPlan: PlanPro,
			DetectedAt:    now,
		})
	}
	return result, rows.Err()
}

func detectHighUsage(ctx context.Context, db *sql.DB, now time.Time) ([]UpsellOpportunity, error) {
	threeWeeksAgo := WeekStart(now.AddDate(0, 0, -21))

	rows, err := db.QueryContext(ctx,
		`SELECT ca.workspace_id, s.plan,
		        AVG(CASE WHEN ca.credits_total > 0 THEN (ca.credits_used::float / ca.credits_total) * 100 ELSE 0 END) as avg_pct
		 FROM credit_allocations ca
		 JOIN subscriptions s ON s.workspace_id = ca.workspace_id
		 WHERE ca.source = 'plan'
		   AND ca.week_start >= $1
		   AND s.plan IN ('free', 'pro')
		 GROUP BY ca.workspace_id, s.plan
		 HAVING AVG(CASE WHEN ca.credits_total > 0 THEN (ca.credits_used::float / ca.credits_total) * 100 ELSE 0 END) > 80`, threeWeeksAgo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []UpsellOpportunity
	for rows.Next() {
		var wsID, plan string
		var avgPct float64
		if err := rows.Scan(&wsID, &plan, &avgPct); err != nil {
			return nil, err
		}
		suggested := PlanPro
		if Plan(plan) == PlanPro {
			suggested = PlanTeam
		}
		result = append(result, UpsellOpportunity{
			WorkspaceID:   wsID,
			CurrentPlan:   Plan(plan),
			Signal:        "high_usage",
			Score:         70,
			Detail:        fmt.Sprintf("Average credit usage %.0f%% over 3 weeks", avgPct),
			SuggestedPlan: suggested,
			DetectedAt:    now,
		})
	}
	return result, rows.Err()
}

func detectDormantPaid(ctx context.Context, db *sql.DB, now time.Time) ([]UpsellOpportunity, error) {
	fourWeeksAgo := WeekStart(now.AddDate(0, 0, -28))

	rows, err := db.QueryContext(ctx,
		`SELECT ca.workspace_id, s.plan,
		        AVG(CASE WHEN ca.credits_total > 0 THEN (ca.credits_used::float / ca.credits_total) * 100 ELSE 0 END) as avg_pct
		 FROM credit_allocations ca
		 JOIN subscriptions s ON s.workspace_id = ca.workspace_id
		 WHERE ca.source = 'plan'
		   AND ca.week_start >= $1
		   AND s.plan IN ('pro', 'team')
		 GROUP BY ca.workspace_id, s.plan
		 HAVING AVG(CASE WHEN ca.credits_total > 0 THEN (ca.credits_used::float / ca.credits_total) * 100 ELSE 0 END) < 10`, fourWeeksAgo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []UpsellOpportunity
	for rows.Next() {
		var wsID, plan string
		var avgPct float64
		if err := rows.Scan(&wsID, &plan, &avgPct); err != nil {
			return nil, err
		}
		result = append(result, UpsellOpportunity{
			WorkspaceID:   wsID,
			CurrentPlan:   Plan(plan),
			Signal:        "dormant_paid",
			Score:         50,
			Detail:        fmt.Sprintf("Average credit usage %.0f%% over 4 weeks", avgPct),
			SuggestedPlan: Plan(plan), // suggest keeping current plan (churn prevention)
			DetectedAt:    now,
		})
	}
	return result, rows.Err()
}

func detectFeatureGateHits(ctx context.Context, db *sql.DB, now time.Time) ([]UpsellOpportunity, error) {
	oneWeekAgo := now.AddDate(0, 0, -7)

	rows, err := db.QueryContext(ctx,
		`SELECT be.workspace_id, s.plan, COUNT(*) as gate_hits
		 FROM billing_events be
		 JOIN subscriptions s ON s.workspace_id = be.workspace_id
		 WHERE be.event_type = 'feature_gate_hit'
		   AND be.created_at >= $1
		 GROUP BY be.workspace_id, s.plan
		 HAVING COUNT(*) >= 3`, oneWeekAgo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []UpsellOpportunity
	for rows.Next() {
		var wsID, plan string
		var hits int
		if err := rows.Scan(&wsID, &plan, &hits); err != nil {
			return nil, err
		}
		suggested := PlanPro
		if Plan(plan) == PlanPro {
			suggested = PlanTeam
		}
		result = append(result, UpsellOpportunity{
			WorkspaceID:   wsID,
			CurrentPlan:   Plan(plan),
			Signal:        "feature_gate_hits",
			Score:         80,
			Detail:        fmt.Sprintf("Hit feature gates %d times this week", hits),
			SuggestedPlan: suggested,
			DetectedAt:    now,
		})
	}
	return result, rows.Err()
}
