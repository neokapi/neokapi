package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
// written in a tool call's text. A tool call's text is read as written, as
// `kapi check` reads a block of one text run, so "caf&eacute;" holds no "café".
var referenceCases = []struct {
	text      string
	matched   string // the finding's words, empty for none
	corrected string // the text with every correction applied
}{
	{"utilize&amp;more", "utilize", "use&amp;more"},
	{"word&nbsp;next", "next", "word&nbsp;following"},
	{"&amp;utilize&nbsp;", "utilize", "&amp;use&nbsp;"},
	{"caf&eacute;", "", "caf&eacute;"},
}

// The scoring and check tools find a term in a tool call's text as
// core/profile's plain form finds it.
func TestVoiceFindingsReadCharacterReferences(t *testing.T) {
	ms := referenceServer()
	profile, err := ms.voiceStore.GetProfile(t.Context(), "hexR")
	require.NoError(t, err)
	sets := coreprofile.CarriedRuleSets(profile)
	for _, tc := range referenceCases {
		t.Run(tc.text, func(t *testing.T) {
			want := coreprofile.MatchTermRulesInText(sets, tc.text)
			findings, err := ms.voiceFindings(t.Context(), profile, "en", tc.text)
			require.NoError(t, err)
			require.Len(t, findings, len(want))
			if tc.matched == "" {
				assert.Empty(t, findings)
				return
			}
			require.Len(t, findings, 1)
			assert.Equal(t, tc.matched, findings[0].OriginalText)
			assert.False(t, findings[0].Position.IsZero())
		})
	}
}

// suggest_corrections and rewrite_in_voice substitute a term beside a
// reference and keep the reference.
func TestCorrectionsBesideCharacterReferences(t *testing.T) {
	ms := referenceServer()
	for _, tc := range referenceCases {
		t.Run(tc.text, func(t *testing.T) {
			_, suggested, err := ms.handleSuggestCorrections(t.Context(), nil, suggestCorrectionsInput{ProfileID: "hexR", Text: tc.text})
			require.NoError(t, err)
			assert.Equal(t, tc.corrected, suggested.Corrected)
			if tc.matched == "" {
				assert.Empty(t, suggested.Corrections)
			} else {
				require.Len(t, suggested.Corrections, 1)
				assert.Equal(t, tc.matched, suggested.Corrections[0].Original)
			}

			_, rewritten, err := ms.handleRewriteInVoice(t.Context(), nil, rewriteInVoiceInput{ProfileID: "hexR", Text: tc.text})
			require.NoError(t, err)
			assert.Equal(t, tc.corrected, rewritten.Rewritten)
			assert.Empty(t, rewritten.Skipped)
		})
	}
}
