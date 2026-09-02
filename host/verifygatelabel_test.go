package host

import (
	"bytes"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ansi strips terminal styling so an assertion reads the text a user sees
// rather than the escape sequences the theme wraps it in.
var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

// TestVerifyGateLabel_TextShowsChecksAndJSONKeepsTheID pins the split between
// the label and the id. `checks` is the word for the gate in the repo's
// vocabulary, so the human table shows it; `qa` is what --gate accepts, what a
// recipe key names, and what the JSON result carries, so it stays byte-identical
// on every machine-readable surface.
func TestVerifyGateLabel_TextShowsChecksAndJSONKeepsTheID(t *testing.T) {
	out := verifyOutput{
		Gates: []verifyGateResult{{
			Gate: gateChecks,
			Pass: false,
			Findings: []verifyFinding{{
				Gate:     gateChecks,
				File:     "docs/index.md",
				Severity: "error",
				Message:  "placeholder {count} is missing from the target",
			}},
		}},
		Summary: verifySummary{Gates: 1, Failed: 1, Findings: 1, Errors: 1},
	}

	var buf bytes.Buffer
	require.NoError(t, out.FormatText(&buf))
	text := ansi.ReplaceAllString(buf.String(), "")

	assert.Contains(t, text, "checks", "the gate table labels the qa gate `checks`")
	assert.NotRegexp(t, `(?m)^\s*qa\b`, text, "the raw gate id must not reach the table")

	encoded, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"gate":"qa"`,
		"the JSON result carries the gate id, not its label")
	assert.NotContains(t, string(encoded), `"gate":"checks"`)
}

// TestVerifyGateLabel_SelectorsTakeTheID keeps --gate on the ids: a user who
// reads `checks` in the table still passes `qa`, and the help text lists what
// the flag accepts.
func TestVerifyGateLabel_SelectorsTakeTheID(t *testing.T) {
	assert.Contains(t, selectableGates, "qa")
	assert.NotContains(t, selectableGates, "checks")

	cmd := NewEnvCommand(t.Context(), "check")
	AddGateFlag(cmd)
	f := cmd.Flags().Lookup(gateFlagName)
	require.NotNil(t, f)
	assert.Contains(t, f.Usage, "qa")

	require.NoError(t, cmd.Flags().Set(gateFlagName, "qa"))
	sel, err := resolveGateSelection(cmd)
	require.NoError(t, err)
	assert.True(t, sel.qa)
	assert.False(t, sel.voice)
	assert.False(t, sel.terms)
}

// TestVerifyGateLabel_UnnamedGatesKeepTheirOwnWord asserts the mapping covers
// only the gate that needs one, so voice and terminology render as themselves.
func TestVerifyGateLabel_UnnamedGatesKeepTheirOwnWord(t *testing.T) {
	assert.Equal(t, "voice", gateDisplayName(gateVoice))
	assert.Equal(t, "terminology", gateDisplayName(gateTerms))
	assert.Equal(t, "checks", gateDisplayName(gateChecks))
	assert.Equal(t, "ship", gateDisplayName(gateShip))
}
