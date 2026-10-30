package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/venue"
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

// referenceRunCases are terms before, after and between character references
// in a block's runs, one a reference spells an edge of, and a reference that
// spells a letter inside a word.
var referenceRunCases = []struct {
	name    string
	runs    []model.Run
	hits    int
	spelled bool
}{
	{"before a reference", []model.Run{model.TextR("utilize"), entityRun("1", "&amp;"), model.TextR("more")}, 1, false},
	{"after a reference", []model.Run{model.TextR("word"), entityRun("1", "&nbsp;"), model.TextR("next")}, 1, false},
	{"between references", []model.Run{entityRun("1", "&amp;"), model.TextR("utilize"), entityRun("2", "&nbsp;")}, 1, false},
	{"a reference spells the last letter", []model.Run{model.TextR("caf"), entityRun("1", "&eacute;")}, 1, true},
	{"a reference joins no words", []model.Run{model.TextR("utiliz"), entityRun("1", "&#101;"), model.TextR("d")}, 0, false},
}

// A draft's voice score finds a term through the target's runs, as `kapi
// check` finds it, and a term a reference spells an edge of is scored with no
// position.
func TestDraftVoiceFindingsReadCharacterReferences(t *testing.T) {
	profile := (&coreprofile.VoiceProfile{ID: "p"}).Carry("pack references", referenceRules)
	sets := coreprofile.CarriedRuleSets(profile)
	for _, tc := range referenceRunCases {
		t.Run(tc.name, func(t *testing.T) {
			text := model.RunsText(tc.runs)
			require.Len(t, coreprofile.MatchTermRulesInRuns(sets, text, tc.runs), tc.hits)

			findings := draftVoiceFindings(sets, profile, text, tc.runs)
			require.Len(t, findings, tc.hits)
			if tc.hits == 1 {
				assert.Equal(t, tc.spelled, findings[0].Metadata[check.MetaSpelled] != "")
				assert.Equal(t, tc.spelled, findings[0].Position.IsZero())
			}
		})
	}
}

// The sweep's voice trap reads a block's source through its runs, so a term
// beside a reference makes the block a trap.
func TestDeriveSweepFixtures_VoiceTrapReadsCharacterReferences(t *testing.T) {
	profile := (&coreprofile.VoiceProfile{ID: "p", Name: "P"}).Carry("pack references", referenceRules)
	for _, tc := range referenceRunCases {
		t.Run(tc.name, func(t *testing.T) {
			b := &model.Block{ID: "b1", Translatable: true}
			b.SetSourceRuns(tc.runs)
			fixtures := DeriveSweepFixtures([]*venue.StoredBlock{{Block: b}}, &SweepContext{Profile: profile})
			if tc.hits == 0 {
				assert.Empty(t, fixtures)
				return
			}
			require.Len(t, fixtures, 1)
			assert.Equal(t, "voice", fixtures[0].Trap)
		})
	}
}

// A model's output is text alone, so the sweep's voice bar reads a reference
// written in it as the character it spells, as core/profile's plain form does.
func TestSweepVoiceAdherentReadsCharacterReferences(t *testing.T) {
	profile := (&coreprofile.VoiceProfile{ID: "p", Name: "P"}).Carry("pack references", referenceRules)
	for _, text := range []string{"utilize&amp;more", "word&nbsp;next", "&amp;utilize&nbsp;", "caf&eacute;"} {
		t.Run(text, func(t *testing.T) {
			require.Len(t, coreprofile.MatchTermRulesInText(coreprofile.CarriedRuleSets(profile), text), 1)
			assert.True(t, sweepVoiceAdherent("clean words", profile))
			assert.False(t, sweepVoiceAdherent(text, profile))
		})
	}
}
