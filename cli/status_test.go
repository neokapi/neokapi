package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeStatusProject creates a temp project with a JSON catalog source, a
// partially-translated nb target (2 of 3 keys), no ja target, and ship gates
// (ja machine-ships, the default needs 80% review).
func writeStatusProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()

	recipe := `version: v1
name: status
defaults:
  source_language: en
  target_languages: [nb, ja]
content:
  - path: en.json
    target: "{lang}.json"
ship_gates:
  - when: { locales: [ja] }
    gate: { translated: 100, reviewed: 0 }
  - gate: { translated: 100, reviewed: 80 }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "proj.kapi"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana","c":"Cherry"}`), 0o644))
	// nb has 2 of 3 keys → 67% translated.
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	return root
}

func runStatusJSON(t *testing.T) StatusOutput {
	t.Helper()
	a := &App{}
	cmd := a.NewStatusCmd()
	require.NoError(t, cmd.Flags().Set("json", "true"))
	out, err := captureStdout(t, func() error { return a.runStatus(cmd, nil) })
	require.NoError(t, err)
	var parsed StatusOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed), "status must emit valid JSON: %s", out)
	return parsed
}

func locale(o StatusOutput, loc string) (LocaleCoverage, bool) {
	for _, lc := range o.Locales {
		if lc.Locale == loc {
			return lc, true
		}
	}
	return LocaleCoverage{}, false
}

func TestStatus_Coverage(t *testing.T) {
	t.Chdir(writeStatusProject(t))
	out := runStatusJSON(t)

	nb, ok := locale(out, "nb")
	require.True(t, ok)
	assert.Equal(t, 3, nb.Total)
	assert.Equal(t, 67, nb.Pct["translated"], "2 of 3 keys translated")
	assert.True(t, nb.Gated)
	assert.False(t, nb.Shippable, "default gate needs 100% translated + 80% reviewed")

	ja, ok := locale(out, "ja")
	require.True(t, ok)
	assert.Equal(t, 3, ja.Total)
	assert.Equal(t, 0, ja.Pct["translated"], "no ja file yet")
	assert.True(t, ja.Gated, "ja matches its locale rule")
	assert.False(t, ja.Shippable)
}

func shipGate(out VerifyOutput) (VerifyGateResult, bool) {
	for _, g := range out.Gates {
		if g.Gate == gateShip {
			return g, true
		}
	}
	return VerifyGateResult{}, false
}

func TestVerify_ShipGate(t *testing.T) {
	t.Chdir(writeStatusProject(t))

	// Without --ship: no ship gate is evaluated (drift is non-blocking here).
	a := &App{}
	cmd := a.NewVerifyCmd()
	require.NoError(t, cmd.Flags().Set("json", "true"))
	out, _ := captureStdout(t, func() error { return a.runVerify(cmd, nil) })
	var parsed VerifyOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	_, has := shipGate(parsed)
	assert.False(t, has, "ship gate must be opt-in (--ship)")

	// With --ship: the ship gate runs and fails (nb/ja are below their gates).
	a2 := &App{}
	cmd2 := a2.NewVerifyCmd()
	require.NoError(t, cmd2.Flags().Set("json", "true"))
	require.NoError(t, cmd2.Flags().Set("ship", "true"))
	out2, runErr := captureStdout(t, func() error { return a2.runVerify(cmd2, nil) })
	var parsed2 VerifyOutput
	require.NoError(t, json.Unmarshal([]byte(out2), &parsed2))
	sg, has := shipGate(parsed2)
	require.True(t, has, "--ship adds the ship gate")
	assert.False(t, sg.Pass, "nb/ja are not shippable")
	assert.NotEmpty(t, sg.Findings)
	assert.Error(t, runErr, "a failed ship gate exits non-zero (the pre-release bar)")
}

func TestStatus_NeverFails(t *testing.T) {
	// status is informational: a behind locale is reported, not an error.
	t.Chdir(writeStatusProject(t))
	a := &App{}
	cmd := a.NewStatusCmd()
	_, err := captureStdout(t, func() error { return a.runStatus(cmd, nil) })
	assert.NoError(t, err, "status must never return a non-nil error for drift")
}
