package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMCPScenariosNameRealTools.
//
// The MCP scenarios score whether the agent picked the tool the task called
// for, so a scenario naming a tool the server does not advertise would score
// every run as a miss and read as a defect in the agent rather than in the
// scenario.
func TestMCPScenariosNameRealTools(t *testing.T) {
	known := map[string]bool{}
	for _, name := range mcpToolCatalogue {
		known[name] = true
	}
	for _, sc := range scenarios {
		if surfaceOf(sc) != surfaceMCP {
			continue
		}
		t.Run(sc.ID, func(t *testing.T) {
			if sc.Kind == negative {
				assert.Empty(t, sc.ExpectTool, "a negative expects no tool at all")
				return
			}
			require.NotEmpty(t, sc.ExpectTool, "an MCP positive is scored on the tool it should reach")
			assert.True(t, known[sc.ExpectTool],
				"scenario expects %q, which the server does not advertise", sc.ExpectTool)
		})
	}
}

// TestMCPScoringIgnoresMereActivation.
//
// The trap this eval could fall into. On MCP the tool list is already in
// context, so "did it reach kapi" is nearly free and says almost nothing. An
// agent that calls the WRONG tool has still triggered, and scoring on Fired
// would call that a pass.
func TestMCPScoringIgnoresMereActivation(t *testing.T) {
	wrong := Result{
		Scenario: Scenario{Kind: positive, Surface: surfaceMCP, ExpectTool: "voice_rewrite"},
		Runs: []Run{
			{Triggered: true, MCPTools: []string{"voice_check"}},
			{Triggered: true, MCPTools: []string{"voice_check"}},
		},
	}
	wrong.score(modeTrigger)
	assert.Equal(t, "fail", wrong.Verdict, "reaching the wrong tool twice is not a pass")
	assert.Equal(t, []string{"voice_check"}, wrong.WrongTool, "the near miss is the useful output")
	assert.Equal(t, 2, wrong.Fired, "it did reach kapi, and that is not what is scored here")

	right := Result{
		Scenario: Scenario{Kind: positive, Surface: surfaceMCP, ExpectTool: "voice_rewrite"},
		Runs: []Run{
			{Triggered: true, MCPTools: []string{"voice_check", "voice_rewrite"}},
			{Triggered: true, MCPTools: []string{"voice_rewrite"}},
		},
	}
	right.score(modeTrigger)
	assert.Equal(t, "pass", right.Verdict, "looking first and then choosing correctly is a pass")

	quiet := Result{
		Scenario: Scenario{Kind: negative, Surface: surfaceMCP},
		Runs:     []Run{{Triggered: false}, {Triggered: false}},
	}
	quiet.score(modeTrigger)
	assert.Equal(t, "pass", quiet.Verdict)
}

// TestSkillScenariosCarryNoMCPExpectation: a skill scenario scored against an
// MCP tool would be scored against something it was never offered.
func TestSkillScenariosCarryNoMCPExpectation(t *testing.T) {
	for _, sc := range scenarios {
		if surfaceOf(sc) == surfaceMCP {
			continue
		}
		assert.Empty(t, sc.ExpectTool, "%s is a skill scenario and has no MCP server", sc.ID)
	}
}

// TestTheTwoSurfacesDoNotOverwriteEachOther.
//
// The dataset is one file holding several reports. Keying it on mode alone was
// enough while every scenario used the skill; with two surfaces an MCP sweep
// would silently erase the skill numbers, and the dashboard would show a
// smaller suite with no sign anything had been lost.
func TestTheTwoSurfacesDoNotOverwriteEachOther(t *testing.T) {
	keys := map[string]bool{}
	for _, r := range []*Report{
		{Mode: modeTrigger, Surface: surfaceSkill},
		{Mode: modeCompletion, Surface: surfaceSkill},
		{Mode: modeTrigger, Surface: surfaceMCP},
	} {
		k := r.Key()
		assert.False(t, keys[k], "two reports share the key %q", k)
		keys[k] = true
	}

	// An unset surface means the skill, so these are the same report and must
	// key the same way. Reports written before MCP existed carry no surface,
	// and treating them as a fourth kind would split the skill history in two.
	assert.Equal(t,
		(&Report{Mode: modeTrigger}).Key(),
		(&Report{Mode: modeTrigger, Surface: surfaceSkill}).Key(),
		"an unset surface is the skill, not a separate one")
}

// TestCompletionSkipsMCP: completion mode drives a scenario to a green gate,
// and an MCP scenario is already fully scored by which tool it picked. Running
// it again in completion mode would spend money to learn the same thing.
func TestCompletionSkipsMCP(t *testing.T) {
	set := selectScenarios("", modeCompletion, "")
	for _, sc := range set {
		assert.NotEqual(t, surfaceMCP, surfaceOf(sc),
			"%s is an MCP scenario and completion mode has nothing extra to tell it", sc.ID)
	}
	assert.NotEmpty(t, set, "completion mode still has skill positives to run")
}
