package venue

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A decision a released client wrote, with the agent host folded into its
// note, and the same decision held as note and host are one record to both
// ends: same identity, same component. So a store that reads the host out
// on the way in still agrees with that client, and no push resends it.
func TestDecisionIdentity_AHostHeldApartIsTheSameRecord(t *testing.T) {
	legacy := UnitDecision{Unit: "u", Variant: "fr", ReviewState: ReviewStateRejected, DecidedAt: "2026-11-23T08:00:00Z",
		Note: "Too formal. (recorded as a person in a claude-code shell)"}
	split := SplitNote(legacy)
	assert.Equal(t, "Too formal.", split.Note)
	assert.Equal(t, "claude-code", split.AgentHost)
	assert.True(t, SameDecision(legacy, split))
	assert.Equal(t, DecisionIdentity(legacy), DecisionIdentity(split))
	assert.Equal(t, DecisionsComponent([]UnitDecision{legacy}), DecisionsComponent([]UnitDecision{split}))

	other := split
	other.AgentHost = "codex"
	assert.False(t, SameDecision(split, other), "another host is another record")
	plain := split
	plain.AgentHost = ""
	assert.False(t, SameDecision(split, plain), "a person's own note without a host is another record")
}

// The host travels the wire as its own field; absent means the sender did not
// say, and a store keeps the host it holds for the same decision.
func TestDecisionHost_OnTheWire(t *testing.T) {
	d := UnitDecision{Unit: "u", Variant: "fr", ReviewState: ReviewStateRejected, DecidedAt: "2026-11-23T08:00:00Z", Note: "Too formal.", AgentHost: "claude-code"}
	raw, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"agent_host":"claude-code"`)
	assert.Contains(t, string(raw), `"note":"Too formal."`)
	var back UnitDecision
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, d, back)

	none, err := json.Marshal(UnitDecision{Unit: "u"})
	require.NoError(t, err)
	assert.NotContains(t, string(none), `"agent_host"`)

	echo := d
	echo.AgentHost = ""
	assert.Equal(t, "claude-code", CarryHost(d, echo).AgentHost, "an older client's echo keeps the host")
	later := echo
	later.DecidedAt = "2026-11-23T09:00:00Z"
	assert.Empty(t, CarryHost(d, later).AgentHost, "a later decision does not inherit it")
}
