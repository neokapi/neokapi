package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Checker errors must produce a blocking finding. Dropping host.RunCheckTool's
// error would show an unchecked unit as clean and allow review_ai.AutoApprove
// to approve it when a plugin is unavailable. These tests verify that failed
// checks block approval.
func TestHasBlockingCheckFinding_TreatsAnIncompleteCheckAsBlocking(t *testing.T) {
	incomplete := []DesktopFinding{{
		Category: "check",
		Fails:    true,
		Message:  "checks did not complete: placeholder: check plugin: connection refused",
	}}
	assert.True(t, hasBlockingCheckFinding(incomplete),
		"a checker that could not run must block auto-approval, not wave the unit through")
}

// TestHasBlockingCheckFinding_CleanUnitIsNotBlocked is the control: a unit with no
// findings still auto-approves, so the guard above rejects only real failures.
func TestHasBlockingCheckFinding_CleanUnitIsNotBlocked(t *testing.T) {
	assert.False(t, hasBlockingCheckFinding(nil))
	assert.False(t, hasBlockingCheckFinding([]DesktopFinding{{
		Category: "placeholder",
		Message:  "cosmetic",
	}}))
}
