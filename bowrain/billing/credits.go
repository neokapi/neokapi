package billing

import (
	"context"
	"time"
)

// Credit allocation sources. Credits are the single user-facing ledger (Epic
// 004). A workspace draws from two buckets:
//
//   - SourcePlan: the weekly plan allowance (50K/500K/2M per WeeklyCredits).
//     Scoped to the current week; unspent plan credits expire at week rollover.
//   - SourcePurchased: one-time top-up packs bought with Stripe. Non-expiring —
//     they persist across weekly rollovers until spent (see the purchased-week
//     sentinel below).
//
// Spend cascades plan-first, then purchased (PgBillingStore.DeductCredits), and
// the spendable balance sums both (PgBillingStore.CheckCredits).
const (
	SourcePlan      = "plan"
	SourcePurchased = "purchased"
)

// purchasedWeekStart / purchasedWeekEnd are the sentinel week bounds stored on
// purchased credit allocations. Purchased packs are not tied to any calendar
// week; storing them under a fixed sentinel collapses every purchase for a
// workspace into a single accumulating row via the
// UNIQUE(workspace_id, week_start, source) constraint, and keeps them out of
// current-week ("this week's plan allowance") queries. They are identified and
// summed purely by source='purchased', so they never expire at week rollover.
var (
	purchasedWeekStart = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	purchasedWeekEnd   = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
)

// allocationWeek returns the (week_start, week_end) bounds to store a grant of
// the given source under: the current calendar week for plan credits, the
// non-expiring sentinel week for purchased packs.
func allocationWeek(source string, now time.Time) (weekStart, weekEnd time.Time) {
	if source == SourcePurchased {
		return purchasedWeekStart, purchasedWeekEnd
	}
	return WeekStart(now), WeekEnd(now)
}

// WeekStart returns Monday 00:00 UTC for the week containing t.
func WeekStart(t time.Time) time.Time {
	t = t.UTC()
	// time.Monday == 1, time.Sunday == 0
	weekday := t.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	daysBack := int(weekday) - int(time.Monday)
	monday := t.AddDate(0, 0, -daysBack)
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
}

// WeekEnd returns next Monday 00:00 UTC (the exclusive end of the week containing t).
func WeekEnd(t time.Time) time.Time {
	return WeekStart(t).AddDate(0, 0, 7)
}

// EnsureWeeklyAllocation creates a credit allocation for the current week
// if one does not already exist. It returns the current allocation.
func EnsureWeeklyAllocation(ctx context.Context, store BillingStore, workspaceID string, plan Plan) (*CreditAllocation, error) {
	alloc, err := store.GetCurrentAllocation(ctx, workspaceID)
	if err == nil && alloc != nil {
		return alloc, nil
	}

	credits := CreditsForPlan(plan)
	if credits < 0 {
		// Unlimited plans get a very large allocation for tracking purposes.
		credits = int64(1<<62 - 1)
	}

	if err := store.GrantCredits(ctx, workspaceID, credits, "plan"); err != nil {
		return nil, err
	}

	return store.GetCurrentAllocation(ctx, workspaceID)
}

// ContainerTimeCredits converts container time duration to credits.
// Container time costs 10 credits per second.
func ContainerTimeCredits(d time.Duration) int64 {
	seconds := max(int64(d.Seconds()), 1)
	return seconds * 10
}

// TokensToCredits converts AI token count to credits.
// 1 token = 1 credit.
func TokensToCredits(tokens int) int64 {
	return int64(tokens)
}
