package host

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTermsLineReportsAwaitingReview holds the terms line to the three states
// terminology can actually be in. The middle one is the one it used to lack: a
// governed edit that has been pushed lives in a change-set until a reviewer
// merges it, and no pull can bring it down as a baseline in the meantime — so
// the line reported "never synced" about terminology that had been sent,
// received, and queued.
func TestTermsLineReportsAwaitingReview(t *testing.T) {
	tests := []struct {
		name     string
		terms    *StatusTerminology
		contains []string
		absent   []string
	}{
		{
			name:     "nothing known reads as never synced",
			terms:    nil,
			contains: []string{"never synced"},
		},
		{
			name:     "a snapshot reads as synced",
			terms:    &StatusTerminology{PulledAt: "2026-08-01T09:00:00Z", Concepts: 12, Relations: 3},
			contains: []string{"synced 2026-08-01T09:00:00Z", "12 concepts", "3 relations"},
			absent:   []string{"awaiting review"},
		},
		{
			name: "proposed but never snapshotted is not never synced",
			terms: &StatusTerminology{
				PendingChangesets: 1,
				PendingChangeset:  "cs-1",
				PendingURL:        "https://bowrain.cloud/acme/changesets/cs-1",
			},
			contains: []string{"1 change-set proposed, awaiting review", "review: https://bowrain.cloud/acme/changesets/cs-1"},
			absent:   []string{"never synced"},
		},
		{
			name: "a snapshot and pending proposals read together",
			terms: &StatusTerminology{
				PulledAt: "2026-08-01T09:00:00Z", Concepts: 12, Relations: 3,
				PendingChangesets: 2, PendingChangeset: "cs-2",
			},
			contains: []string{"synced 2026-08-01T09:00:00Z", "2 change-sets proposed, awaiting review"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeTermsLine(&buf, tt.terms)
			for _, want := range tt.contains {
				assert.Contains(t, buf.String(), want)
			}
			for _, notWant := range tt.absent {
				assert.NotContains(t, buf.String(), notWant)
			}
		})
	}
}
