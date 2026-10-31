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

// A redaction declaration that names rules, detectors or a placeholder
// without `enabled: true` reads as a policy and redacts nothing. It is
// refused at load at every scope, by the same check and with the same
// message; `enabled: false` alone is not refused.
func TestValidate_ARedactionPolicyMustBeSwitchedOn(t *testing.T) {
	const instead = " without enabled: true, so it redacts nothing. Write enabled: true to apply the policy, or enabled: false with no other key to switch redaction off"
	load := func(defaults, scoped string) error {
		var p KapiProject
		require.NoError(t, yaml.Unmarshal([]byte(`version: v1
name: scoped
defaults:
  source_language: en
`+defaults+`collections:
`+scoped), &p))
		return p.Validate()
	}
	require.EqualError(t, load("", "  - path: \"hr/*.json\"\n    redaction:\n      rules: hr.yaml\n"),
		"collections[0].redaction sets rules"+instead, "a bare entry's rules without enabled")
	require.EqualError(t, load("", "  - name: legal\n    redaction:\n      detectors: [rules]\n    content:\n      - path: \"legal/*.md\"\n"),
		"collections[0].redaction sets detectors"+instead)
	require.EqualError(t, load("", "  - name: legal\n    content:\n      - path: \"legal/*.md\"\n        redaction:\n          enabled: false\n          placeholder: \"[X]\"\n"),
		"collections[0].content[0].redaction sets placeholder"+instead, "an item's placeholder under enabled: false")
	require.EqualError(t, load("  redaction:\n    rules: default.yaml\n", "  - path: \"docs/*.md\"\n"),
		"defaults.redaction sets rules"+instead, "the defaults are held to the same rule")
	require.EqualError(t, load("  redaction:\n    enabled: false\n    rules: default.yaml\n    detectors: [rules, entities]\n", "  - path: \"docs/*.md\"\n"),
		"defaults.redaction sets rules and detectors"+instead, "enabled: false beside other keys")

	require.NoError(t, load("", "  - name: public\n    redaction:\n      enabled: false\n    content:\n      - path: \"public/*.md\"\n"),
		"enabled: false alone switches the collection off")
	require.NoError(t, load("  redaction:\n    enabled: false\n", "  - path: \"docs/*.md\"\n"),
		"enabled: false alone switches the defaults off")
	require.NoError(t, load("  redaction:\n    enabled: true\n    rules: default.yaml\n", "  - path: \"docs/*.md\"\n"),
		"the corrected defaults load")
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
	require.ErrorContains(t, p.Validate(), `collections[0].redaction: unknown detector "regex"`)
}
