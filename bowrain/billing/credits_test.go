package billing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMonthStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			"mid-month returns first of month",
			time.Date(2026, 3, 16, 10, 30, 0, 0, time.UTC),
			time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"first instant of month returns itself",
			time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"last instant of month stays in month",
			time.Date(2026, 3, 31, 23, 59, 59, 999_999_999, time.UTC),
			time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"february",
			time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC),
			time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"leap-year february 29",
			time.Date(2028, 2, 29, 12, 0, 0, 0, time.UTC),
			time.Date(2028, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"december",
			time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC),
			time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			// A zone west of UTC where the local date is still last month: the
			// UTC month is what keys the allocation.
			"non-UTC timezone is converted",
			time.Date(2026, 2, 28, 22, 0, 0, 0, time.FixedZone("EST", -5*3600)), // = Mar 1 03:00 UTC
			time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MonthStart(tt.in)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, 1, got.Day())
			assert.Equal(t, time.UTC, got.Location())
		})
	}
}

func TestMonthEnd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			"mid-month returns first of next month",
			time.Date(2026, 3, 16, 10, 30, 0, 0, time.UTC),
			time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"december rolls into next year",
			time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC),
			time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"january 31 does not skip february",
			time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC),
			time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"february (leap year) ends march 1",
			time.Date(2028, 2, 29, 12, 0, 0, 0, time.UTC),
			time.Date(2028, 3, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MonthEnd(tt.in)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, 1, got.Day())
		})
	}
}

func TestMonthStartEndConsistency(t *testing.T) {
	t.Parallel()

	// Month end must be exactly the next month's start: contiguous periods
	// with no gap or overlap, across month lengths and year boundaries.
	dates := []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC),
		time.Date(2028, 2, 29, 6, 0, 0, 0, time.UTC),
	}

	for _, d := range dates {
		ms := MonthStart(d)
		me := MonthEnd(d)
		assert.Equal(t, MonthStart(me), me, "month end must itself be a month start for %v", d)
		assert.Equal(t, ms.AddDate(0, 1, 0), me, "month end is exactly one month after month start for %v", d)
		assert.True(t, !d.UTC().Before(ms) && d.UTC().Before(me), "t must fall inside [MonthStart, MonthEnd) for %v", d)
	}
}

// allocationPeriod routes plan grants to the current month and the
// non-expiring sources to the sentinel period — the keying that makes trial
// grants and purchased packs survive month rollovers.
func TestAllocationPeriod(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)

	ps, pe := allocationPeriod(SourcePlan, now)
	assert.Equal(t, MonthStart(now), ps)
	assert.Equal(t, MonthEnd(now), pe)

	for _, src := range []string{SourceTrial, SourcePurchased} {
		ps, pe := allocationPeriod(src, now)
		assert.Equal(t, nonExpiringPeriodStart, ps, "%s is non-expiring", src)
		assert.Equal(t, nonExpiringPeriodEnd, pe, "%s is non-expiring", src)
	}
}

func TestContainerTimeCredits(t *testing.T) {
	t.Parallel()
	assert.Equal(t, int64(10), ContainerTimeCredits(1*time.Second))
	assert.Equal(t, int64(600), ContainerTimeCredits(60*time.Second))
	assert.Equal(t, int64(10), ContainerTimeCredits(500*time.Millisecond)) // rounds up to 1s minimum
}

func TestTokensToCredits(t *testing.T) {
	t.Parallel()
	assert.Equal(t, int64(100), TokensToCredits(100))
	assert.Equal(t, int64(0), TokensToCredits(0))
	assert.Equal(t, int64(50000), TokensToCredits(50000))
}
