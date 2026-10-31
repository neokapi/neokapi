package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// redactionRecipe declares a policy at every level a recipe can: the
// defaults, a named collection, one of its items, a bare entry, and an item
// that switches redaction off.
const redactionRecipe = `version: v1
name: redaction
defaults:
  source_language: en
  redaction:
    enabled: true
    rules: default.yaml
collections:
  - name: legal
    redaction:
      enabled: true
      rules: legal.yaml
      detectors: [rules]
    content:
      - path: "legal/*.md"
      - path: "legal/contracts/*.md"
        redaction:
          enabled: true
          rules: contracts.yaml
  - path: "hr/*.json"
    format: json
    redaction:
      enabled: true
      rules: hr.yaml
  - name: public
    content:
      - path: "public/*.md"
        redaction:
          enabled: false
  - name: docs
    content:
      - path: "docs/*.md"
`

func TestRedactionForPath_TheMostSpecificDeclarationGoverns(t *testing.T) {
	p := loadRecipe(t, redactionRecipe)

	for _, tc := range []struct {
		path, rules, at string
	}{
		{"legal/terms.md", "legal.yaml", "collections[0].redaction"},
		{"legal/contracts/nda.md", "contracts.yaml", "collections[0].content[1].redaction"},
		{"hr/staff.json", "hr.yaml", "collections[1].redaction"},
		{"docs/a.md", "default.yaml", "defaults.redaction"},
		{"elsewhere.md", "default.yaml", "defaults.redaction"},
		{"", "default.yaml", "defaults.redaction"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			got := p.RedactionForPath(tc.path)
			require.NotNil(t, got.Spec)
			assert.Equal(t, tc.rules, got.Spec.Rules)
			assert.Equal(t, tc.at, got.At)
		})
	}

	t.Run("a declaration replaces the defaults whole", func(t *testing.T) {
		got := p.RedactionForPath("legal/contracts/nda.md")
		assert.Empty(t, got.Spec.Detectors, "the item's policy carries no detectors of the collection's")
	})

	t.Run("a declaration switched off governs its files with none", func(t *testing.T) {
		got := p.RedactionForPath("public/index.md")
		assert.Nil(t, got.Spec)
		assert.Equal(t, "collections[2].content[0].redaction", got.At)
	})

	t.Run("the spec is a copy", func(t *testing.T) {
		got := p.RedactionForPath("legal/terms.md")
		got.Spec.Detectors[0] = "entities"
		assert.Equal(t, []string{"rules"}, p.Collections[0].Redaction.Detectors)
	})
}

func TestDeclaredRedactions_ListsEveryPolicySwitchedOnInRecipeOrder(t *testing.T) {
	p := loadRecipe(t, redactionRecipe)
	var at []string
	for _, r := range p.DeclaredRedactions() {
		at = append(at, r.At)
	}
	assert.Equal(t, []string{
		"defaults.redaction",
		"collections[0].redaction",
		"collections[0].content[1].redaction",
		"collections[1].redaction",
	}, at)
	assert.True(t, p.DeclaresRedaction())

	only := loadRecipe(t, `version: v1
name: only-a-collection
defaults:
  source_language: en
collections:
  - name: legal
    redaction:
      enabled: true
      rules: legal.yaml
    content:
      - path: "legal/*.md"
  - name: docs
    content:
      - path: "docs/*.md"
`)
	assert.True(t, only.DeclaresRedaction(), "a policy on one collection is a declared policy")
	assert.Nil(t, only.RedactionForPath("docs/a.md").Spec, "a file outside that collection has none")
	assert.Nil(t, (*KapiProject)(nil).RedactionForPath("docs/a.md").Spec)
	assert.False(t, (&KapiProject{}).DeclaresRedaction())
}

func TestValidate_ACollectionsRedactionDetectors(t *testing.T) {
	var p KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(`version: v1
name: bad
defaults:
  source_language: en
collections:
  - name: legal
    redaction:
      enabled: true
      detectors: [regex]
    content:
      - path: "legal/*.md"
`), &p))
	require.ErrorContains(t, p.Validate(), `collections[0]: redaction: unknown detector "regex"`)
}
