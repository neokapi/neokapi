package host

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
)

// The session note is what one process tells the others: an agent is at work in
// this project right now. Nothing on the agent's own surface reads it back, so
// it is asserted here, against the workspace the server writes it to.

func TestNoteMCPSessionSaysAnAgentIsWorkingHere(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "session-note")
	recipe := filepath.Join(root, "kapi.yaml")

	ctx := t.Context()
	app.NoteMCPSession(ctx, recipe, "claude-code")

	ws, err := app.Workspace(ctx)
	require.NoError(t, err)
	held, err := ws.AgentSessions(ctx, "", 0)
	require.NoError(t, err)
	require.Len(t, held, 1, "one server process is one session note")
	assert.Equal(t, MCPSessionID(), held[0].ID, "the note carries the session the operations carry")
	assert.Equal(t, "claude-code", held[0].Agent, "the client's own name, as it introduced itself")
	assert.NotEmpty(t, held[0].Project, "a surface showing who is working shows where")
	assert.False(t, held[0].Started.IsZero())

	// Noting again moves last-seen forward and keeps the moment the session
	// opened, which is what lets a surface say how long it has been working.
	first := held[0]
	app.NoteMCPSession(ctx, recipe, "claude-code")
	held, err = ws.AgentSessions(ctx, "", 0)
	require.NoError(t, err)
	require.Len(t, held, 1, "a second note is the same session, not a second one")
	assert.Equal(t, first.Started, held[0].Started)
	assert.False(t, held[0].LastSeen.Before(first.LastSeen))

	// A window is how a reader tells a session at work from one that stopped.
	active, err := ws.AgentSessions(ctx, held[0].Project, time.Hour)
	require.NoError(t, err)
	assert.Len(t, active, 1)
}

// A workspace that will not open, or a recipe that carries no identity, leaves
// the note unwritten rather than failing the call it rode on. An agent mid-task
// can do nothing about either, and a tool that refused to record an observation
// over a heartbeat would be the surface getting in the way of the work.
func TestNoteMCPSessionNeverFailsTheCall(t *testing.T) {
	app, _ := contextOpsApp(t)
	assert.NotPanics(t, func() {
		app.NoteMCPSession(t.Context(), "", "")
		app.NoteMCPSession(t.Context(), filepath.Join(t.TempDir(), "kapi.yaml"), "cursor")
	})
}

// TestMCPAgentActorIsAlwaysAnAgent: the caller never says who it is. A client
// that could record as a person would be claiming the rights the policy
// reserves for one, and the tools take no actor argument at all.
func TestMCPAgentActorIsAlwaysAnAgent(t *testing.T) {
	actor := mcpAgentActor(nil)
	assert.Equal(t, contextop.ActorAgent, actor.Kind)
	assert.Equal(t, MCPSessionID(), actor.Session, "one session per server process")
	assert.Equal(t, MCPSessionID(), mcpAgentActor(nil).Session, "minted once, not per call")

	// And the policy in force refuses what that actor may not do, whichever
	// surface asks.
	for kind, refused := range map[contextop.Kind]bool{
		contextop.KindObserve: false,
		contextop.KindPropose: false,
		contextop.KindCorrect: false,
		contextop.KindConfirm: true,
		contextop.KindWiden:   true,
	} {
		err := contextop.PersonDecides(contextop.Transition{Actor: actor, Kind: kind, Targeted: true})
		if refused {
			require.ErrorIsf(t, err, contextop.ErrRefused, "an agent may not %s", kind)
			continue
		}
		require.NoErrorf(t, err, "an agent may %s", kind)
	}
}

func TestMCPEvidenceAndItsRequirement(t *testing.T) {
	assert.Nil(t, mcpEvidence("  ", "", " "), "a location nobody named is no evidence")
	assert.Equal(t,
		[]contextop.Evidence{{Path: "docs/guide.md", Unit: "u1", Quote: "Utilise the editor"}},
		mcpEvidence(" docs/guide.md ", "u1", "Utilise the editor "))

	require.Error(t, requireEvidence("context_propose", nil))
	assert.Contains(t, requireEvidence("context_propose", nil).Error(), "evidence")
	assert.NoError(t, requireEvidence("context_propose", mcpEvidence("docs/guide.md", "", "")))
}

// The sentence an agent ends its report with has to be usable as it stands: it
// names what was recorded, what is waiting for a decision, and the command that
// reviews it.
func TestSessionReportIsWhatAnAgentSays(t *testing.T) {
	out := sessionOutput(contextop.SessionSummary{
		Session:    "s4f1c2",
		Project:    workspace.ProjectKey("prj_docs"),
		Operations: 3,
		ByKind:     map[contextop.Kind]int{contextop.KindObserve: 2, contextop.KindPropose: 1},
		ByStatus:   map[contextop.Status]int{contextop.StatusCandidate: 3},
	})
	assert.Equal(t, 2, out.Observed)
	assert.Equal(t, 1, out.Proposed)
	assert.Equal(t, 3, out.Candidates)
	assert.Equal(t, "kapi context log --session s4f1c2", out.Review)
	assert.Contains(t, out.Report, "2 facts observed")
	assert.Contains(t, out.Report, "1 rule proposed")
	assert.Contains(t, out.Report, "3 candidates are waiting for a decision")
	assert.Contains(t, out.Report, out.Review)

	quiet := sessionOutput(contextop.SessionSummary{Session: "s0"})
	assert.Contains(t, quiet.Report, "Recorded nothing",
		"a session that recorded nothing says so, which is a fact a person can act on")
}
