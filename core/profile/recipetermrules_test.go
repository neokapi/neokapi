package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecipeTermRules_EncodeRoundTrip(t *testing.T) {
	d := RecipeTermRules{
		All:      []TermRule{{Term: "reading", Replacement: "relevé", Advisory: true}},
		ByLocale: map[string][]TermRule{"de": {{Term: "reading", Replacement: "Ablesung"}}},
	}
	enc, err := d.Encode()
	require.NoError(t, err)
	again, err := d.Encode()
	require.NoError(t, err)
	assert.Equal(t, enc, again, "one declaration encodes to one string")

	back, err := DecodeRecipeTermRules(enc)
	require.NoError(t, err)
	assert.Equal(t, d, back)

	empty, err := DecodeRecipeTermRules("")
	require.NoError(t, err)
	assert.True(t, empty.Empty())
	enc, err = empty.Encode()
	require.NoError(t, err)
	assert.Empty(t, enc, "no rules encode to the empty setting")
	_, err = DecodeRecipeTermRules("{")
	assert.Error(t, err)
}

// For gives a language the rules every language takes and its own, each once.
func TestRecipeTermRules_For(t *testing.T) {
	period := TermRule{Term: "billing period", Replacement: "période de facturation"}
	de := TermRule{Term: "billing period", Replacement: "Abrechnungszeitraum"}
	d := RecipeTermRules{
		All:      []TermRule{period},
		ByLocale: map[string][]TermRule{"de": {de, period}},
	}
	assert.Equal(t, []TermRule{period}, d.For("fr"))
	assert.Equal(t, []TermRule{period, de}, d.For("de"))
}

// A recipe rule stands in for a stored rule on the same term, and every other
// stored rule stays.
func TestWithDeclaredTermRules(t *testing.T) {
	stored := []TermRule{
		{Term: "billing period", Replacement: "facturation"},
		{Term: "account", Replacement: "compte"},
	}
	declared := []TermRule{{Term: "billing period", Replacement: "période de facturation"}}
	assert.Equal(t, []TermRule{
		{Term: "account", Replacement: "compte"},
		{Term: "billing period", Replacement: "période de facturation"},
	}, WithDeclaredTermRules(stored, declared))
	assert.Equal(t, stored, WithDeclaredTermRules(stored, nil))
}

// Covers holds when every held rule is still declared, unchanged, for the
// same languages.
func TestRecipeTermRules_Covers(t *testing.T) {
	period := TermRule{Term: "billing period", Replacement: "période de facturation"}
	reading := TermRule{Term: "reading", Replacement: "relevé"}
	held := RecipeTermRules{All: []TermRule{period}}

	assert.True(t, RecipeTermRules{All: []TermRule{period, reading}}.Covers(held), "adding a rule")
	assert.True(t, held.Covers(RecipeTermRules{}), "anything covers no rules")
	assert.False(t, RecipeTermRules{All: []TermRule{reading}}.Covers(held), "removing a rule")

	advisory := period
	advisory.Advisory = true
	assert.False(t, RecipeTermRules{All: []TermRule{advisory}}.Covers(held), "making a rule advisory")

	assert.False(t, RecipeTermRules{ByLocale: map[string][]TermRule{"fr": {period}}}.Covers(held),
		"narrowing a rule to one language")
	assert.True(t, held.Covers(RecipeTermRules{ByLocale: map[string][]TermRule{"fr": {period}}}),
		"widening a rule to every language")
}
