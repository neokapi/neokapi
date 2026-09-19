package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
)

// Quoted JSON in CLI help is program syntax, so a word inside one is not a use
// of that term.
//
// This is the half a narrower reading of a placeholder would break. Comparing a
// source against its target reads {"reason":"findings"} as prose and says
// nothing about it, which is what stops a translated word inside a JSON literal
// being reported as a dropped placeholder. Masking has to keep the opposite
// reading, overwriting the literal so "findings" inside it never fires a term
// rule. The two readings live beside each other in core/check; this holds the
// masking one to account through the tool a person runs.
func TestTermCheckTool_TermsInsideAJSONLiteralAreNotTerms(t *testing.T) {
	t.Parallel()
	rules := []coreprofile.TermRule{
		{Term: "findings", Replacement: "funn"},
		{Term: "decision", Replacement: "beslutning"},
	}
	tests := []struct {
		name       string
		source     string
		target     string
		wantPassed string
	}{
		{
			name:       "a term inside a JSON literal does not fire a rule",
			source:     `emits {"decision":"block","reason":"findings"} when a gate fails`,
			target:     `sender ut {"decision":"block","reason":"findings"} når en port feiler`,
			wantPassed: "true",
		},
		{
			name:       "a JSON key is not a use of the term either",
			source:     `the {"decision": "block"} payload`,
			target:     `nyttelasten {"decision": "block"}`,
			wantPassed: "true",
		},
		{
			name:       "the same word in the prose beside it fires",
			source:     "reports the findings when a gate fails",
			target:     "rapporterer findings når en port feiler",
			wantPassed: "false",
		},
		{
			name:       "a use in prose is satisfied while the literal is kept verbatim",
			source:     `reports the findings and emits {"reason":"findings"}`,
			target:     `rapporterer funnene og sender ut {"reason":"findings"}`,
			wantPassed: "true",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tl := tools.NewTermCheckTool(&tools.TermCheckConfig{TermRules: rules, TargetLocale: "nb"})
			block := model.NewBlock("tu1", tt.source)
			block.SetTargetText("nb", tt.target)
			result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})

			got := result.Resource.(*model.Block)
			assert.Equal(t, tt.wantPassed, got.Properties[tools.PropTermCheckPassed],
				"errors: %s", got.Properties[tools.PropTermCheckErrors])
		})
	}
}
