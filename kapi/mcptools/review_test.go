package mcptools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeMCPReviewProject scaffolds a two-unit nb project with a pending review
// queue for the MCP review-tool handlers.
func writeMCPReviewProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "1")
	root := t.TempDir()
	recipe := `version: v1
name: rev
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - name: app
    content:
      - path: en.json
        target: "{lang}.json"
ship_gate: { translated: 100, reviewed: 100 }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	return root
}

func TestHandleReviewQueue(t *testing.T) {
	root := writeMCPReviewProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, out, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Total)
	require.Len(t, out.Pending, 2)
	assert.Equal(t, "nb", out.Pending[0].Locale)
	assert.Equal(t, "app", out.Pending[0].Collection)

	// Locale filter — no de units exist.
	_, out, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj, Locale: "de"})
	require.NoError(t, err)
	assert.Equal(t, 0, out.Total)

	// Collection filter.
	_, out, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj, Collection: "app"})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Total)
	_, out, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj, Collection: "other"})
	require.NoError(t, err)
	assert.Equal(t, 0, out.Total)
}

func TestHandleReviewQueue_NoProject(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	a := testApp()
	_, _, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no kapi project")
}

func TestHandleReviewUnit(t *testing.T) {
	root := writeMCPReviewProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, queue, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	item := queue.Pending[0]

	_, out, err := handleReviewUnit(t.Context(), a, ReviewUnitInput{
		Project: proj, Locale: item.Locale, File: item.File, Key: item.Key,
	})
	require.NoError(t, err)
	require.NotNil(t, out.Unit)
	assert.Equal(t, "translated", out.Unit.Status)
	assert.NotEmpty(t, out.Unit.Source)
	assert.NotEmpty(t, out.Unit.Target)

	_, _, err = handleReviewUnit(t.Context(), a, ReviewUnitInput{
		Project: proj, Locale: item.Locale, File: item.File, Key: "missing",
	})
	require.Error(t, err)
}

// TestHandleReviewUnit_CarriesTheContext holds the bar for the agent surface:
// an agent asked to judge a translation is handed at least what the model that
// produced it was handed.
func TestHandleReviewUnit_CarriesTheContext(t *testing.T) {
	root := writeMCPReviewProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, queue, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	require.Len(t, queue.Pending, 2)

	_, out, err := handleReviewUnit(t.Context(), a, ReviewUnitInput{
		Project: proj, Locale: "nb", File: "nb.json", Key: "a",
	})
	require.NoError(t, err)
	require.NotNil(t, out.Unit)
	require.NotNil(t, out.Unit.Context, "review_unit answers with the review model")

	rc := out.Unit.Context
	assert.Equal(t, "app", rc.Point.Collection)
	assert.Equal(t, "en.json", rc.Point.Path, "the point is the SOURCE file's coordinate")
	assert.Equal(t, "a", rc.Neighbourhood.Key)
	assert.Equal(t, host.DefaultReviewWindow, rc.Neighbourhood.Window)
	assert.Empty(t, rc.Neighbourhood.Before, "`a` is the first unit in the file")
	require.Len(t, rc.Neighbourhood.After, 1)
	assert.Equal(t, "b", rc.Neighbourhood.After[0].Key)
	require.Len(t, rc.Neighbourhood.After[0].Source, 1)
	require.NotNil(t, rc.Neighbourhood.After[0].Source[0].Text)
	assert.Equal(t, "Banana", rc.Neighbourhood.After[0].Source[0].Text.Text)
}

// TestHandleReviewDecision_ApproveRejectSignOff drives the three decision
// tools end to end: identities land in the state store as agent-class, the
// queue shrinks, and a redundant call reports changed=false.
func TestHandleReviewDecision_ApproveRejectSignOff(t *testing.T) {
	root := writeMCPReviewProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, queue, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	require.Len(t, queue.Pending, 2)
	first, second := queue.Pending[0], queue.Pending[1]

	// approve_unit with a client-derived identity.
	_, dec, err := handleReviewDecision(t.Context(), a, ReviewDecisionInput{
		Project: proj, Locale: first.Locale, File: first.File, Key: first.Key,
	}, cli.ReviewDecisionApproved, "agent/claude-code")
	require.NoError(t, err)
	assert.True(t, dec.Changed)
	assert.Equal(t, "agent/claude-code", dec.By)

	// Redundant approval is a no-op, not an error.
	_, dec, err = handleReviewDecision(t.Context(), a, ReviewDecisionInput{
		Project: proj, Locale: first.Locale, File: first.File, Key: first.Key,
	}, cli.ReviewDecisionApproved, "agent/claude-code")
	require.NoError(t, err)
	assert.False(t, dec.Changed)

	// reject_unit carries the note; sign_off_unit is the top rung.
	_, dec, err = handleReviewDecision(t.Context(), a, ReviewDecisionInput{
		Project: proj, Locale: second.Locale, File: second.File, Key: second.Key,
		Note: "wrong term",
	}, cli.ReviewDecisionRejected, "agent")
	require.NoError(t, err)
	assert.True(t, dec.Changed)

	// Both decided units left the queue.
	_, queue, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	assert.Equal(t, 0, queue.Total)

	// Identities and the note reach the committed record via the explicit
	// commit — decisions stage in the working store, and `kapi commit` is the
	// only door into the git-tracked shards under .kapi/state/.
	// A fresh App: the project store is owned per App, and this one exists only
	// to publish what the App under test staged into the same file.
	committer := &host.App{}
	defer committer.Shutdown()
	_, err = committer.CommitProjectState(t.Context(), root)
	require.NoError(t, err)
	units, err := state.ReadCommitted(project.LayoutAt(root).UnitStateDir())
	require.NoError(t, err)
	require.Len(t, units, 2)
	byUnit := map[string]state.UnitState{}
	for _, u := range units {
		byUnit[u.Unit] = u
	}
	assert.Equal(t, "agent/claude-code", byUnit[first.Key].Decision.By)
	assert.False(t, state.IsAIDecision(byUnit[first.Key].Decision.By),
		"agent decisions are human-class for gates")
	assert.Equal(t, "agent", byUnit[second.Key].Decision.By)
	assert.Equal(t, "wrong term", byUnit[second.Key].Decision.Note)
}

func TestAgentIdentity_Fallback(t *testing.T) {
	// No session (nil request) → the bare "agent" identity.
	assert.Equal(t, "agent", agentIdentity(nil))
}
