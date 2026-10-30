package host

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The terms store and a starter pack's terms can state the same rule. The
// answer states it once, with every wording to avoid on one line.
func TestSayThisNotThatStatesEachRuleOnce(t *testing.T) {
	hits := []ContextTermHit{
		{ConceptID: "c1", Term: "the Ledger", Locale: "en", Status: "forbidden", Discouraged: true, Replacement: "Fernwell Ledger", Definition: "The product."},
		{ConceptID: "c1", Term: "Fernwell Ledger", Locale: "en", Status: "preferred", Definition: "The product."},
	}
	voice := (&coreprofile.VoiceProfile{}).Carry("pack fernwell", []coreprofile.TermRule{
		{Replacement: "person", Note: "who uses the studio"},
		{Term: "the Ledger", Replacement: "Fernwell Ledger"},
		{Term: "Fernwell", Replacement: "Fernwell Ledger", Note: "never alone"},
		{Term: "seat", Replacement: "person"},
		{Term: "synergy"},
	})
	binding := []coreprofile.TermRule{{Term: "seat", Replacement: "person"}}

	rules, total := sayThisNotThat(hits, binding, voice, 0)
	require.Equal(t, 3, total)
	assert.Equal(t, ContextRule{Say: "Fernwell Ledger", Not: []string{"the Ledger", "Fernwell"}, Note: "The product.", Locale: "en", From: []string{"terms", "pack fernwell"}}, rules[0])
	assert.Equal(t, ContextRule{Say: "person", Not: []string{"seat"}, Note: "who uses the studio", From: []string{"workspace", "pack fernwell"}}, rules[1])
	assert.Equal(t, ContextRule{Not: []string{"synergy"}, From: []string{"pack fernwell"}}, rules[2])

	assert.Equal(t, `- Fernwell Ledger, not "the Ledger" or "Fernwell": The product.`, ruleLine(rules[0], false))
	assert.Equal(t, `- Avoid "synergy"`, ruleLine(rules[2], false))

	capped, total := sayThisNotThat(hits, binding, voice, 2)
	assert.Len(t, capped, 2)
	assert.Equal(t, 3, total)
}

// Two rules with one wording to use stay two lines when one of them is
// advisory, so each keeps its own outcome; marked alike, they share a line.
func TestSayThisNotThatKeepsAdvisoryRulesApart(t *testing.T) {
	hits := []ContextTermHit{
		{ConceptID: "use", Term: "utilize", Locale: "en", Status: "forbidden", Discouraged: true, Replacement: "use"},
		{ConceptID: "use", Term: "use", Locale: "en", Status: "preferred"},
		{ConceptID: "leverage", Term: "leverage", Locale: "en", Status: "forbidden", Discouraged: true, Replacement: "use", Advisory: true},
		{ConceptID: "employ", Term: "employ", Locale: "en", Status: "forbidden", Discouraged: true, Replacement: "use"},
	}
	rules, total := sayThisNotThat(hits, nil, nil, 0)
	require.Equal(t, 2, total)
	assert.Equal(t, ContextRule{Say: "use", Not: []string{"utilize", "employ"}, Locale: "en", From: []string{"terms"}}, rules[0])
	assert.Equal(t, ContextRule{Say: "use", Not: []string{"leverage"}, Locale: "en", From: []string{"terms"}, Advisory: true}, rules[1])
}

// A rule that only names the wording to use avoids nothing and has no outcome
// of its own, so it shares the line of an advisory rule with that wording,
// whichever arrives first.
func TestSayThisNotThatMergesABareWordingIntoAnAdvisoryLine(t *testing.T) {
	advisory := ContextTermHit{ConceptID: "cart", Term: "cart", Locale: "en", Status: "forbidden", Discouraged: true, Replacement: "shopping cart", Advisory: true}
	want := ContextRule{Say: "shopping cart", Not: []string{"cart"}, Locale: "en", From: []string{"terms", "voice house"}, Advisory: true}

	// The bare wording after the advisory rule.
	voice := (&coreprofile.VoiceProfile{}).Carry("voice house", []coreprofile.TermRule{{Replacement: "shopping cart"}})
	rules, total := sayThisNotThat([]ContextTermHit{advisory}, nil, voice, 0)
	require.Equal(t, 1, total)
	assert.Equal(t, want, rules[0])

	// The bare wording first: a workspace rule naming only the wording, then
	// the advisory rule from the voice file.
	binding := []coreprofile.TermRule{{Replacement: "shopping cart"}}
	voice = (&coreprofile.VoiceProfile{}).Carry("voice house", []coreprofile.TermRule{{Term: "cart", Replacement: "shopping cart", Advisory: true}})
	rules, total = sayThisNotThat(nil, binding, voice, 0)
	require.Equal(t, 1, total)
	assert.Equal(t, ContextRule{Say: "shopping cart", Not: []string{"cart"}, From: []string{"workspace", "voice house"}, Advisory: true}, rules[0])

	// A failing rule with the same wording after that still has its own line.
	voice = (&coreprofile.VoiceProfile{}).Carry("voice house", []coreprofile.TermRule{
		{Term: "cart", Replacement: "shopping cart", Advisory: true},
		{Term: "basket", Replacement: "shopping cart"},
	})
	rules, total = sayThisNotThat(nil, binding, voice, 0)
	require.Equal(t, 2, total)
	assert.Equal(t, []string{"basket"}, rules[1].Not)
	assert.False(t, rules[1].Advisory)
}

// A discouraged term standing in a concept of its own names its replacement
// in its note, and the answer reads it there as the check does.
func TestDiscouragedReplacementReadsTheNote(t *testing.T) {
	c := terms.Concept{ID: "leverage", Terms: []terms.Term{
		{Text: "leverage", Locale: "en", Status: model.TermForbidden, Note: terms.ReplacementNote("use")},
	}}
	assert.Equal(t, "use", discouragedReplacement(c, c.Terms[0]))

	c.Terms = append(c.Terms, terms.Term{Text: "rely on", Locale: "en", Status: model.TermPreferred})
	assert.Equal(t, "rely on", discouragedReplacement(c, c.Terms[0]), "the preferred term comes first")
}

// The text answer: the voice brief, one list of word rules, the suggestions
// nobody has established, and what to record. Nothing about how it was found.
func TestContextAnswerTextIsTaskShaped(t *testing.T) {
	voice := &coreprofile.VoiceProfile{
		Name:        "Fernwell",
		Description: "Plain, for studio owners who are not accountants.",
		Style:       coreprofile.StyleRules{SentenceLength: "short"},
	}
	voice.Carry("pack fernwell", []coreprofile.TermRule{{Term: "seat", Replacement: "person"}})
	res := &ContextAnswer{
		Point:      ContextPoint{Path: "docs/billing.md", Default: true},
		Scope:      ScopeProject,
		Provenance: &ContextProvenance{Project: "prj_x", Revision: 3},
		VoiceBrief: coreprofile.RenderVoiceBrief(voice),
		Voice:      &ContextVoice{Name: "Fernwell"},
		Suggestions: []ContextSuggestion{
			{Kind: string(contextop.SubjectNote), Text: "Quickcast is the forecast feature, one word", SuggestedBy: "claude-code",
				Evidence: []contextop.Evidence{{Path: "README.md", Quote: "Quickcast"}}},
			{Kind: string(contextop.SubjectTerm), Term: "seat", Replacement: "person", SuggestedBy: "codex"},
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
		"\nNothing else is recorded for this file. "+recordingAdvice+"\n",
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
	notice := ContextFilesNotice{Files: []string{".kapi/terms.tbx"}, Command: "kapi store import"}
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
