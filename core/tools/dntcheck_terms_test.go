package tools

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
)

// The check's whole behaviour is its term list, and for this repo that list was
// empty: the recipe names none, so `kapi check --dnt` passed everything while
// the terms store had said "never translate" about the product names all along.
// A guardrail that cannot fail reports reassurance, which is worse than no
// guardrail.

func TestEffectiveTermsUnionsRecipeAndStore(t *testing.T) {
	cfg := NewDNTCheckConfig(model.LocaleID("nb"))
	cfg.Terms = []string{"Acme"}
	cfg.TermRules = []profile.TermRule{
		{Term: "kapi", DoNotTranslate: true},
		{Term: "Bowrain", DoNotTranslate: true},
		// An ordinary terminology rule is a translation decision, not a
		// do-not-translate claim.
		{Term: "engine", Replacement: "motor"},
	}

	assert.Equal(t, []string{"Acme", "kapi", "Bowrain"}, cfg.EffectiveTerms())
}

func TestEffectiveTermsDeduplicates(t *testing.T) {
	cfg := NewDNTCheckConfig(model.LocaleID("nb"))
	cfg.Terms = []string{"kapi"}
	cfg.TermRules = []profile.TermRule{{Term: "kapi", DoNotTranslate: true}}

	assert.Equal(t, []string{"kapi"}, cfg.EffectiveTerms(),
		"a term declared in both places is checked once")
}

// With neither source the list is empty, and the check has nothing to say. That
// is the state this repo was in.
func TestEffectiveTermsEmptyWithoutEither(t *testing.T) {
	cfg := NewDNTCheckConfig(model.LocaleID("nb"))
	assert.Empty(t, cfg.EffectiveTerms())
}

func TestEffectiveTermsIgnoresBlankRules(t *testing.T) {
	cfg := NewDNTCheckConfig(model.LocaleID("nb"))
	cfg.TermRules = []profile.TermRule{
		{Term: "", DoNotTranslate: true},
		{Term: "kapi", DoNotTranslate: true},
	}
	assert.Equal(t, []string{"kapi"}, cfg.EffectiveTerms())
}
