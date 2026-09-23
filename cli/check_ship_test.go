package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckShip_AbsorbsVerify: `kapi check --ship` runs the project's bound
// quality gates through the shared verify engine (voice, terminology, and
// rule-based findings on the failing fixture) and returns the quality-gate
// sentinel.
func TestCheckShip_AbsorbsVerify(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	a := &App{}
	cmd := NewCheckCmd(a)
	require.NoError(t, cmd.Flags().Set("ship", "true"))

	out, runErr := captureStdout(t, func() error {
		return a.RunCheck(cmd, nil)
	})

	require.ErrorIs(t, runErr, ErrQualityGate, "unmet gates exit non-zero (exit 3)")
	assert.Contains(t, out, "voice", "voice gate runs (Globex drops the score below the default 80)")
	assert.Contains(t, out, "terminology", "terminology gate runs against the project terms store")
	assert.Contains(t, out, "checks", "the rule-based check gate runs over the target files")
	assert.NotContains(t, out, "qa", "the table labels that gate `checks`; `qa` is the id --gate takes")
	assert.Contains(t, out, "FAIL")
}

// TestCheckShip_VoiceGateFailsOnAFailingFinding: the voice gate fails on a
// failing finding, and the compliance score it reports gates nothing.
func TestCheckShip_VoiceGateFailsOnAFailingFinding(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	a := &App{}
	cmd := NewCheckCmd(a)
	require.NoError(t, cmd.Flags().Set("ship", "true"))

	out, runErr := captureStdout(t, func() error { return a.RunCheck(cmd, nil) })
	require.ErrorIs(t, runErr, ErrQualityGate)
	assert.NotContains(t, out, "below the required minimum")
	assert.Contains(t, out, "FAILS")
}

// TestCheckShip_NoFailReportsButExitsZero: --no-fail keeps ship mode
// report-only, matching verify's fix-loop semantics.
func TestCheckShip_NoFailReportsButExitsZero(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	a := &App{}
	cmd := NewCheckCmd(a)
	require.NoError(t, cmd.Flags().Set("ship", "true"))
	require.NoError(t, cmd.Flags().Set("no-fail", "true"))

	out, runErr := captureStdout(t, func() error { return a.RunCheck(cmd, nil) })
	require.NoError(t, runErr, "--no-fail downgrades a failing gate to report-only")
	assert.Contains(t, out, "FAIL", "the verdict is still reported")
}
