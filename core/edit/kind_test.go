package edit_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/edit"
	"github.com/stretchr/testify/assert"
)

func TestClassifyEdit(t *testing.T) {
	t.Parallel()

	const sentence = "You must accept the terms before we can activate your account"

	tests := []struct {
		name          string
		prior, curren string
		want          edit.Kind
	}{
		{"identical", sentence, sentence, edit.None},

		// Cosmetic, and cosmetic at any length — which is the point. A score
		// puts these anywhere between 90 and 98 depending on how much text
		// surrounds them.
		{"a trailing period", "Get started", "Get started.", edit.Cosmetic},
		{"a trailing period, long", sentence, sentence + ".", edit.Cosmetic},
		{"a comma", sentence, "You must accept the terms, before we can activate your account", edit.Cosmetic},
		{"capitalisation", "Get started", "Get Started", edit.Cosmetic},
		{"extra spacing", "Get started", "Get  started", edit.Cosmetic},
		{"a curly apostrophe", "Don't stop", "Don’t stop", edit.Cosmetic},
		{"straight to curly quotes", `Say "hello"`, "Say “hello”", edit.Cosmetic},
		{"a hyphen appears", "account setup", "account set-up", edit.Cosmetic},
		{"a hyphen becomes a dash", "2019-2020", "2019—2020", edit.Cosmetic},

		// Substantive, and substantive at any length. The first is the case a
		// 95% fill floor gets exactly wrong.
		{"a negation added", sentence, "You must not accept the terms before we can activate your account", edit.Substantive},
		{"a number changed", "Cancel within 30 days", "Cancel within 3 days", edit.Substantive},
		{"a word swapped", "Click the button", "Click the link", edit.Substantive},
		{"a word added", "account setup", "account setup today", edit.Substantive},
		{"a possessive added", "account setup", "account setup's", edit.Substantive},
		{"reordered", "Click the button below", "Below, click the button", edit.Substantive},

		// The disguise the word boundary exists to catch: stripping punctuation
		// without it makes these one text.
		{"not able is not notable", "not able", "notable", edit.Substantive},
		{"a word split", "signup", "sign up", edit.Substantive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, edit.Classify(tt.prior, tt.curren))
		})
	}
}

func TestEditKindSafeToFill(t *testing.T) {
	t.Parallel()

	assert.True(t, edit.None.SafeToFill())
	assert.True(t, edit.Cosmetic.SafeToFill(), "the words did not move")
	assert.False(t, edit.Substantive.SafeToFill(),
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

	assert.Equal(t, edit.Cosmetic, edit.Classify(short, short+"."))
	assert.Equal(t, edit.Cosmetic, edit.Classify(long, long+"."))

	assert.Equal(t, edit.Substantive, edit.Classify(short, "Get started now"))
	assert.Equal(t, edit.Substantive, edit.Classify(long, long+" now"))
}
