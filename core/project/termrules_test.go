package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/profile"
)

func loadRulesRecipe(t *testing.T, dir, body string) *KapiProject {
	t.Helper()
	path := filepath.Join(dir, RecipeFileName)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	p, err := Load(path)
	require.NoError(t, err)
	return p
}

// Every place a recipe writes `term_rules:` contributes: flow steps inline and
// in flows_dir, parallel branches, and the project-wide and per-language tool
// presets. A rule declared twice is kept once, and two different rules for one
// term are both kept.
func TestDeclaredTermRules_EveryLocation(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "flows"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "flows", "review.yaml"), []byte(`steps:
  - tool: term-check
    config:
      term_rules:
        - term: invoice
          replacement: facture
`), 0o644))
	// Shadowed by the inline flow of the same name, as a run resolves it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "flows", "converge.yaml"), []byte(`steps:
  - tool: term-check
    config:
      term_rules:
        - term: shadowed
          replacement: ombre
`), 0o644))

	p := loadRulesRecipe(t, dir, `version: v1
name: rules
flows_dir: flows
defaults:
  source_language: en
  target_languages: [fr, de]
  tools:
    recycle:
      term_rules:
        - term: account
          replacement: compte
  locales:
    de:
      tools:
        translate:
          term_rules:
            - term: billing period
              replacement: Abrechnungszeitraum
collections:
  - path: en.json
    target: "{lang}.json"
flows:
  converge:
    steps:
      - tool: translate
        config:
          term_rules: &rules
            - term: billing period
              replacement: période de facturation
            - term: reading
              replacement: relevé
              advisory: true
      - parallel:
          - tool: term-check
            config:
              term_rules: *rules
          - tool: term-check
            config:
              term_rules:
                - term: reading
                  replacement: relevé
`)
	d, err := p.DeclaredTermRules(dir)
	require.NoError(t, err)

	assert.Equal(t, []profile.TermRule{
		{Term: "billing period", Replacement: "période de facturation"},
		{Term: "reading", Replacement: "relevé", Advisory: true},
		{Term: "reading", Replacement: "relevé"},
		{Term: "invoice", Replacement: "facture"},
		{Term: "account", Replacement: "compte"},
	}, d.All)
	assert.Equal(t, map[string][]profile.TermRule{
		"de": {{Term: "billing period", Replacement: "Abrechnungszeitraum"}},
	}, d.ByLocale)

	assert.Len(t, d.For("fr"), 5)
	assert.Equal(t, profile.TermRule{Term: "billing period", Replacement: "Abrechnungszeitraum"}, d.For("de")[5])

	// Without the recipe's directory the flow files are not read.
	inline, err := p.DeclaredTermRules("")
	require.NoError(t, err)
	assert.Len(t, inline.All, 4)
}

func TestDeclaredTermRules_None(t *testing.T) {
	p := loadRulesRecipe(t, t.TempDir(), "version: v1\nname: none\ncollections:\n  - path: en.json\n")
	d, err := p.DeclaredTermRules("")
	require.NoError(t, err)
	assert.True(t, d.Empty())
	enc, err := d.Encode()
	require.NoError(t, err)
	assert.Empty(t, enc)

	var nilProj *KapiProject
	d, err = nilProj.DeclaredTermRules("")
	require.NoError(t, err)
	assert.True(t, d.Empty())
}

// A term_rules value that is not a list of rules names where it sits.
func TestDeclaredTermRules_Malformed(t *testing.T) {
	p := &KapiProject{Defaults: Defaults{Tools: map[string]map[string]any{
		"term-check": {"term_rules": "billing period"},
	}}}
	_, err := p.DeclaredTermRules("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "defaults.tools.term-check.term_rules")
}
