package tools_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tools"
)

// A translation keeps a code span as written, so the span sits in the source
// and in the target. In neither does it change what the terms ask: the source's
// code owes no rendering, and the target's code neither uses a term nor misses
// one. The prose around it is held to the terms as always.
func TestTermCheckTool_CodeSpanOnBothSides(t *testing.T) {
	t.Parallel()
	rules := []coreprofile.TermRule{
		{Term: "check", Replacement: "kontroll"},
		{Term: "kapi", DoNotTranslate: true, CaseSensitive: new(true)},
	}
	tests := []struct {
		name       string
		source     string
		target     string
		wantPassed string
		wantErrors int
	}{
		{
			name:       "the code span alone",
			source:     "Run `kapi check` first.",
			target:     "Kjør `kapi check` først.",
			wantPassed: "true",
		},
		{
			name:       "a quoted command alone",
			source:     "A hook runs 'kapi check --staged'.",
			target:     "En krok kjører 'kapi check --staged'.",
			wantPassed: "true",
		},
		{
			name:       "the prose use rendered beside the code span",
			source:     "Run `kapi check`, then fix each check.",
			target:     "Kjør `kapi check`, og rett hver kontroll.",
			wantPassed: "true",
		},
		{
			name:       "the prose use not rendered beside the code span",
			source:     "Run `kapi check`, then fix each check.",
			target:     "Kjør `kapi check`, og rett hver sjekk.",
			wantPassed: "false",
			wantErrors: 1,
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
			errs := got.Properties[tools.PropTermCheckErrors]
			assert.Equal(t, tt.wantPassed, got.Properties[tools.PropTermCheckPassed], "errors: %s", errs)
			if tt.wantErrors == 0 {
				assert.Empty(t, errs)
				return
			}
			found := strings.Split(errs, "; ")
			assert.Len(t, found, tt.wantErrors, "only the prose use is demanded: %s", errs)
			assert.Contains(t, errs, `"kontroll"`)
		})
	}
}
