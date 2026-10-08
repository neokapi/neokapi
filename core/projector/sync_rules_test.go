package projector_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/workspace"
)

// TestWidenedRulesTravelWithTheProject: a rule a person widened to the
// workspace is recorded under the project that decided it, so a push carries it
// and every machine that pulls the project holds it in its own workspace, both
// when the pull replays the segments and when it starts from a checkpoint. A
// narrowing travels the same way.
func TestWidenedRulesTravelWithTheProject(t *testing.T) {
	tests := []struct {
		name            string
		checkpointEvery int
		wantCheckpoint  bool
	}{
		{name: "replayed from the segments", checkpointEvery: -1},
		{name: "started from a checkpoint", checkpointEvery: 1, wantCheckpoint: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			remote := workspace.NewFileRemote(t.TempDir())
			opts := workspace.SyncOptions{LocalKinds: projector.LocalKinds, CheckpointEvery: tt.checkpointEvery}

			from, fromWS, _ := open(t)
			rules := from.Rules()
			require.NoError(t, rules.WidenRule(ctx, workspace.Rule{ID: "prj_docs\x00a", Kind: "term", Origin: key, Payload: []byte(`{"a":1}`)}))
			require.NoError(t, rules.WidenRule(ctx, workspace.Rule{ID: "prj_docs\x00b", Kind: "term", Origin: key, Payload: []byte(`{"b":1}`)}))
			require.NoError(t, rules.NarrowRule(ctx, "prj_docs\x00b"))
			pushed, err := fromWS.NewSync(remote, key, from.Syncer(), opts).Push(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCheckpoint, pushed.Checkpoint != "")

			to, toWS, _ := open(t)
			pulled, err := toWS.NewSync(remote, key, to.Syncer(), opts).Pull(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCheckpoint, pulled.Checkpoint != "")
			assert.Equal(t, []string{"prj_docs\x00a"}, ruleIDs(t, toWS), "the widened rule arrived and the narrowed one did not")

			// Narrowing on the first machine takes the rule out on the second.
			require.NoError(t, rules.NarrowRule(ctx, "prj_docs\x00a"))
			_, err = fromWS.NewSync(remote, key, from.Syncer(), opts).Push(ctx)
			require.NoError(t, err)
			_, err = toWS.NewSync(remote, key, to.Syncer(), opts).Pull(ctx)
			require.NoError(t, err)
			assert.Empty(t, ruleIDs(t, toWS))
		})
	}
}

// ruleIDs lists the ids of the rules a workspace holds widened.
func ruleIDs(t *testing.T, ws *workspace.Workspace) []string {
	t.Helper()
	held, err := ws.WidenedRules(t.Context(), "")
	require.NoError(t, err)
	out := []string{}
	for _, r := range held {
		out = append(out, r.ID)
	}
	return out
}
