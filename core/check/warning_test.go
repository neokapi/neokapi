package check

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWarningsNeverChangeTheOutcome pins the contract a warning carries: it is
// reported, and nothing about the run's outcome moves because of it.
func TestWarningsNeverChangeTheOutcome(t *testing.T) {
	warnings := []Warning{{
		Code:    "voice.unknown_key",
		Message: `unknown key "vocab" (line 4) is ignored when the profile loads`,
		Source:  ".kapi/voice.yaml",
		Key:     "channels.docs.vocab",
	}}
	critical := []Diagnostic{{Rule: "terms.vocabulary", Check: "terms", Fails: true}}
	for _, tc := range []struct {
		name    string
		target  Target
		diags   []Diagnostic
		verdict Verdict
	}{
		{name: "passed", target: Target{Kind: "file", Blocks: 2}, verdict: VerdictPassed},
		{name: "failed", target: Target{Kind: "file", Blocks: 2}, diags: critical, verdict: VerdictFailed},
		{name: "did not run", target: Target{Kind: "file"}, verdict: VerdictDidNotRun},
	} {
		t.Run(tc.name, func(t *testing.T) {
			without := BuildReport(tc.target, tc.diags)
			with := BuildReport(tc.target, tc.diags)
			with.Warnings = warnings
			with.Decide()

			require.Equal(t, tc.verdict, with.Verdict)
			assert.Equal(t, without.Verdict, with.Verdict)
			assert.Equal(t, without.Pass, with.Pass)
			assert.Equal(t, without.DidNotRun, with.DidNotRun)
			assert.Equal(t, without.DidNotRunCause, with.DidNotRunCause)
			assert.Equal(t, without.Summary, with.Summary)
			assert.Equal(t, warnings, with.Warnings)
		})
	}
}

func TestWarningsAreOmittedWhenThereAreNone(t *testing.T) {
	body, err := json.Marshal(BuildReport(Target{Kind: "file", Blocks: 1}, nil))
	require.NoError(t, err)
	assert.NotContains(t, string(body), `"warnings"`)
}

func TestMergeWarningsKeepsEachWarningOnceInOrder(t *testing.T) {
	a := Warning{Code: "voice.unknown_key", Message: "m", Source: ".kapi/voice.yaml", Key: "tone.formalty"}
	b := Warning{Code: "voice.unfamiliar_value", Message: "n", Source: ".kapi/voice.yaml", Key: "tone.emotion"}
	c := Warning{Code: "voice.unknown_key", Message: "o", Source: "pack:docs", Key: "style.x"}

	assert.Equal(t, []Warning{b, a, c}, MergeWarnings([]Warning{c, a}, []Warning{a, b}, []Warning{c}))
	assert.Nil(t, MergeWarnings())
	assert.Nil(t, MergeWarnings(nil, []Warning{}))
}
