package billing

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Credit allocation sources. Credits are the single user-facing ledger (Epic
// 004). A workspace draws from three buckets:
//
//   - SourcePlan: the monthly plan allowance for PAID plans (see
//     MonthlyCredits). Scoped to the current calendar month; unspent plan
//     credits expire at month rollover. Free has no recurring allowance.
//   - SourceTrial: the one-time trial grant every workspace receives at
//     creation (FreeTrialGrantCredits). Non-expiring, granted exactly once per
//     workspace, ever — it is what bounds free-tier cost exposure now that the
//     Free plan has no recurring allowance.
//   - SourcePurchased: one-time top-up packs bought with Stripe. Non-expiring —
//     they persist across monthly rollovers until spent (see the non-expiring
//     sentinel below).
//
// Spend cascades plan → trial → purchased (PgBillingStore.DeductCredits): the
// expiring bucket is drained first (it evaporates at month end anyway), then
// the promotional trial grant, and paid-for pack credits last — a customer's
// paid balance is preserved the longest. The spendable balance sums all three
// (PgBillingStore.CheckCredits).
const (
	SourcePlan      = "plan"
	SourceTrial     = "trial"
	SourcePurchased = "purchased"
)

// nonExpiringPeriodStart / nonExpiringPeriodEnd are the sentinel period bounds
// stored on non-expiring credit allocations (trial grants and purchased packs).
// Neither is tied to any calendar period; storing them under a fixed sentinel
// collapses every grant of a source into a single row per workspace via the
// UNIQUE(workspace_id, week_start, source) constraint — one accumulating row
// for purchased packs, one grant-once row for the trial — and keeps them out of
// current-month ("this month's plan allowance") queries. They are identified
// and summed purely by source, so they never expire at month rollover.
var (
	nonExpiringPeriodStart = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	nonExpiringPeriodEnd   = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
)

// allocationPeriod returns the (period start, period end) bounds to store a
// grant of the given source under: the current calendar month for plan credits,
// the non-expiring sentinel for trial grants and purchased packs.
//
// The columns holding these bounds are still named week_start/week_end — a
// historical name from the weekly-allowance era, kept because renaming them is
// not a zero-downtime change (see billing migration 6).
func allocationPeriod(source string, now time.Time) (periodStart, periodEnd time.Time) {
	if source == SourceTrial || source == SourcePurchased {
		return nonExpiringPeriodStart, nonExpiringPeriodEnd
	}
	return MonthStart(now), MonthEnd(now)
}

// MonthStart returns the first instant (day 1, 00:00 UTC) of the calendar month
// containing t.
func MonthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// MonthEnd returns the first instant of the next calendar month (the exclusive
// end of the month containing t) — which is also when a paid plan's allowance
// refreshes.
func MonthEnd(t time.Time) time.Time {
	return MonthStart(t).AddDate(0, 1, 0)
}

// EnsureMonthlyAllocation lazily creates the current month's plan allocation
// for a PAID plan if one does not already exist, and returns it. It is invoked
// on every workspace-scoped request (MonthlyAllocationMiddleware), so the
// fast path is a single read.
//
// Two deliberate skips (both return (nil, nil)):
//
//   - Plans with no recurring allowance (Free, MonthlyCredits == 0). The Free
//     tier's credits are the one-time trial grant (EnsureTrialGrant), not a
//     recurring allocation.
//   - Workspaces whose subscription is still `trialing`. The local card-free
//     trial grants Pro *features*, not the Pro monthly credit allowance —
//     otherwise every signup would mint a paid-size allocation and the
//     founder's cost exposure would not be bounded to trial grants. The paid
//     allowance starts with the first non-trialing month touch after
//     conversion.
//
// Transition note (weekly → monthly): rows from the weekly era are keyed on
// their old week bounds, so they simply age out — a paid workspace mid-week at
// deploy time gets its month allocation on next touch, and the month-keyed
// UNIQUE constraint (plus grant-once-per-period semantics in GrantCredits)
// prevents double-granting within the transition month.
func EnsureMonthlyAllocation(ctx context.Context, store BillingStore, workspaceID string, plan Plan) (*CreditAllocation, error) {
	alloc, err := store.GetCurrentAllocation(ctx, workspaceID)
	if err == nil && alloc != nil {
		return alloc, nil
	}

	credits := CreditsForPlan(plan)
	if credits == 0 {
		return nil, nil
	}
	if credits < 0 {
		// Unlimited plans get a very large allocation for tracking purposes.
		credits = int64(1<<62 - 1)
	}

	if sub, err := store.GetSubscription(ctx, workspaceID); err == nil && sub != nil && sub.Status == "trialing" {
		return nil, nil
	}

	if err := store.GrantCredits(ctx, workspaceID, credits, SourcePlan); err != nil {
		return nil, err
	}

	return store.GetCurrentAllocation(ctx, workspaceID)
}

// EnsureTrialGrant grants the workspace its one-time trial credits if it has
// never received them. It is called at workspace creation (onboarding and
// explicit workspace creation) and — as a lazy backfill for workspaces created
// before the trial-grant model existed — from the allocation middleware for
// plans with no recurring allowance. Idempotent: at most one trial grant per
// workspace, ever (enforced by the store's unique key). Nil-store safe; errors
// are logged, not returned, because a missed grant self-heals on the next
// touch and must never fail workspace creation.
func EnsureTrialGrant(ctx context.Context, store BillingStore, workspaceID string) {
	if store == nil || workspaceID == "" {
		return
	}
	if _, err := store.GrantTrialCredits(ctx, workspaceID, FreeTrialGrantCredits); err != nil {
		slog.Info("billing: failed to ensure trial credit grant", "id", workspaceID, "error", err)
	}
}

// errNoAllocation preserves the historical "no allocation" signal CheckCredits
// returns for a workspace with no credit rows at all, so callers (the enqueue
// pre-check, QuotaGuard) degrade to allowing the request on deployments where
// billing rows are not being provisioned.
var errNoAllocation = errors.New("check credits: no credit allocations for workspace")

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
