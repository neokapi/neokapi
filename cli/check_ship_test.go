package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckShip_AbsorbsVerify: `kapi check --ship` runs the project's bound
// quality gates through the shared verify engine — brand, terminology, and QA
// findings on the failing fixture — and returns the quality-gate sentinel.
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
	assert.Contains(t, out, "brand", "brand gate runs (Globex drops the score below the default 80)")
	assert.Contains(t, out, "terminology", "terminology gate runs against the project termbase")
	assert.Contains(t, out, "qa", "QA gate runs over the target files")
	assert.Contains(t, out, "FAIL")
}

// TestCheckShip_MinScoreDefaultsToBrandThreshold: check's own --min-score
// default (0, the report score gate) must not leak into ship mode — an
// untouched flag means the brand gate's DefaultBrandMinScore. The fixture's
// brand score is 75, so the brand gate fails only if the 80 default applied.
func TestCheckShip_MinScoreDefaultsToBrandThreshold(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	a := &App{}
	cmd := NewCheckCmd(a)
	require.NoError(t, cmd.Flags().Set("ship", "true"))

	out, runErr := captureStdout(t, func() error { return a.RunCheck(cmd, nil) })
	require.ErrorIs(t, runErr, ErrQualityGate)
	assert.Contains(t, out, "below the required minimum 80",
		"ship mode applies the brand-gate default, not check's 0")
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

// TestVerifyAlias_HiddenAndForwards: `kapi verify` survives as a hidden
// one-release alias that prints a deprecation pointer and forwards to the
// same engine `kapi check --ship` uses.
func TestVerifyAlias_HiddenAndForwards(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	a := &App{}
	cmd := NewVerifyCmd(a)
	assert.True(t, cmd.Hidden, "verify is a hidden alias (#1078 one-release policy)")
	assert.Empty(t, cmd.GroupID, "hidden aliases are excluded from the help groups")

	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	_, runErr := captureStdout(t, func() error { return cmd.RunE(cmd, nil) })
	require.ErrorIs(t, runErr, ErrQualityGate, "the alias still gates")
	assert.Contains(t, stderr.String(), "note: `kapi verify` is deprecated — use `kapi check --ship`")
}
