package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every positive scenario now carries a definition of done. That is worth a
// test rather than a count, because the failure mode is not a missing gate: it
// is a gate that cannot fail, which reads as a pass forever.

func TestEveryPositiveScenarioHasAGate(t *testing.T) {
	var ungated []string
	for _, sc := range scenarios {
		if sc.Kind != positive || surfaceOf(sc) == surfaceMCP {
			continue
		}
		if strings.TrimSpace(sc.CompletionGate) == "" {
			ungated = append(ungated, sc.ID)
		}
	}
	assert.Empty(t, ungated,
		"a scenario with no definition of done verifies nothing, and completion mode reports it as `no gate` rather than as a pass")
}

// TestAGateIsRedBeforeTheAgentRuns builds each scenario's workspace and runs its
// gate against it, exactly as a sweep does, and requires every one to fail.
//
// Reading the gate as a string was not enough. It caught `|| true` and it
// caught `test -f <a fixture file>`, and it passed three gates that were green
// on an untouched workspace anyway:
//
//   - `kapi voice check` accepts any YAML at all. Every profile field is
//     optional, so an empty file loads as a profile and scores 100/100 with no
//     findings, and the gate was satisfied by a directory that merely contained
//     a .yaml — which p16 and p17 both ship.
//   - p17's whole point is that the profile must NOT be rewritten before the
//     agent asks, so "a valid profile exists" was green at the start and stayed
//     green if the agent renamed everything.
//   - p11's project gate was satisfied by the recipe the fixture provides.
//
// Running the gate is the only check that finds these, so this test runs it.
func TestAGateIsRedBeforeTheAgentRuns(t *testing.T) {
	root, err := repoRoot()
	require.NoError(t, err)
	kapiBin := findKapi(root)
	if kapiBin == "" {
		t.Skip("no kapi binary: `make build` first, or the gates cannot be run")
	}
	opts := Options{RepoRoot: root, KapiBin: kapiBin}

	for i := range scenarios {
		sc := scenarios[i]
		if strings.TrimSpace(sc.CompletionGate) == "" {
			continue
		}
		t.Run(sc.ID, func(t *testing.T) {
			t.Parallel()
			dir, err := os.MkdirTemp("", "gate-"+sc.ID+"-")
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.RemoveAll(dir) })

			require.NoError(t, buildWorkspace(dir, &sc, root, kapiBin, armSkill))

			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			res := runGate(ctx, dir, sc.CompletionGate, opts)
			require.NotNil(t, res)

			assert.NotZero(t, res.ExitCode,
				"the gate passes on the workspace the agent starts in, so it measures the fixture rather than the agent\ngate: %s\noutput: %s",
				sc.CompletionGate, res.Output)
		})
	}
}

// TestGateSyntaxCannotBeTriviallyTrue keeps the cheap string checks, which run
// without a kapi binary and name the mistake more directly than a red exit code
// would.
func TestGateSyntaxCannotBeTriviallyTrue(t *testing.T) {
	for _, sc := range scenarios {
		if sc.CompletionGate == "" {
			continue
		}
		t.Run(sc.ID, func(t *testing.T) {
			g := sc.CompletionGate
			assert.NotContains(t, g, "|| true", "this can never fail")
			assert.NotContains(t, g, "; true", "this can never fail")
			assert.NotEqual(t, "true", strings.TrimSpace(g))
		})
	}
}

// TestProjectGatesPassTheProjectExplicitly.
//
// The isolation contract sets KAPI_NO_PROJECT=1, so discovery is off. A bare
// `kapi status` cannot find the recipe the agent just wrote, and the gate would
// fail for a reason that has nothing to do with the agent.
func TestProjectGatesPassTheProjectExplicitly(t *testing.T) {
	for _, sc := range scenarios {
		g := sc.CompletionGate
		if g == "" {
			continue
		}
		for _, verb := range []string{"kapi status", "kapi check", "kapi up"} {
			if !strings.Contains(g, verb) {
				continue
			}
			assert.Contains(t, g, "-p .",
				"%s: `%s` needs -p . because KAPI_NO_PROJECT=1 turns discovery off", sc.ID, verb)
		}
	}
}

// TestAnswerGatesUseTheAnswerFile: a scenario whose deliverable is an answer
// leaves no artefact, so its gate reads the closing message the runner wrote.
func TestAnswerGatesUseTheAnswerFile(t *testing.T) {
	found := 0
	for _, sc := range scenarios {
		if strings.Contains(sc.CompletionGate, answerFile) {
			found++
		}
	}
	assert.Positive(t, found,
		"the read-only scenarios are gated on the closing message, or they are not gated at all")
}

// TestEveryFixtureRecipeLoads.
//
// Both project fixtures were invented, and every key in them was wrong:
// `version: "1"` where the loader wants `v1`, `source:`/`targets:` where the
// fields are `source_language`/`target_languages`, `include:` where a
// collection carries `content: - path:`. Only the last was ever reported, since
// Defaults and Collection both end in an inline Extras map and an unrecognised
// key is preserved rather than rejected. So the sweep spent its budget handing
// agents a project kapi refuses to load, and said nothing.
func TestEveryFixtureRecipeLoads(t *testing.T) {
	root, err := repoRoot()
	require.NoError(t, err)
	kapiBin := findKapi(root)
	if kapiBin == "" {
		t.Skip("no kapi binary: `make build` first")
	}
	opts := Options{RepoRoot: root, KapiBin: kapiBin}

	// scenarios already holds the MCP set: mcp_scenarios.go appends it in init.
	for i := range scenarios {
		sc := scenarios[i]
		hasRecipe := false
		for _, f := range sc.Fixture {
			if f.As == "kapi.yaml" {
				hasRecipe = true
			}
		}
		if !hasRecipe {
			continue
		}
		t.Run(sc.ID, func(t *testing.T) {
			t.Parallel()
			dir, err := os.MkdirTemp("", "recipe-"+sc.ID+"-")
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			require.NoError(t, buildWorkspace(dir, &sc, root, kapiBin, armSkill))

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			res := runGate(ctx, dir, "kapi status -p . >/dev/null", opts)
			require.NotNil(t, res)
			assert.Zero(t, res.ExitCode, "kapi cannot load this fixture's recipe: %s", res.Output)
		})
	}
}
