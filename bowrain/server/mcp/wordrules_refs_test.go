package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// referenceRules are the word rules the character-reference cases are checked
// under.
var referenceRules = []coreprofile.TermRule{
	{Term: "utilize", Replacement: "use"},
	{Term: "next", Replacement: "following"},
	{Term: "café", Replacement: "coffee shop"},
}

// referenceServer returns a server whose single profile "hexR" carries
// referenceRules and which holds no terms store.
func referenceServer() *MCPServer {
	p := (&coreprofile.VoiceProfile{ID: "hexR", Name: "References", Scope: "ws1"}).Carry("pack references", referenceRules)
	return &MCPServer{voiceStore: &memVoiceStore{profiles: []*coreprofile.VoiceProfile{p}}}
}

// referenceCases are terms before, after and between character references
// written in a tool call's text, and one a reference spells an edge of.
var referenceCases = []struct {
	text      string
	matched   string // the finding's words
	spelled   bool   // a reference spells an edge of the words
	corrected string // the text with every correction applied
}{
	{"utilize&amp;more", "utilize", false, "use&amp;more"},
	{"word&nbsp;next", "next", false, "word&nbsp;following"},
	{"&amp;utilize&nbsp;", "utilize", false, "&amp;use&nbsp;"},
	{"caf&eacute;", "café", true, "caf&eacute;"},
}

// The scoring and check tools find a term in a tool call's text as
// core/profile's plain form finds it, and a term a reference spells an edge
// of is reported as the page shows it with no position.
func TestVoiceFindingsReadCharacterReferences(t *testing.T) {
	ms := referenceServer()
	profile, err := ms.voiceStore.GetProfile(t.Context(), "hexR")
	require.NoError(t, err)
	sets := coreprofile.CarriedRuleSets(profile)
	for _, tc := range referenceCases {
		t.Run(tc.text, func(t *testing.T) {
			want := coreprofile.MatchTermRulesInText(sets, tc.text)
			require.Len(t, want, 1)

			findings, err := ms.voiceFindings(t.Context(), profile, "en", tc.text)
			require.NoError(t, err)
			require.Len(t, findings, 1)
			assert.Equal(t, tc.matched, findings[0].OriginalText)
			assert.Equal(t, tc.spelled, findings[0].Metadata[check.MetaSpelled] != "")
			assert.Equal(t, tc.spelled, findings[0].Position.IsZero())
		})
	}
}

// suggest_corrections and rewrite_in_voice substitute a term beside a
// reference and keep the reference, and build no edit from a term a
// reference spells an edge of.
func TestCorrectionsBesideCharacterReferences(t *testing.T) {
	ms := referenceServer()
	for _, tc := range referenceCases {
		t.Run(tc.text, func(t *testing.T) {
			_, suggested, err := ms.handleSuggestCorrections(t.Context(), nil, suggestCorrectionsInput{ProfileID: "hexR", Text: tc.text})
			require.NoError(t, err)
			assert.Equal(t, tc.corrected, suggested.Corrected)
			if tc.spelled {
				assert.Empty(t, suggested.Corrections)
			} else {
				require.Len(t, suggested.Corrections, 1)
				assert.Equal(t, tc.matched, suggested.Corrections[0].Original)
			}

			_, rewritten, err := ms.handleRewriteInVoice(t.Context(), nil, rewriteInVoiceInput{ProfileID: "hexR", Text: tc.text})
			require.NoError(t, err)
			assert.Equal(t, tc.corrected, rewritten.Rewritten)
			if tc.spelled {
				require.Len(t, rewritten.Skipped, 1)
				assert.Equal(t, coreprofile.RewriteSkipCharacterReference, rewritten.Skipped[0].Reason)
				assert.Equal(t, []string{tc.matched}, rewritten.Skipped[0].Matched)
			}
		})
	}
}
