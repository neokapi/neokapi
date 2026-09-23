package host

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The terms store and the voice vocabulary can state the same rule. The
// answer states it once, with every wording to avoid on one line.
func TestSayThisNotThatStatesEachRuleOnce(t *testing.T) {
	hits := []ContextTermHit{
		{ConceptID: "c1", Term: "the Ledger", Locale: "en", Status: "forbidden", Discouraged: true, Replacement: "Fernwell Ledger", Definition: "The product."},
		{ConceptID: "c1", Term: "Fernwell Ledger", Locale: "en", Status: "preferred", Definition: "The product."},
	}
	voice := &coreprofile.VoiceProfile{Vocabulary: coreprofile.VocabularyRules{
		ForbiddenTerms: []coreprofile.TermRule{
			{Term: "the Ledger", Replacement: "Fernwell Ledger"},
			{Term: "Fernwell", Replacement: "Fernwell Ledger", Note: "never alone"},
			{Term: "seat", Replacement: "person"},
			{Term: "synergy"},
		},
		PreferredTerms: []coreprofile.TermRule{{Term: "person", Note: "who uses the studio"}},
	}}
	binding := []coreprofile.TermRule{{Term: "seat", Replacement: "person"}}

	rules, total := sayThisNotThat(hits, binding, voice, 0)
	require.Equal(t, 3, total)
	assert.Equal(t, ContextRule{Say: "Fernwell Ledger", Not: []string{"the Ledger", "Fernwell"}, Note: "The product.", Locale: "en", From: []string{"terms", "voice"}}, rules[0])
	assert.Equal(t, ContextRule{Say: "person", Not: []string{"seat"}, Note: "who uses the studio", From: []string{"workspace", "voice"}}, rules[1])
	assert.Equal(t, ContextRule{Not: []string{"synergy"}, From: []string{"voice"}}, rules[2])

	assert.Equal(t, `- Fernwell Ledger, not "the Ledger" or "Fernwell": The product.`, ruleLine(rules[0], false))
	assert.Equal(t, `- Avoid "synergy"`, ruleLine(rules[2], false))

	capped, total := sayThisNotThat(hits, binding, voice, 2)
	assert.Len(t, capped, 2)
	assert.Equal(t, 3, total)
}

// The text answer: the voice brief, one list of word rules, the suggestions
// nobody has established, and what to record. Nothing about how it was found.
func TestContextAnswerTextIsTaskShaped(t *testing.T) {
	voice := &coreprofile.VoiceProfile{
		Name:        "Fernwell",
		Description: "Plain, for studio owners who are not accountants.",
		Style:       coreprofile.StyleRules{SentenceLength: "short"},
		Vocabulary:  coreprofile.VocabularyRules{ForbiddenTerms: []coreprofile.TermRule{{Term: "seat", Replacement: "person"}}},
	}
	res := &ContextAnswer{
		Point:      ContextPoint{Path: "docs/billing.md", Default: true},
		Scope:      ScopeProject,
		Provenance: &ContextProvenance{Project: "prj_x", Revision: 3},
		VoiceBrief: coreprofile.RenderVoiceBrief(voice),
		Voice:      &ContextVoice{Name: "Fernwell"},
		Candidates: []ContextCandidate{
			{Kind: string(contextop.SubjectNote), Text: "Quickcast is the forecast feature, one word", ProposedBy: "claude-code",
				Evidence: []contextop.Evidence{{Path: "README.md", Quote: "Quickcast"}}},
			{Kind: string(contextop.SubjectTerm), Term: "seat", Replacement: "person", ProposedBy: "codex"},
		},
		Notes: []string{"project scope: concept relations, revisions and market scoping live in a connected workspace"},
	}
	res.Rules, res.RulesTotal = sayThisNotThat(nil, nil, voice, 0)

	var buf bytes.Buffer
	require.NoError(t, res.FormatText(&buf))
	assert.Equal(t, "# Writing docs/billing.md\n"+
		"\nVoice: Fernwell. Plain, for studio owners who are not accountants. Short sentences.\n"+
		"\nSay this, not that:\n- person, not \"seat\"\n"+
		"\nSuggested, not yet established:\n- Quickcast is the forecast feature, one word (claude-code, seen in README.md)\n"+
		"\nNothing else is recorded for this file. If you notice a name or spelling the project keeps to, "+
		"record it with context_observe (or `kapi context observe`).\n",
		buf.String())

	res.Explain()
	buf.Reset()
	require.NoError(t, res.FormatText(&buf))
	assert.Contains(t, buf.String(), "## How this was answered")
	assert.Contains(t, buf.String(), "workspace revision 3")
	assert.Contains(t, buf.String(), "- project scope: concept relations")
}

// What a person or an agent must act on leads the text: context files nothing
// has read in, a voice that would not load.
func TestContextAnswerTextLeadsWithWhatNeedsAction(t *testing.T) {
	notice := ContextFilesNotice{Files: []string{".kapi/terms.tbx"}, Command: "kapi context import"}
	res, err := ResolveContextAt(t.Context(), ContextPointSources{
		Path:     "docs/a.md",
		Unread:   &notice,
		VoiceErr: assert.AnError,
		Scope:    ScopeProject,
	}, ContextPointRequest{Path: "docs/a.md"})
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, res.FormatText(&buf))
	text := buf.String()
	assert.Contains(t, text, "\nThis project carries context files (.kapi/terms.tbx)")
	assert.Contains(t, text, "Ask the person to run it.\n")
	assert.Contains(t, text, "\nThe voice bound here could not be loaded: ")
	assert.Contains(t, text, "Nothing is recorded for this file yet.")
}
