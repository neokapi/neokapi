package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Codex's event stream reports no model, so an attempt whose identity rested on
// the stream alone would always read as unavailable. The rollout file the host
// keeps beside the session carries the turn's model.
func TestEvalRecoverIdentityFromTheCodexRollout(t *testing.T) {
	state := t.TempDir()
	session := "01a0c579-ff2a-79a3-8afe-9f4ee2bdb70f"
	dir := filepath.Join(state, "codex", "sessions", "2026", "09", "21")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	rollout := `{"type":"turn_context","payload":{"model":"gpt-5.6-terra"}}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rollout-"+session+".jsonl"), []byte(rollout), 0o600))

	transcript := EvalTranscript{Host: "codex", SessionID: session}
	evalRecoverIdentity(&transcript, EvalPrepared{Paths: EvalPaths{State: state}})
	assert.Equal(t, "gpt-5.6-terra", transcript.ActualModel)
}

func TestEvalRecoverIdentityLeavesAReportedModelAlone(t *testing.T) {
	transcript := EvalTranscript{Host: "claude", ActualModel: "claude-sonnet-5", SessionID: "s-1"}
	evalRecoverIdentity(&transcript, EvalPrepared{Paths: EvalPaths{State: t.TempDir()}})
	assert.Equal(t, "claude-sonnet-5", transcript.ActualModel)

	unknown := EvalTranscript{Host: "codex", SessionID: "missing"}
	evalRecoverIdentity(&unknown, EvalPrepared{Paths: EvalPaths{State: t.TempDir()}})
	assert.Empty(t, unknown.ActualModel, "an identity with no record stays unknown rather than guessed")
}

// A command line reading its own help writes nothing, so it must not stand in
// for the session's first change to a file.
func TestEvalHelpIsNeitherAWriteNorAnAsk(t *testing.T) {
	transcript := EvalTranscript{Calls: []EvalCall{}}
	transcript.addShellCall("kapi apply --help | sed -n '1,240p'")
	transcript.addShellCall("kapi context docs/billing.md")
	transcript.addShellCall("kapi apply --from edits.jsonl")

	require.Len(t, transcript.Calls, 3)
	assert.Equal(t, evalKindOther, transcript.Calls[0].Kind)
	assert.Equal(t, evalKindAsk, transcript.Calls[1].Kind)
	assert.Equal(t, evalKindWrite, transcript.Calls[2].Kind)
	assert.True(t, evalAskedFirst(transcript.Calls))
}
