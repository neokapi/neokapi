package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTermRuleRenderings(t *testing.T) {
	rule := TermRule{
		Term:             "alert",
		Replacement:      " varsel ",
		ReplacementForms: []string{"varsler"},
		Accepted: []Rendering{
			{Text: "alarm", Forms: []string{"alarmer"}},
			{Text: "Varsel"},
			{Text: " "},
		},
	}
	assert.Equal(t, []Rendering{
		{Text: "varsel", Forms: []string{"varsler"}},
		{Text: "alarm", Forms: []string{"alarmer"}},
	}, rule.Renderings())

	assert.Nil(t, TermRule{Term: "kapi", DoNotTranslate: true}.Renderings(), "a rule with no replacement requires nothing")
}

func TestCloneCopiesRenderings(t *testing.T) {
	p := &VoiceProfile{Vocabulary: VocabularyRules{ForbiddenTerms: []TermRule{{
		Term:             "alert",
		Replacement:      "varsel",
		ReplacementForms: []string{"varsler"},
		Accepted:         []Rendering{{Text: "alarm", Forms: []string{"alarmer"}}},
	}}}}
	c := p.Clone()
	c.Vocabulary.ForbiddenTerms[0].ReplacementForms[0] = "changed"
	c.Vocabulary.ForbiddenTerms[0].Accepted[0].Text = "changed"
	c.Vocabulary.ForbiddenTerms[0].Accepted[0].Forms[0] = "changed"

	orig := p.Vocabulary.ForbiddenTerms[0]
	assert.Equal(t, []string{"varsler"}, orig.ReplacementForms)
	assert.Equal(t, "alarm", orig.Accepted[0].Text)
	assert.Equal(t, []string{"alarmer"}, orig.Accepted[0].Forms)
}
