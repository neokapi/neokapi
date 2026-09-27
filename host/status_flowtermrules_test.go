package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// A flow step's own term_rules hold that step while the flow runs, and the
// ship gate reads the terms bound where content sits. A locale only the flow's
// rules cover is ungoverned at the gate, and `kapi status` says where the
// flow's rules apply and how to bring them to the gate.
func TestStatus_UngovernedLocaleNamesTheFlowTermRules(t *testing.T) {
	dir := t.TempDir()
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
name: flowrules
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - path: en.json
    target: "{lang}.json"
flows:
  plain:
    steps:
      - tool: translate
        config:
          provider: demo
  converge:
    steps:
      - tool: translate
        config:
          provider: demo
          term_rules:
            - term: billing period
              replacement: période de facturation
`), 0o644))
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, []string{"converge"}, flowsCarryingTermRules(proj),
		"a flow whose steps carry no term_rules is left out")

	out := StatusOutput{
		Locales:       []LocaleCoverage{{Locale: "fr", NotGoverned: []string{"terms"}}},
		flowTermRules: flowsCarryingTermRules(proj),
	}
	var b strings.Builder
	out.writeBasisLines(&b)
	text := b.String()
	assert.Contains(t, text, "No terms govern fr: no terms bound where its content sits have a term for it, "+
		"so terminology is not a bar to shipping there.")
	assert.Contains(t, text, `The term_rules in flow "converge" apply to the steps that carry them, when that flow runs.`)
	assert.Contains(t, text, "add them to the project's terms (`kapi terms import`)")

	// Without flow rules the ungoverned line stands alone.
	b.Reset()
	StatusOutput{Locales: out.Locales}.writeBasisLines(&b)
	assert.NotContains(t, b.String(), "term_rules")
}
