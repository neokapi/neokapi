package tools_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tools"
)

var kapiName = coreprofile.TermRule{Term: "kapi", DoNotTranslate: true, ConceptID: "c-kapi"}

// TestTermCheck_DoNotTranslate holds a translation to a do-not-translate term: a
// source that uses the term needs it verbatim in the target.
func TestTermCheck_DoNotTranslate(t *testing.T) {
	bowrain := coreprofile.TermRule{Term: "Bowrain", Forms: []string{"Bowrains"}, DoNotTranslate: true}
	tests := []struct {
		name   string
		rule   coreprofile.TermRule
		source string
		target string
		pass   bool
	}{
		{"a translated term fails", kapiName, "Open kapi to begin.", "Åpne capi for å begynne.", false},
		{"a verbatim term passes", kapiName, "Open kapi to begin.", "Åpne kapi for å begynne.", true},
		{"the source's own casing passes", kapiName, "Kapi opens the project.", "Kapi åpner prosjektet.", true},
		{"another casing fails", kapiName, "Open kapi to begin.", "Åpne KAPI for å begynne.", false},
		{"a declared form is a use", bowrain, "Both Bowrains sync.", "Begge Bowreinene synkroniserer.", false},
		{"a term inside a placeholder is not a use", kapiName, "{kapi} is ready.", "{kapi} er klar.", true},
		{"an argument name does not keep the term", kapiName, "Open kapi with {kapi}.", "Åpne capi med {kapi}.", false},
		{"a source that does not use the term demands nothing", kapiName, "Open the project.", "Åpne prosjektet.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			passed, msg := termCheck(t, nb(tt.rule), tt.source, tt.target)
			assert.Equal(t, tt.pass, passed, msg)
			if !tt.pass {
				assert.Contains(t, msg, `do-not-translate term "`+tt.rule.Term+`"`)
				assert.NotContains(t, msg, "; ", "a finding never contains the separator between findings")
			}
		})
	}
}

// TestTermCheck_DoNotTranslateLeavesOtherRulesAlone is the control: beside a
// do-not-translate rule, an ordinary rule decides what it decides without one.
func TestTermCheck_DoNotTranslateLeavesOtherRulesAlone(t *testing.T) {
	save := coreprofile.TermRule{Term: "save", Replacement: "lagre"}
	for _, target := range []string{"Lagre i kapi.", "Spar i kapi."} {
		alone, aloneMsg := termCheck(t, nb(save), "Save in kapi.", target)
		beside, besideMsg := termCheck(t, nb(save, kapiName), "Save in kapi.", target)
		assert.Equal(t, alone, beside, target)
		assert.Equal(t, aloneMsg, besideMsg, target)
	}
}

// TestTermCheck_DoNotTranslateCanaries: a configuration holding only
// do-not-translate rules has a canary the tool flags, and a mixed configuration
// has one for each kind of rule.
func TestTermCheck_DoNotTranslateCanaries(t *testing.T) {
	cfg := nb(kapiName)
	canaries, uncheckable := tools.TermCheckCanaries(cfg)
	require.NotEmpty(t, canaries, uncheckable)
	tl := tools.NewTermCheckTool(cfg)
	for _, c := range canaries {
		result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: c.Block})
		assert.Equal(t, "false", result.Resource.(*model.Block).Properties[tools.PropTermCheckPassed], c.Name)
	}

	mixed, _ := tools.TermCheckCanaries(nb(coreprofile.TermRule{Term: "berth", Replacement: "kaiplass"}, kapiName))
	var names []string
	for _, c := range mixed {
		names = append(names, c.Name)
	}
	joined := strings.Join(names, " | ")
	assert.Contains(t, joined, `"berth"`)
	assert.Contains(t, joined, `"kapi"`)
}

// TestTermCheck_DoNotTranslateIsAValidRule: a do-not-translate rule names no
// replacement, and an ordinary rule still needs one.
func TestTermCheck_DoNotTranslateIsAValidRule(t *testing.T) {
	require.NoError(t, nb(kapiName).Validate())
	assert.Error(t, nb(coreprofile.TermRule{Term: "berth"}).Validate())
}

// TestTermCheckMatching_CountsDoNotTranslate: the matching the verdict records
// counts do-not-translate rules among the rules applied.
func TestTermCheckMatching_CountsDoNotTranslate(t *testing.T) {
	m := tools.TermCheckMatching(nb(coreprofile.TermRule{Term: "berth", Replacement: "kaiplass"}, kapiName))
	assert.Equal(t, 2, m.Rules)
}
