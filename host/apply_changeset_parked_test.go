package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

// A parked locale has its drafts in the project store and no file on disk.
// kapi apply establishes such a draft with a decide operation on the source
// document and the locale's edition, and the decision counts where the gate
// reads it, as the review queue's own approval does.
func TestApply_DecideEstablishesAParkedDraft(t *testing.T) {
	a, cmd, recipe, dir := parkedReviewProject(t)
	out, _ := parkedReviewPass(t, a, cmd, recipe)
	require.False(t, parkedLocaleResult(t, out, "nl").Shippable, "nl parks on the first pass")
	keys := parkedQueueKeys(t, a, recipe, "nl")
	require.Len(t, keys, 4)

	applyCmd := NewEnvCommand(t.Context(), "apply")
	AddProjectFlag(applyCmd)
	require.NoError(t, applyCmd.Flags().Set("project", recipe))
	var ops []map[string]any
	for _, key := range keys[:2] {
		ops = append(ops, map[string]any{"op": "decide", "outcome": "establish", "if_match": "*",
			"at": map[string]any{"doc": "src/en.json", "block": key, "edition": "nl"}})
	}
	res, err := applyJSON(t, a, applyCmd, changeSetOf(t, ops...), ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	for _, op := range res.Ops {
		assert.Equal(t, change.OpApplied, op.Status, "%+v", op.Error)
	}
	assert.Equal(t, 50, parkedCoverage(t, a, cmd, recipe, dir, "nl").Pct["established"],
		"the decisions count where the gate reads them")
	assert.Len(t, parkedQueueKeys(t, a, recipe, "nl"), 2, "the decided drafts leave the queue")
}
