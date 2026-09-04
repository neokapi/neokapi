package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/host/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// northwindVoice forbids three terms and names no replacement for any: the
// profile from issue #2219, on which the rewrite returned the text unchanged
// with `changes: null` and exit 0, indistinguishable from a clean text.
const northwindVoice = `name: Northwind
tone:
    personality: [plain, direct]
    formality: neutral
    emotion: neutral
    humor: none
style:
    active_voice: true
vocabulary:
    forbidden_terms:
        - term: utilize
          severity: major
        - term: leverage
          severity: major
        - term: cutting-edge
          severity: major
`

const mixedVoice = `name: Mixed
tone:
    formality: neutral
vocabulary:
    forbidden_terms:
        - term: utilize
          replacement: use
        - term: leverage
          severity: minor
    competitor_terms:
        - term: Globex
`

// runVoiceRewrite executes `voice rewrite` on a fresh command with the given
// arguments and returns what it printed and the RunE error.
func runVoiceRewrite(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "1")
	cmd := newVoiceRewriteCmd(&App{})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestVoiceRewrite_ReportsSkippedRules(t *testing.T) {
	path := writeTempProfile(t, northwindVoice)
	text := "Leverage our cutting-edge workspace to utilize your content."

	raw, runErr := runVoiceRewrite(t, "--profile-file", path, "--input-text", text, "--json")
	require.NoError(t, runErr, "nothing failed: the tool did what it could")
	assert.Equal(t, ExitOK, ExitCode(nil, runErr))

	var out output.VoiceRewriteOutput
	require.NoError(t, json.Unmarshal([]byte(raw), &out), "voice rewrite must emit valid JSON: %s", raw)
	assert.Equal(t, text, out.Rewritten)
	assert.Nil(t, out.Changes)
	assert.Contains(t, raw, `"changes": null`)

	require.Len(t, out.Skipped, 3)
	var terms []string
	for _, s := range out.Skipped {
		terms = append(terms, s.Term)
		assert.Equal(t, profile.RewriteSkipNoReplacement, s.Reason)
		assert.Equal(t, profile.SeverityMajor, s.Severity)
		assert.Equal(t, "forbidden", s.List)
	}
	assert.Equal(t, []string{"utilize", "leverage", "cutting-edge"}, terms)
}

func TestVoiceRewrite_MixedProfileChangesAndSkips(t *testing.T) {
	path := writeTempProfile(t, mixedVoice)

	raw, runErr := runVoiceRewrite(t, "--profile-file", path,
		"--input-text", "Leverage Globex to utilize your content.", "--json")
	require.NoError(t, runErr)

	var out output.VoiceRewriteOutput
	require.NoError(t, json.Unmarshal([]byte(raw), &out), raw)
	assert.Equal(t, "Leverage Globex to use your content.", out.Rewritten)
	assert.Equal(t, []output.VoiceChange{{From: "utilize", To: "use", Count: 1}}, out.Changes)
	require.Len(t, out.Skipped, 2)
	assert.Equal(t, "leverage", out.Skipped[0].Term)
	assert.Equal(t, profile.SeverityMinor, out.Skipped[0].Severity)
	assert.Equal(t, "Globex", out.Skipped[1].Term)
	assert.Equal(t, "competitor", out.Skipped[1].List)
	assert.Equal(t, profile.SeverityCritical, out.Skipped[1].Severity)
}

func TestVoiceRewrite_HumanOutputSummarisesSkipped(t *testing.T) {
	path := writeTempProfile(t, northwindVoice)

	raw, runErr := runVoiceRewrite(t, "--profile-file", path,
		"--input-text", "Leverage our cutting-edge workspace to utilize your content.")
	require.NoError(t, runErr)
	assert.Contains(t, raw, "3 violation(s) found, not rewritten: utilize, leverage, cutting-edge")
	assert.Contains(t, raw, `"leverage" [major/forbidden]: no replacement in the profile`)
	assert.Contains(t, raw, "verify with 'kapi voice check'")
}

func TestVoiceRewrite_CleanTextReportsNothing(t *testing.T) {
	path := writeTempProfile(t, northwindVoice)

	raw, runErr := runVoiceRewrite(t, "--profile-file", path, "--input-text", "Use the workspace.", "--json")
	require.NoError(t, runErr)
	var out output.VoiceRewriteOutput
	require.NoError(t, json.Unmarshal([]byte(raw), &out), raw)
	assert.Nil(t, out.Changes)
	assert.Nil(t, out.Skipped)

	human, runErr := runVoiceRewrite(t, "--profile-file", path, "--input-text", "Use the workspace.")
	require.NoError(t, runErr)
	assert.Equal(t, "Use the workspace.\n", human)
}
