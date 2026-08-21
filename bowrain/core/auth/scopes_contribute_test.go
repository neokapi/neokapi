package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The contribute scope is the machine's share of the loop: it can move content
// and propose, and it can never decide a review.
func TestContributeScopeProposesButNeverDecides(t *testing.T) {
	rs, err := ParseScope("contribute")
	require.NoError(t, err)

	grants := []Permission{PermViewContent, PermTranslate, PermManageFiles, PermManageMemory, PermManageTerms, PermRunFlows}
	for _, p := range grants {
		assert.NotZero(t, rs.Permissions&p, "contribute must grant %v", p)
	}
	denies := []Permission{PermReview, PermManageVoice, PermManageProject, PermManageMembers, PermManageConnectors, PermManageAutomation, PermRollbackChanges}
	for _, p := range denies {
		assert.Zero(t, rs.Permissions&p, "contribute must not grant %v", p)
	}
}
