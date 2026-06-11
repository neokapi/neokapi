package billing

import (
	"context"
	"time"
)

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
