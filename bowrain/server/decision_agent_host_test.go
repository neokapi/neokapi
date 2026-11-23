package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// The feed shows a decision's note alone and the agent host beside it, from
// an audit event this server writes (agent_host) and from one an older server
// wrote with the host inside the note.
func TestFeedDecision_TheNoteStandsAlone(t *testing.T) {
	for name, data := range map[string]string{
		"held apart": `{"decision":"rejected","reason":"Too formal.","agent_host":"claude-code"}`,
		"folded in":  `{"decision":"rejected","reason":"Too formal. (recorded as a person in a claude-code shell)"}`,
	} {
		d := parseAuditDecision(event.AuditEntry{Data: data})
		assert.Equal(t, "Too formal.", d.note, name)
		assert.Equal(t, "claude-code", d.agentHost, name)
	}
}

// The review sheet's provenance shows the note alone and the agent host as
// its own field, whichever form the ledger row holds.
func TestReviewProvenance_TheNoteStandsAlone(t *testing.T) {
	sb := &venue.StoredBlock{Block: model.NewBlock("b", "Hello")}
	for name, d := range map[string]venue.UnitDecision{
		"held apart": {ReviewState: venue.ReviewStateRejected, Note: "Too formal.", AgentHost: "claude-code"},
		"folded in":  {ReviewState: venue.ReviewStateRejected, Note: "Too formal. (recorded as a person in a claude-code shell)"},
	} {
		p := reviewProvenanceOf(&d, sb, "fr")
		assert.Equal(t, "Too formal.", p.Note, name)
		assert.Equal(t, "claude-code", p.AgentHost, name)
	}
}
