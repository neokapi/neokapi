package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// referenceRules are the word rules the character-reference cases are checked
// under.
var referenceRules = []coreprofile.TermRule{
	{Term: "utilize", Replacement: "use"},
	{Term: "next", Replacement: "following"},
	{Term: "café", Replacement: "coffee shop"},
}

// entityRun is a character reference as the HTML reader keeps it.
func entityRun(id, data string) model.Run {
	return model.PhR(model.PlaceholderRun{ID: id, Type: "code:entity", Data: data})
}

// The check endpoints find a term in a request's text as core/profile's plain
// form finds it: the text is read as written, as `kapi check` reads a block of
// one text run, so "caf&eacute;" holds no "café".
func TestVoiceGateFindingsReadCharacterReferences(t *testing.T) {
	s := &Server{}
	profile := (&coreprofile.VoiceProfile{ID: "p"}).Carry("pack references", referenceRules)
	cases := []struct {
		text    string
		matched string // empty for no finding
	}{
		{"utilize&amp;more", "utilize"},
		{"word&nbsp;next", "next"},
		{"&amp;utilize&nbsp;", "utilize"},
		{"caf&eacute;", ""},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			want := coreprofile.MatchTermRulesInText(coreprofile.CarriedRuleSets(profile), tc.text)
			findings, err := s.voiceGateFindings(t.Context(), "", profile, "en", tc.text)
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

// A stored target is matched through its runs, as `kapi check` matches it,
// both by the recheck oracle for a promoted rule and by the voice half of the
// term-compliance gate. A term a reference spells an edge of is a use.
func TestTargetsMatchThroughTheirRuns(t *testing.T) {
	sets := []coreprofile.TermRuleSet{{Rules: referenceRules}}
	profile := (&coreprofile.VoiceProfile{ID: "p"}).Carry("pack references", referenceRules)
	cases := []struct {
		name string
		runs []model.Run
		uses bool
	}{
		{"before a reference", []model.Run{model.TextR("utilize"), entityRun("1", "&amp;"), model.TextR("more")}, true},
		{"after a reference", []model.Run{model.TextR("word"), entityRun("1", "&nbsp;"), model.TextR("next")}, true},
		{"between references", []model.Run{entityRun("1", "&amp;"), model.TextR("utilize"), entityRun("2", "&nbsp;")}, true},
		{"a reference spells the last letter", []model.Run{model.TextR("caf"), entityRun("1", "&eacute;")}, true},
		{"a reference joins no words", []model.Run{model.TextR("utiliz"), entityRun("1", "&#101;"), model.TextR("d")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &model.Block{ID: "b1", Translatable: true}
			b.SetSourceText("source")
			b.SetTargetRuns("fr", tc.runs)
			found := len(coreprofile.MatchTermRulesInRuns(sets, model.RunsText(tc.runs), tc.runs)) > 0
			require.Equal(t, tc.uses, found, "core/profile")

			assert.Equal(t, tc.uses, targetUsesRules(sets, b, "fr"))
			compliance := blockTermCompliance(t.Context(), b, "en", "fr", nil, profile)
			assert.Equal(t, tc.uses, compliance == store.TermComplianceViolation, "compliance = %v", compliance)
		})
	}
}
