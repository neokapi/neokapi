package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// writeEmptyShipProject writes a project whose only declared content holds no
// blocks, so every content gate over it checks nothing.
func writeEmptyShipProject(t *testing.T) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	recipe := "version: v1\nname: empty\ndefaults:\n  source_language: en\ncollections:\n  - name: content\n    content:\n      - path: content.json\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "content.json"), []byte(`{}`), 0o644))
	return root
}

// TestShip_ANoContentGateDidNotRun is the ship gate's zero-coverage case: the
// checks gate reads a file with no blocks. It must not pass, under any flag.
func TestShip_ANoContentGateDidNotRun(t *testing.T) {
	root := writeEmptyShipProject(t)

	out, err := (&App{}).computeVerify(sourceShipCommand(t, root), nil)
	require.NoError(t, err)
	qa, ok := gateByName(out, gateChecks)
	require.True(t, ok)
	require.NotNil(t, qa.Coverage)
	assert.Equal(t, 0, qa.Coverage.Blocks)
	assert.Equal(t, check.VerdictDidNotRun, qa.Verdict)
	assert.False(t, qa.Pass)
	assert.Equal(t, check.VerdictDidNotRun, out.Verdict)
	assert.Equal(t, check.CauseNothingToCheck, out.DidNotRunCause)
	assert.False(t, out.Pass)

	for _, noFail := range []bool{false, true} {
		cmd := sourceShipCommand(t, root)
		if noFail {
			require.NoError(t, cmd.Flags().Set("no-fail", "true"))
		}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		runErr := (&App{}).RunVerify(cmd, nil)
		require.ErrorIs(t, runErr, ErrCheckNotRun, "no-fail=%v", noFail)
		assert.Equal(t, ExitNotRun, ExitCode(cmd, runErr))
		assert.Contains(t, buf.String(), "DID NOT RUN")
		assert.Contains(t, buf.String(), "Did not run: there was nothing in scope to check. (nothing_to_check)")
		assert.NotContains(t, buf.String(), "NO CONTENT")
	}
}

// TestShip_ABrokenCheckerReadsDifferentlyFromNothingToCheck puts an inert
// checker behind the ship gate's source checks. The gate must say a checker
// failed its canary, which is not what the empty project above says.
func TestShip_ABrokenCheckerReadsDifferentlyFromNothingToCheck(t *testing.T) {
	root, _ := sourceShipFixture(t)
	real := hygieneTool
	hygieneTool = inertHygiene
	t.Cleanup(func() { hygieneTool = real })

	cmd := sourceShipCommand(t, root)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	runErr := (&App{}).RunVerify(cmd, nil)
	require.ErrorIs(t, runErr, ErrCheckNotRun)
	assert.Equal(t, ExitNotRun, ExitCode(cmd, runErr))
	assert.Contains(t, buf.String(), "Did not run: a checker failed its canary, so this run's result cannot be trusted. (checker_invalid)")
	assert.NotContains(t, buf.String(), "nothing in scope")
	assert.Contains(t, runErr.Error(), check.CauseCheckerInvalid)

	out, err := (&App{}).computeVerify(sourceShipCommand(t, root), nil)
	require.NoError(t, err)
	assert.Equal(t, check.VerdictDidNotRun, out.Verdict)
	assert.Equal(t, check.CauseCheckerInvalid, out.DidNotRunCause)
}

// TestShip_TargetGatesCarryCanaries runs the bilingual checks and terminology
// gates over a real pair and asserts each analyzer run caught its canary.
func TestShip_TargetGatesCarryCanaries(t *testing.T) {
	root, target := writeVerifyProject(t)
	// A clean target, so the gates pass and the canaries are what shows they
	// could have failed.
	require.NoError(t, os.WriteFile(target, []byte("{\n  \"greeting\": \"Bonjour {name}, bienvenue chez Globex!\",\n  \"save\": \"Enregistrer\"\n}\n"), 0o644))
	t.Chdir(root)

	out, err := runVerifyGates(t, map[string]string{"gate": gateChecks + "," + gateTerms})
	_ = err
	for _, name := range []string{gateChecks, gateTerms} {
		g, ok := gateByName(out, name)
		require.True(t, ok, name)
		require.NotNil(t, g.Execution, name)
		caught := 0
		for _, run := range g.Execution.Analyzers {
			if run.ID != "checks.target" && run.ID != "terms.target" {
				continue
			}
			require.NotNil(t, run.Canary, run.ID)
			assert.Equal(t, check.CanaryCaught, run.Canary.Status, run.ID)
			caught++
		}
		assert.Positive(t, caught, "%s ran its target analyzer", name)
	}
}

// TestBuildVerifyOutput_Precedence settles a run from its gates in the order a
// single report is settled: a missed canary anywhere, then a failure, then a
// gate that did not run.
func TestBuildVerifyOutput_Precedence(t *testing.T) {
	proven := &check.Execution{Analyzers: []check.AnalyzerExecution{{ID: "hygiene", Status: check.AnalyzerPassed, Required: true, Canary: &check.CanaryOutcome{Status: check.CanaryCaught, Probes: 1}}}}
	invalid := &check.Execution{Analyzers: []check.AnalyzerExecution{{ID: "hygiene", Status: check.AnalyzerInvalid, Canary: &check.CanaryOutcome{Status: check.CanaryMissed}}}}
	passing := verifyGateResult{Gate: gateChecks, Pass: true, Coverage: &verifyCoverage{Files: 1, Blocks: 3}, Execution: proven}
	failing := verifyGateResult{Gate: gateTerms, Pass: false, Coverage: &verifyCoverage{Files: 1, Blocks: 3}, Execution: proven}
	empty := verifyGateResult{Gate: gateChecks, Pass: true, Coverage: &verifyCoverage{}, Execution: proven}
	broken := verifyGateResult{Gate: gateChecks, Pass: true, Coverage: &verifyCoverage{Files: 1, Blocks: 3}, Execution: invalid}
	ship := verifyGateResult{Gate: gateShip, Pass: true}

	tests := []struct {
		name  string
		gates []verifyGateResult
		want  check.Verdict
		cause string
	}{
		{"a passing content gate beside a passing ship gate", []verifyGateResult{passing, ship}, check.VerdictPassed, ""},
		{"a failure outranks a gate with nothing to check", []verifyGateResult{failing, empty}, check.VerdictFailed, ""},
		{"a missed canary outranks a failure", []verifyGateResult{failing, broken}, check.VerdictDidNotRun, check.CauseCheckerInvalid},
		{"a gate with nothing to check leaves the run unverified", []verifyGateResult{passing, empty}, check.VerdictDidNotRun, check.CauseNothingToCheck},
		{"an unbound gate did not run", []verifyGateResult{passing, unboundGate(gateVoice, "defaults.voice")}, check.VerdictDidNotRun, check.CauseContentNotChecked},
		{"no gate at all", nil, check.VerdictDidNotRun, check.CauseNothingToCheck},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := buildVerifyOutput(append([]verifyGateResult(nil), tt.gates...))
			assert.Equal(t, tt.want, out.Verdict)
			assert.Equal(t, tt.cause, out.DidNotRunCause)
			assert.Equal(t, tt.want == check.VerdictPassed, out.Pass)
			assert.Equal(t, len(tt.gates), out.Summary.Passed+out.Summary.Failed+out.Summary.DidNotRun)
		})
	}
}
