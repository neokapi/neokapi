package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// The command line's half of the decision model.
//
// An operation recorded from a shell states no actor, so the environment
// answers. These tests drive the host API with an empty actor, which is what
// `kapi context observe` and its neighbours pass, and assert on what the log
// holds afterwards.

// asAgent puts the environment an agent host leaves on a command in place for
// one test. KAPI_ACTOR is set, which is also what takes this process past the
// carve-out commandActor makes for a test binary.
func asAgent(t *testing.T, name, session string) {
	t.Helper()
	t.Setenv(EnvActor, "agent")
	t.Setenv(EnvAgentName, name)
	t.Setenv(EnvAgentSession, session)
}

// TestACommandLineAgentIsRecordedAsOne is issue #2912: the skill drives the
// command line for the three write habits, and every one of them went into the
// log as a person.
func TestACommandLineAgentIsRecordedAsOne(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-cli-agent")
	asAgent(t, "codex", "s-cli-1")

	observed, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Project:  recipeOf(root),
		Text:     "the docs address the reader as you",
		Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
	})
	require.NoError(t, err)
	assert.Equal(t, contextop.ActorAgent, observed.Actor.Kind)
	assert.Equal(t, "codex", observed.Actor.Name)
	assert.Equal(t, "s-cli-1", observed.Actor.Session)

	proposed, err := app.ProposeContextRule(t.Context(), ContextProposeRequest{
		Project:  recipeOf(root),
		Term:     &coreprofile.TermRule{Term: "utilise", Replacement: "use"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml", Quote: "We utilise the widget"}},
	})
	require.NoError(t, err)
	assert.Equal(t, contextop.ActorAgent, proposed.Actor.Kind)

	corrected, err := app.RecordContextCorrection(t.Context(), ContextCorrectRequest{
		Project:  recipeOf(root),
		From:     "sign in",
		To:       "log in",
		Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
	})
	require.NoError(t, err)
	assert.Equal(t, contextop.ActorAgent, corrected.Actor.Kind)

	// The three filters a person reviews an agent run with.
	byKind, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root), Actor: "agent"})
	require.NoError(t, err)
	assert.Len(t, byKind.Operations, 3)

	bySession, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root), Session: "s-cli-1"})
	require.NoError(t, err)
	assert.Len(t, bySession.Operations, 3)

	mine, err := app.ContextOperations(t.Context(), ContextLogRequest{
		Project: recipeOf(root), Session: ContextLogSessionSelf,
	})
	require.NoError(t, err)
	assert.Len(t, mine.Operations, 3, "an agent reads its own run back without being told the id")
}

// TestACommandLineAgentIsRefusedEveryDecision is the other half of #2912: with
// the actor resolved, the policy reaches the command line, and an agent that
// tries a decision is told what to do instead.
func TestACommandLineAgentIsRefusedEveryDecision(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-cli-policy")
	proposed := proposeUtilise(t, app, root, person)
	asAgent(t, "codex", "s-cli-2")

	_, err := app.ConfirmContextOperation(t.Context(), ContextConfirmRequest{
		Project: recipeOf(root), ID: proposed.ID,
	})
	require.ErrorIs(t, err, contextop.ErrRefused)
	assert.Contains(t, err.Error(), "kapi context log --status candidate",
		"the refusal teaches the move that is the agent's to make")

	_, err = app.DiscardContextOperation(t.Context(), ContextDiscardRequest{
		Project: recipeOf(root), ID: proposed.ID,
	})
	require.ErrorIs(t, err, contextop.ErrRefused, "a person's proposal is not an agent's to discard")

	_, err = app.RevertContextOperations(t.Context(), ContextRevertRequest{
		Project: recipeOf(root), Session: "s-cli-2",
	})
	require.ErrorIs(t, err, contextop.ErrRefused, "reverting a whole session is a person's")

	_, err = app.WidenContextOperation(t.Context(), ContextWidenRequest{
		Project: recipeOf(root), ID: proposed.ID, To: WidenToWorkspace,
	})
	require.Error(t, err)
}

// TestACommandLineAgentWithdrawsItsOwnCandidate: the policy already lets an
// actor clean up after itself inside its own session, and the command line
// reaches that the same way the MCP tools do.
func TestACommandLineAgentWithdrawsItsOwnCandidate(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-cli-withdraw")
	asAgent(t, "codex", "s-cli-3")

	proposed, err := app.ProposeContextRule(t.Context(), ContextProposeRequest{
		Project:  recipeOf(root),
		Term:     &coreprofile.TermRule{Term: "utilise", Replacement: "use"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml", Quote: "We utilise the widget"}},
	})
	require.NoError(t, err)

	_, err = app.DiscardContextOperation(t.Context(), ContextDiscardRequest{
		Project: recipeOf(root), ID: proposed.ID,
	})
	require.NoError(t, err, "an agent may withdraw what it proposed in this session")

	held, err := app.ContextOperations(t.Context(), ContextLogRequest{
		Project: recipeOf(root), Subjects: true,
	})
	require.NoError(t, err)
	require.Len(t, held.Operations, 1)
	assert.Equal(t, contextop.StatusDiscarded, held.Operations[0].Status)
}

// TestAPersonInAnAgentShellIsOnTheRecord: the override exists for a person
// typing in an agent host's shell, and an operation that used it says so.
func TestAPersonInAnAgentShellIsOnTheRecord(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-cli-override")
	t.Setenv("CLAUDECODE", "1")
	t.Setenv(EnvActor, "person")

	observed, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Project: recipeOf(root),
		Text:    "the docs address the reader as you",
	})
	require.NoError(t, err)
	assert.Equal(t, contextop.ActorPerson, observed.Actor.Kind)
	assert.Equal(t, "recorded as a person in a claude-code shell", observed.Note)

	confirmed, err := app.ConfirmContextOperation(t.Context(), ContextConfirmRequest{
		Project: recipeOf(root),
		ID:      proposeUtilise(t, app, root, person).ID,
		Note:    "we say use everywhere",
	})
	require.NoError(t, err, "the override carries a person's rights, which is what it is for")
	assert.Equal(t, "we say use everywhere (recorded as a person in a claude-code shell)", confirmed.Note)
}

// TestAStatedActorIsTakenAtItsWord: the MCP tools and the desktop name the
// actor, and the environment does not overrule them.
func TestAStatedActorIsTakenAtItsWord(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-cli-stated")
	asAgent(t, "codex", "s-cli-4")

	observed, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor:   person,
		Project: recipeOf(root),
		Text:    "the docs address the reader as you",
	})
	require.NoError(t, err)
	assert.Equal(t, person, observed.Actor)
	assert.Empty(t, observed.Note)
}

// TestReadingThisSessionNeedsOne: a person typing at their own shell has no
// session, and `--session this` says so rather than answering with nothing.
func TestReadingThisSessionNeedsOne(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-cli-nosession")

	_, err := app.ContextOperations(t.Context(), ContextLogRequest{
		Project: recipeOf(root), Session: ContextLogSessionSelf,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), ContextLogSessionSelf)
}
