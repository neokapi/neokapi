package host

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/contextop"
)

// kapi apply stamps the actor the environment names, and a change set has no
// field that could name another one. A person's term is recorded as the
// person's, with a note saying kapi apply applied it; an agent's shell gets the
// refusal apply_edits gives, and nothing is written; a person typing in an
// agent host's shell says so, and the note keeps that.
func TestApply_StampsTheEnvironmentsActor(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		applied  bool
		wantNote string
	}{
		{name: "a person", applied: true, wantNote: "applied with `kapi apply`"},
		{name: "an agent's shell", env: map[string]string{EnvActor: "agent", EnvAgentName: "claude", EnvAgentSession: "s9"}},
		{
			name:     "a person in an agent host's shell",
			env:      map[string]string{EnvActor: "person", "CLAUDECODE": "1"},
			applied:  true,
			wantNote: "applied with `kapi apply` (recorded as a person in a claude-code shell)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			app, _ := contextOpsApp(t)
			root := contextOpsProject(t, "cli-apply-actor")
			cmd := executionCommand(t)
			cmd.Flags().String(projectFlagName, recipeOf(root), "")
			body := changeSetOf(t, map[string]any{"op": "term", "action": "upsert", "term": "leverage", "replacement": "use", "locale": "en", "status": "forbidden"})
			res, err := applyJSON(t, app, cmd, body, ApplyOptions{})
			log, lerr := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root)})
			require.NoError(t, lerr)

			if !tt.applied {
				assert.Equal(t, ExitGate, ExitCode(cmd, err))
				assert.Equal(t, change.SetRefused, res.Status)
				require.NotNil(t, res.Ops[0].Error)
				assert.Equal(t, change.CodeNotPermitted, res.Ops[0].Error.Code)
				assert.Contains(t, res.Ops[0].Error.Message, "agent claude/s9 may not edit")
				assert.NotContains(t, projectTerms(t, app, root), "leverage")
				assert.Empty(t, log.Operations)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, change.OpApplied, res.Ops[0].Status, "%+v", res.Ops[0].Error)
			require.Len(t, log.Operations, 1)
			assert.Equal(t, contextop.ActorPerson, log.Operations[0].Actor.Kind)
			assert.Equal(t, tt.wantNote, log.Operations[0].Note)
		})
	}
}

// --dry-run writes nothing, and it shows each asset operation as the write
// would treat it: a person's as previewed, an agent's with the refusal the
// write gives, so the preview exits as the write would.
func TestApply_DryRunPreviewsTheActorsRefusal(t *testing.T) {
	ops := map[string]struct {
		op     map[string]any
		refuse string
	}{
		"term":   {map[string]any{"op": "term", "action": "upsert", "term": "leverage", "replacement": "use", "locale": "en", "status": "forbidden"}, "agent claude/s9 may not edit"},
		"memory": {map[string]any{"op": "memory", "action": "add", "from": map[string]any{"edition": "en", "text": "Save"}, "to": map[string]any{"edition": "nb", "text": "Lagre"}}, "agent claude/s9 may not edit"},
		"recipe": {map[string]any{"op": "recipe", "path": "defaults.coordinates.brand", "value": "kapi"}, "ask a person to change kapi.yaml"},
	}
	for kind, tc := range ops {
		for _, agent := range []bool{false, true} {
			name := kind + "/a person"
			if agent {
				name = kind + "/an agent's shell"
			}
			t.Run(name, func(t *testing.T) {
				if agent {
					t.Setenv(EnvActor, "agent")
					t.Setenv(EnvAgentName, "claude")
					t.Setenv(EnvAgentSession, "s9")
				}
				app, _ := contextOpsApp(t)
				root := contextOpsProject(t, "cli-apply-preview")
				recipeBefore, err := os.ReadFile(recipeOf(root))
				require.NoError(t, err)
				cmd := executionCommand(t)
				cmd.Flags().String(projectFlagName, recipeOf(root), "")
				res, err := applyJSON(t, app, cmd, changeSetOf(t, tc.op), ApplyOptions{DryRun: true})
				if !agent {
					require.NoError(t, err)
					assert.Equal(t, change.SetPreviewed, res.Status)
				} else {
					assert.Equal(t, ExitGate, ExitCode(cmd, err))
					require.NotNil(t, res.Ops[0].Error)
					assert.Equal(t, change.CodeNotPermitted, res.Ops[0].Error.Code)
					assert.Contains(t, res.Ops[0].Error.Message, tc.refuse)
				}
				recipeAfter, err := os.ReadFile(recipeOf(root))
				require.NoError(t, err)
				assert.Equal(t, string(recipeBefore), string(recipeAfter))
				assert.NotContains(t, projectTerms(t, app, root), "leverage")
			})
		}
	}
}
