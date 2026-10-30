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
        - term: leverage
        - term: cutting-edge
`

const mixedVoice = `name: Mixed
tone:
    formality: neutral
vocabulary:
    forbidden_terms:
        - term: utilize
          replacement: use
        - term: leverage
          advisory: true
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
		assert.True(t, s.Fails)
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
	assert.False(t, out.Skipped[0].Fails)
	assert.Equal(t, "Globex", out.Skipped[1].Term)
	assert.Equal(t, "competitor", out.Skipped[1].List)
	assert.True(t, out.Skipped[1].Fails)
}

func TestVoiceRewrite_HumanOutputSummarisesSkipped(t *testing.T) {
	path := writeTempProfile(t, northwindVoice)

	raw, runErr := runVoiceRewrite(t, "--profile-file", path,
		"--input-text", "Leverage our cutting-edge workspace to utilize your content.")
	require.NoError(t, runErr)
	assert.Contains(t, raw, "3 violation(s) found, not rewritten: utilize, leverage, cutting-edge")
	assert.Contains(t, raw, `"leverage" [fails/forbidden]: no replacement in the profile`)
	assert.Contains(t, raw, "verify with 'kapi voice check'")
}

// A term beside a character reference is rewritten and the reference kept; a
// term a reference spells an edge of is left in place and reported with its
// reason, as the page shows it.
func TestVoiceRewrite_CharacterReferences(t *testing.T) {
	path := writeTempProfile(t, `name: References
tone:
    formality: neutral
vocabulary:
    forbidden_terms:
        - term: utilize
          replacement: use
        - term: next
          replacement: following
        - term: café
          replacement: coffee shop
`)
	cases := []struct {
		text, rewritten, skipped string
	}{
		{"utilize&amp;more", "use&amp;more", ""},
		{"word&nbsp;next", "word&nbsp;following", ""},
		{"&amp;utilize&nbsp;", "&amp;use&nbsp;", ""},
		{"caf&eacute;", "caf&eacute;", "café"},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			raw, runErr := runVoiceRewrite(t, "--profile-file", path, "--input-text", tc.text, "--json")
			require.NoError(t, runErr)
			var out output.VoiceRewriteOutput
			require.NoError(t, json.Unmarshal([]byte(raw), &out), raw)
			assert.Equal(t, tc.rewritten, out.Rewritten)
			if tc.skipped == "" {
				assert.Empty(t, out.Skipped)
				return
			}
			require.Len(t, out.Skipped, 1)
			assert.Equal(t, profile.RewriteSkipCharacterReference, out.Skipped[0].Reason)
			assert.Equal(t, []string{tc.skipped}, out.Skipped[0].Matched)

			human, runErr := runVoiceRewrite(t, "--profile-file", path, "--input-text", tc.text)
			require.NoError(t, runErr)
			assert.Contains(t, human, `"café" [fails/forbidden]: written with a character reference`)
		})
	}
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
