package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
)

func TestClassifyEdit(t *testing.T) {
	t.Parallel()

	const sentence = "You must accept the terms before we can activate your account"

	tests := []struct {
		name          string
		prior, curren string
		want          tools.EditKind
	}{
		{"identical", sentence, sentence, tools.EditNone},

		// Cosmetic, and cosmetic at any length — which is the point. A score
		// puts these anywhere between 90 and 98 depending on how much text
		// surrounds them.
		{"a trailing period", "Get started", "Get started.", tools.EditCosmetic},
		{"a trailing period, long", sentence, sentence + ".", tools.EditCosmetic},
		{"a comma", sentence, "You must accept the terms, before we can activate your account", tools.EditCosmetic},
		{"capitalisation", "Get started", "Get Started", tools.EditCosmetic},
		{"extra spacing", "Get started", "Get  started", tools.EditCosmetic},
		{"a curly apostrophe", "Don't stop", "Don’t stop", tools.EditCosmetic},
		{"straight to curly quotes", `Say "hello"`, "Say “hello”", tools.EditCosmetic},
		{"a hyphen appears", "account setup", "account set-up", tools.EditCosmetic},
		{"a hyphen becomes a dash", "2019-2020", "2019—2020", tools.EditCosmetic},

		// Substantive, and substantive at any length. The first is the case a
		// 95% fill floor gets exactly wrong.
		{"a negation added", sentence, "You must not accept the terms before we can activate your account", tools.EditSubstantive},
		{"a number changed", "Cancel within 30 days", "Cancel within 3 days", tools.EditSubstantive},
		{"a word swapped", "Click the button", "Click the link", tools.EditSubstantive},
		{"a word added", "account setup", "account setup today", tools.EditSubstantive},
		{"a possessive added", "account setup", "account setup's", tools.EditSubstantive},
		{"reordered", "Click the button below", "Below, click the button", tools.EditSubstantive},

		// The disguise the word boundary exists to catch: stripping punctuation
		// without it makes these one text.
		{"not able is not notable", "not able", "notable", tools.EditSubstantive},
		{"a word split", "signup", "sign up", tools.EditSubstantive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tools.ClassifyEdit(tt.prior, tt.curren))
		})
	}
}

func TestEditKindSafeToFill(t *testing.T) {
	t.Parallel()

	assert.True(t, tools.EditNone.SafeToFill())
	assert.True(t, tools.EditCosmetic.SafeToFill(), "the words did not move")
	assert.False(t, tools.EditSubstantive.SafeToFill(),
		"and no amount of surrounding text makes a moved word safe")
}

// TestClassifyEditDoesNotSoftenWithLength is the property a score cannot have.
//
// The same edit, in a two-word string and in a paragraph, is the same kind.
// Under a percentage it is 91 in one and 98 in the other, which is how a single
// fill floor ends up refusing the harmless case and accepting the dangerous one.
func TestClassifyEditDoesNotSoftenWithLength(t *testing.T) {
	t.Parallel()

	short := "Get started"
	long := "Get started with your account today by choosing the plan that suits your team best"

	assert.Equal(t, tools.EditCosmetic, tools.ClassifyEdit(short, short+"."))
	assert.Equal(t, tools.EditCosmetic, tools.ClassifyEdit(long, long+"."))

	assert.Equal(t, tools.EditSubstantive, tools.ClassifyEdit(short, "Get started now"))
	assert.Equal(t, tools.EditSubstantive, tools.ClassifyEdit(long, long+" now"))
}
