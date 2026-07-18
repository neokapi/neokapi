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

	// 1. Credit exhaustion: free workspaces that have fully spent their
	// one-time trial grant.
	creditExhaustion, err := detectCreditExhaustion(ctx, db, now)
	if err != nil {
		return nil, fmt.Errorf("credit exhaustion: %w", err)
	}
	opportunities = append(opportunities, creditExhaustion...)

	// 2. High usage, low plan: paid workspaces consistently using >80% of
	// their monthly credits.
	highUsage, err := detectHighUsage(ctx, db, now)
	if err != nil {
		return nil, fmt.Errorf("high usage: %w", err)
	}
	opportunities = append(opportunities, highUsage...)

	// 3. Dormant paid: paid workspaces with <10% usage across recent months.
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
	// Free workspaces whose one-time trial grant is fully spent. With no
	// recurring Free allowance, a spent grant is the hard stop — the strongest
	// upgrade signal there is.
	rows, err := db.QueryContext(ctx,
		`SELECT ca.workspace_id, s.plan
		 FROM credit_allocations ca
		 JOIN subscriptions s ON s.workspace_id = ca.workspace_id
		 WHERE ca.source = 'trial'
		   AND ca.credits_used >= ca.credits_total
		   AND s.plan = 'free'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []UpsellOpportunity
	for rows.Next() {
		var wsID, plan string
		if err := rows.Scan(&wsID, &plan); err != nil {
			return nil, err
		}
		result = append(result, UpsellOpportunity{
			WorkspaceID:   wsID,
			CurrentPlan:   Plan(plan),
			Signal:        "credit_exhaustion",
			Score:         90,
			Detail:        "One-time trial credit grant fully spent",
			SuggestedPlan: PlanPro,
			DetectedAt:    now,
		})
	}
	return result, rows.Err()
}

func detectHighUsage(ctx context.Context, db *sql.DB, now time.Time) ([]UpsellOpportunity, error) {
	// Monthly plan allocations for the current and two prior months. Only paid
	// plans have plan allocations now; Free is covered by trial exhaustion.
	windowStart := MonthStart(now.AddDate(0, -2, 0))

	rows, err := db.QueryContext(ctx,
		`SELECT ca.workspace_id, s.plan,
		        AVG(CASE WHEN ca.credits_total > 0 THEN (ca.credits_used::float / ca.credits_total) * 100 ELSE 0 END) as avg_pct
		 FROM credit_allocations ca
		 JOIN subscriptions s ON s.workspace_id = ca.workspace_id
		 WHERE ca.source = 'plan'
		   AND ca.week_start >= $1
		   AND s.plan = 'pro'
		 GROUP BY ca.workspace_id, s.plan
		 HAVING AVG(CASE WHEN ca.credits_total > 0 THEN (ca.credits_used::float / ca.credits_total) * 100 ELSE 0 END) > 80`, windowStart)
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
			Signal:        "high_usage",
			Score:         70,
			Detail:        fmt.Sprintf("Average monthly credit usage %.0f%% over recent months", avgPct),
			SuggestedPlan: PlanTeam,
			DetectedAt:    now,
		})
	}
	return result, rows.Err()
}

func detectDormantPaid(ctx context.Context, db *sql.DB, now time.Time) ([]UpsellOpportunity, error) {
	// Current and two prior monthly allocations.
	windowStart := MonthStart(now.AddDate(0, -2, 0))

	rows, err := db.QueryContext(ctx,
		`SELECT ca.workspace_id, s.plan,
		        AVG(CASE WHEN ca.credits_total > 0 THEN (ca.credits_used::float / ca.credits_total) * 100 ELSE 0 END) as avg_pct
		 FROM credit_allocations ca
		 JOIN subscriptions s ON s.workspace_id = ca.workspace_id
		 WHERE ca.source = 'plan'
		   AND ca.week_start >= $1
		   AND s.plan IN ('pro', 'team')
		 GROUP BY ca.workspace_id, s.plan
		 HAVING AVG(CASE WHEN ca.credits_total > 0 THEN (ca.credits_used::float / ca.credits_total) * 100 ELSE 0 END) < 10`, windowStart)
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
			Detail:        fmt.Sprintf("Average monthly credit usage %.0f%% over recent months", avgPct),
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
