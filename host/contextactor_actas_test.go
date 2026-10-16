package host

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

// A surface that has said who is acting is believed over the environment: a
// desktop app launched from a terminal inside an agent host inherits that
// host's marker, and without ActAs the person's own imports and decisions would
// be recorded as the agent's and refused.
func TestCommandActorActAsWinsOverTheEnvironment(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	t.Setenv(EnvActor, string(contextop.ActorAgent))

	var app App
	resolved, err := app.commandActor()
	require.NoError(t, err)
	require.Equal(t, contextop.ActorAgent, resolved.Actor.Kind, "the environment names an agent")

	app.ActAs(contextop.Actor{Kind: contextop.ActorPerson})
	resolved, err = app.commandActor()
	require.NoError(t, err)
	require.Equal(t, contextop.ActorPerson, resolved.Actor.Kind)
	require.Empty(t, resolved.Host)
	require.False(t, resolved.PersonOverride)
}
