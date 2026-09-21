package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

// TestAgentHostTableIsTheVerifiedOne pins the table. A row says that a variable
// in somebody's shell means an agent is driving the command, and a row added
// from documentation rather than from the host itself attributes a person's
// work to an agent. Changing this test is how a new row gets added, which is
// the point of pinning it.
func TestAgentHostTableIsTheVerifiedOne(t *testing.T) {
	assert.Equal(t, []AgentHostMarker{
		{Name: "claude-code", Marker: "CLAUDECODE", Session: "CLAUDE_CODE_SESSION_ID"},
		{Name: "codex", Marker: "CODEX_SESSION_ID", Session: "CODEX_SESSION_ID"},
	}, AgentHostMarkers())
}

// TestEveryAgentHostRowIsDetected drives one subtest per row, so a row cannot
// join the table without the behaviour being exercised.
func TestEveryAgentHostRowIsDetected(t *testing.T) {
	for _, row := range AgentHostMarkers() {
		t.Run(row.Name, func(t *testing.T) {
			resolved, err := ResolveCommandActor(envOf(map[string]string{
				row.Marker:  "1",
				row.Session: "run-of-" + row.Name,
			}))
			require.NoError(t, err)
			assert.Equal(t, contextop.ActorAgent, resolved.Actor.Kind)
			assert.Equal(t, row.Name, resolved.Actor.Name)
			assert.Equal(t, "run-of-"+row.Name, resolved.Actor.Session)
			assert.Equal(t, row.Name, resolved.Host)
			assert.False(t, resolved.PersonOverride)
		})

		t.Run(row.Name+" with no session of its own", func(t *testing.T) {
			if row.Marker == row.Session {
				t.Skip("this host's marker is its session, so it always names one")
			}
			resolved, err := ResolveCommandActor(envOf(map[string]string{row.Marker: "1"}))
			require.NoError(t, err)
			assert.Equal(t, contextop.ActorAgent, resolved.Actor.Kind)
			assert.Equal(t, AgentRunSession(), resolved.Actor.Session,
				"a host that names no session falls back to the one derived from its process")
		})

		t.Run(row.Name+" cleared", func(t *testing.T) {
			resolved, err := ResolveCommandActor(envOf(map[string]string{row.Marker: "  "}))
			require.NoError(t, err)
			assert.Equal(t, contextop.ActorPerson, resolved.Actor.Kind,
				"an empty marker is how a harness clears one it inherited")
		})
	}
}

func TestResolveCommandActor(t *testing.T) {
	cases := []struct {
		name         string
		env          map[string]string
		wantKind     contextop.ActorKind
		wantName     string
		wantSession  string
		wantHost     string
		wantOverride bool
	}{
		{
			name:     "a plain shell is a person",
			env:      map[string]string{},
			wantKind: contextop.ActorPerson,
		},
		{
			name: "an unrecognised host names itself",
			env: map[string]string{
				EnvActor:        "agent",
				EnvAgentName:    "some-assistant",
				EnvAgentSession: "run-7",
			},
			wantKind:    contextop.ActorAgent,
			wantName:    "some-assistant",
			wantSession: "run-7",
		},
		{
			name:        "a declared agent that names nothing is still an agent",
			env:         map[string]string{EnvActor: "agent"},
			wantKind:    contextop.ActorAgent,
			wantSession: AgentRunSession(),
		},
		{
			name: "the caller's session wins over the host's",
			env: map[string]string{
				"CLAUDECODE":             "1",
				"CLAUDE_CODE_SESSION_ID": "the-host-id",
				EnvAgentSession:          "the-caller-id",
			},
			wantKind:    contextop.ActorAgent,
			wantName:    "claude-code",
			wantSession: "the-caller-id",
			wantHost:    "claude-code",
		},
		{
			name: "the caller's name wins over the host's",
			env: map[string]string{
				"CLAUDECODE":             "1",
				"CLAUDE_CODE_SESSION_ID": "the-host-id",
				EnvAgentName:             "a-subagent",
			},
			wantKind:    contextop.ActorAgent,
			wantName:    "a-subagent",
			wantSession: "the-host-id",
			wantHost:    "claude-code",
		},
		{
			name:     "a person says so where no host is detected",
			env:      map[string]string{EnvActor: "person"},
			wantKind: contextop.ActorPerson,
		},
		{
			name: "a person typing in an agent host's shell is on the record",
			env: map[string]string{
				"CLAUDECODE": "1",
				EnvActor:     "person",
			},
			wantKind:     contextop.ActorPerson,
			wantHost:     "claude-code",
			wantOverride: true,
		},
		{
			name:        "the value is read without regard to case or spacing",
			env:         map[string]string{EnvActor: "  Agent  ", EnvAgentSession: "run-9"},
			wantKind:    contextop.ActorAgent,
			wantSession: "run-9",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved, err := ResolveCommandActor(envOf(tc.env))
			require.NoError(t, err)
			assert.Equal(t, tc.wantKind, resolved.Actor.Kind)
			assert.Equal(t, tc.wantName, resolved.Actor.Name)
			assert.Equal(t, tc.wantSession, resolved.Actor.Session)
			assert.Equal(t, tc.wantHost, resolved.Host)
			assert.Equal(t, tc.wantOverride, resolved.PersonOverride)
		})
	}
}

// TestUnreadableActorKindIsRefused: a typo that silently recorded an agent's
// work as a person's would be the confusion the variable exists to settle.
func TestUnreadableActorKindIsRefused(t *testing.T) {
	for _, value := range []string{"tool", "robot", "yes"} {
		t.Run(value, func(t *testing.T) {
			_, err := ResolveCommandActor(envOf(map[string]string{EnvActor: value}))
			require.Error(t, err)
			assert.Contains(t, err.Error(), EnvActor)
			assert.Contains(t, err.Error(), value)
		})
	}
}

// TestAPersonOverrideRidesOnTheNote: the record says the operation claimed a
// person's rights under a host the environment named.
func TestAPersonOverrideRidesOnTheNote(t *testing.T) {
	under := ResolvedActor{Host: "claude-code", PersonOverride: true}
	assert.Equal(t, "recorded as a person in a claude-code shell", under.NoteWith(""))
	assert.Equal(t, "we say it both ways (recorded as a person in a claude-code shell)",
		under.NoteWith("we say it both ways"))

	plain := ResolvedActor{}
	assert.Empty(t, plain.NoteWith(""))
	assert.Equal(t, "we say it both ways", plain.NoteWith("we say it both ways"))
}
