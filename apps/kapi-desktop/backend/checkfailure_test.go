package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/stretchr/testify/assert"
)

// blockCheckFindings used to discard host.RunCheckTool's error, which made every
// one of its callers fail UNSAFE: the review pane showed a clean unit, HasFindings
// read false, and — worst — review_ai's AutoApprove consults hasBlockingCheckFinding
// on that very slice, so a checker whose plugin was down would auto-approve a
// translation whose placeholder integrity was never verified. The failure is now a
// `major` synthetic finding, so the gate treats "not checked" as blocking.
func TestHasBlockingCheckFinding_TreatsAnIncompleteCheckAsBlocking(t *testing.T) {
	incomplete := []DesktopFinding{{
		Category: "check",
		Severity: string(check.SeverityMajor),
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
		Severity: string(check.SeverityMinor),
		Message:  "cosmetic",
	}}))
}
