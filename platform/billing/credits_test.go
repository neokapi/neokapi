package billing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWeekStart(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			"monday returns same monday",
			time.Date(2026, 3, 16, 10, 30, 0, 0, time.UTC), // Monday
			time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			"tuesday returns monday",
			time.Date(2026, 3, 17, 15, 0, 0, 0, time.UTC), // Tuesday
			time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			"wednesday returns monday",
			time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC), // Wednesday
			time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			"friday returns monday",
			time.Date(2026, 3, 20, 23, 59, 59, 0, time.UTC), // Friday
			time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			"saturday returns monday",
			time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC), // Saturday
			time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			"sunday returns monday",
			time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC), // Sunday
			time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			"non-UTC timezone is converted",
			time.Date(2026, 3, 17, 3, 0, 0, 0, time.FixedZone("EST", -5*3600)), // Tuesday EST = still Tuesday UTC
			time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			"monday midnight exactly",
			time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			"week crossing month boundary",
			time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC), // Thursday April 2
			time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC), // Monday March 30
		},
		{
			"week crossing year boundary",
			time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),  // Thursday Jan 1
			time.Date(2025, 12, 29, 0, 0, 0, 0, time.UTC), // Monday Dec 29
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WeekStart(tt.in)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, time.Monday, got.Weekday())
		})
	}
}

func TestWeekEnd(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			"monday returns next monday",
			time.Date(2026, 3, 16, 10, 30, 0, 0, time.UTC),
			time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC),
		},
		{
			"sunday returns next monday",
			time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC),
		},
		{
			"friday returns next monday",
			time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WeekEnd(tt.in)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, time.Monday, got.Weekday())
		})
	}
}

func TestWeekStartEndConsistency(t *testing.T) {
	// Week end should always be exactly 7 days after week start.
	dates := []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC),
	}

	for _, d := range dates {
		ws := WeekStart(d)
		we := WeekEnd(d)
		diff := we.Sub(ws)
		assert.Equal(t, 7*24*time.Hour, diff, "week end should be exactly 7 days after week start for %v", d)
	}
}

func TestContainerTimeCredits(t *testing.T) {
	assert.Equal(t, int64(10), ContainerTimeCredits(1*time.Second))
	assert.Equal(t, int64(600), ContainerTimeCredits(60*time.Second))
	assert.Equal(t, int64(10), ContainerTimeCredits(500*time.Millisecond)) // rounds up to 1s minimum
}

func TestTokensToCredits(t *testing.T) {
	assert.Equal(t, int64(100), TokensToCredits(100))
	assert.Equal(t, int64(0), TokensToCredits(0))
	assert.Equal(t, int64(50000), TokensToCredits(50000))
}
