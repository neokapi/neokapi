package main

import (
	"context"
	"os"
	"strings"
	"testing"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPulledWorkspaceExposesTheSkill asks the agent what it can see.
//
// The pulled arm's finding is "it never asked", and that sentence has two
// readings: the model did not think to, or the harness never gave it the skill.
// Only one of those is about the product, and the dataset cannot tell them apart
// — a workspace that fails to expose the skill produces exactly the same empty
// KapiCommands as a model that ignored it.
//
// So this asks, through runAgent rather than a hand-rolled invocation, because a
// probe that differs from the real run in one flag proves nothing about the real
// run. Both arms are probed: the skill has to appear in the pulled one and not
// in the bare one, or it is coming from the developer's own ~/.claude and the
// arms never differed at all.
//
// Costs two model calls, so it runs on request:
//
//	LAB_PROBE=1 go test ./scripts/authoringlab -run ExposesTheSkill -v
func TestPulledWorkspaceExposesTheSkill(t *testing.T) {
	if os.Getenv("LAB_PROBE") == "" {
		t.Skip("set LAB_PROBE=1: this spends two model calls")
	}
	root := testRepoRoot(t)
	if _, err := pristineTar(root); err != nil {
		t.Skip("no subject archive: ./scripts/fetch-lab-repo.sh")
	}
	claudeBin, err := findClaude()
	require.NoError(t, err)
	kapiBin, err := findKapi(root)
	require.NoError(t, err)

	base, err := loadProfile()
	require.NoError(t, err)
	profile := coreprofile.ResolveProfile(base, "", "", points[0].Persona)

	const prompt = "List the names of every Agent Skill available to you right now, " +
		"one per line, nothing else. Use no tools other than the one that writes the file."

	ctx := context.Background()
	pulled := runAgent(ctx, AgentOpts{
		ClaudeBin: claudeBin, Root: root, Model: "claude-sonnet-5", Prompt: prompt,
		KapiBin: kapiBin, Arm: armSetup{pull: true, profile: profile},
	})
	require.Empty(t, pulled.Err)
	bare := runAgent(ctx, AgentOpts{
		ClaudeBin: claudeBin, Root: root, Model: "claude-sonnet-5", Prompt: prompt,
	})
	require.Empty(t, bare.Err)

	t.Logf("pulled saw:\n%s", pulled.Text)
	assert.True(t, listsSkill(pulled.Text, "kapi"),
		"the pulled arm cannot ask for what it was never given: the workspace installs "+
			"the skill but the agent does not see it, so an empty KapiCommands says nothing about the model")
	assert.False(t, listsSkill(bare.Text, "kapi"),
		"the bare arm sees the kapi skill too, so it is coming from the developer's own "+
			"~/.claude and the two arms are not the comparison this lab publishes")
}

// TestTheRulesFilesReachTheAgent is the follow-up experiment, not a gate.
//
// The pulled arm's finding is that no run reached for kapi. The product's
// answer is to put the rules where the agent already looks: kapi writes the
// voice and the rules in force into AGENTS.md and CLAUDE.md, which Claude Code
// loads without a tool call, ending on the line that asks for `kapi check`
// before the work is done.
//
// So: the same workspace with the rules files kapi writes. The agent needs no
// lookup to follow the voice; what is measured is whether it runs the check the
// section asks for. Either outcome is a result worth reading, and the assertion
// states which one it found.
//
//	LAB_PROBE=1 go test ./scripts/authoringlab -run RulesFilesReachTheAgent -v
func TestTheRulesFilesReachTheAgent(t *testing.T) {
	if os.Getenv("LAB_PROBE") == "" {
		t.Skip("set LAB_PROBE=1: this spends model calls")
	}
	root := testRepoRoot(t)
	if _, err := pristineTar(root); err != nil {
		t.Skip("no subject archive: ./scripts/fetch-lab-repo.sh")
	}
	claudeBin, err := findClaude()
	require.NoError(t, err)
	kapiBin, err := findKapi(root)
	require.NoError(t, err)
	base, err := loadProfile()
	require.NoError(t, err)

	point := points[0]
	profile := coreprofile.ResolveProfile(base, "", "", point.Persona)

	for _, model := range []string{"claude-sonnet-5", "claude-opus-5"} {
		t.Run(model, func(t *testing.T) {
			run := runAgent(context.Background(), AgentOpts{
				ClaudeBin: claudeBin, Root: root, Model: model, Prompt: point.Task,
				KapiBin: kapiBin,
				Arm:     armSetup{pull: true, profile: profile, rulesFiles: true},
			})
			require.Empty(t, run.Err)
			t.Logf("%s ran: %v", model, run.KapiCommands)
			ranCheck := false
			for _, c := range run.KapiCommands {
				if strings.Contains(c, "kapi check") {
					ranCheck = true
				}
			}
			assert.True(t, ranCheck,
				"with the rules in CLAUDE.md asking for `kapi check`, the agent finished without running it")
		})
	}
}

// listsSkill matches a name on its own line, so `okapi-expert` is not `kapi`.
func listsSkill(text, name string) bool {
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "-*0123456789. \t")
		if before, _, _ := strings.Cut(line, ":"); strings.TrimSpace(before) == name {
			return true
		}
	}
	return false
}
