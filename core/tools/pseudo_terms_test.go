package tools

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A product name is the same string in every locale, so a probe that mangles it
// reports a bug in every sentence that mentions one. The terms store has said
// "Never translate" about kapi, neokapi and Bowrain since it was written; these
// cover the tool acting on it.

func dntRules(terms ...string) []profile.TermRule {
	rules := make([]profile.TermRule, 0, len(terms))
	for _, t := range terms {
		rules = append(rules, profile.TermRule{Term: t, DoNotTranslate: true})
	}
	return rules
}

func protectedText(runs []model.Run) (protected, translated []string) {
	for _, r := range runs {
		if r.Text == nil {
			continue
		}
		if r.Text.NoTranslate {
			protected = append(protected, r.Text.Text)
		} else {
			translated = append(translated, r.Text.Text)
		}
	}
	return protected, translated
}

func TestProtectTermsKeepsProductNames(t *testing.T) {
	// Longest first is the tool's job, not the caller's: dntTerms sorts.
	cfg := &PseudoConfig{TermRules: dntRules("kapi", "neokapi", "Bowrain", "kapi-desktop")}
	terms := dntTerms(cfg)

	cases := []struct {
		name      string
		in        string
		protected []string
	}{
		{"prose", "Install kapi to begin", []string{"kapi"}},
		{"several names", "Bowrain runs neokapi", []string{"Bowrain", "neokapi"}},
		// The docs navbar writes "Kapi"; both spellings name the product.
		{"other case", "Open Kapi Desktop", []string{"Kapi"}},
		{"upper case", "KAPI in a heading", []string{"KAPI"}},
		// "kapi" sits inside "neokapi"; the longer term has to win.
		{"contained name", "the neokapi framework", []string{"neokapi"}},
		{"hyphenated name", "install kapi-desktop now", []string{"kapi-desktop"}},
		// A longer word that merely starts with a name is ordinary prose.
		{"not a whole word", "kapistry is not the product", nil},
		{"no name at all", "just some prose", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runs := protectTerms(tc.in, terms)
			assert.Equal(t, tc.in, model.RunsText(runs), "runs must rebuild the text")
			got, _ := protectedText(runs)
			assert.Equal(t, tc.protected, got)
		})
	}
}

func TestPseudoTranslateRunsLeavesProductNames(t *testing.T) {
	cfg := &PseudoConfig{}
	cfg.Reset()
	cfg.Prefix, cfg.Suffix = "", ""
	cfg.TermRules = dntRules("kapi", "Bowrain")

	out := pseudoTranslateRuns([]model.Run{
		{Text: &model.TextRun{Text: "Install kapi to use Bowrain today"}},
	}, cfg)

	protected, translated := protectedText(out)
	assert.Equal(t, []string{"kapi", "Bowrain"}, protected)
	for _, s := range translated {
		assert.NotContains(t, s, "Install", "the prose around a name still translates")
	}
	assert.Equal(t, "Install kapi to use Bowrain today", untranslatedOf(out),
		"only the accents differ; the protected words are byte-identical")
}

// untranslatedOf rebuilds the sentence with the accented pieces replaced by
// their run text, so the assertion above reads on the protected words alone.
func untranslatedOf(runs []model.Run) string {
	var out strings.Builder
	for _, r := range runs {
		if r.Text == nil {
			continue
		}
		if r.Text.NoTranslate {
			out.WriteString(r.Text.Text)
			continue
		}
		out.WriteString(unaccent(r.Text.Text))
	}
	return out.String()
}

func unaccent(s string) string {
	rev := map[rune]rune{}
	for a, b := range accentMap {
		rev[b] = a
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if orig, ok := rev[r]; ok {
			out = append(out, orig)
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// A rule with a replacement is a translation decision, and says nothing about a
// locale that is not a language. Only the do-not-translate ones are read here.
func TestDNTTermsIgnoresOrdinaryRules(t *testing.T) {
	cfg := &PseudoConfig{TermRules: []profile.TermRule{
		{Term: "engine", Replacement: "motor"},
		{Term: "kapi", DoNotTranslate: true},
		{Term: "", DoNotTranslate: true},
	}}
	require.Equal(t, []string{"kapi"}, dntTerms(cfg))
}

// Without a terms store nothing changes, which is what keeps `kapi
// pseudo-translate` on a loose file behaving as it always has.
func TestNoTermRulesLeavesEverythingTranslatable(t *testing.T) {
	runs := protectTerms("Install kapi to begin", dntTerms(&PseudoConfig{}))
	require.Len(t, runs, 1)
	assert.False(t, runs[0].Text.NoTranslate)
}

// A do-not-translate term is a string, and a string has no senses. "Okapi"
// names the upstream Java framework here and also an animal, and the matcher
// protects both — which is right for this project, where every occurrence is
// the framework, and is the thing to weigh before declaring a term that is also
// an ordinary word. The concept's definition is where the intended sense is
// recorded.
func TestProtectTermsCannotTellSensesApart(t *testing.T) {
	terms := dntTerms(&PseudoConfig{TermRules: dntRules("Okapi", "okapi-bridge")})

	// The framework, and the common noun beside it that should still translate.
	runs := protectTerms("the Okapi Framework", terms)
	got, rest := protectedText(runs)
	assert.Equal(t, []string{"Okapi"}, got)
	assert.Equal(t, []string{"the ", " Framework"}, rest)

	// Longest first: the bridge keeps its second half.
	runs = protectTerms("through okapi-bridge", terms)
	got, _ = protectedText(runs)
	assert.Equal(t, []string{"okapi-bridge"}, got)

	// And the animal is protected too, for want of a way to tell.
	runs = protectTerms("The okapi is a forest giraffe", terms)
	got, _ = protectedText(runs)
	assert.Equal(t, []string{"okapi"}, got)
}
