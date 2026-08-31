package locale

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Canonical and Parse are one normalization reached two ways: Canonical cleans
// the styles a person or an operating system writes, then hands the result to
// Parse. Two functions that each canonicalize a tag their own way is the defect
// this asserts against — a locale would key one way through an ingress boundary
// and another through a strict one, and nothing would report it.
func TestCanonicalAgreesWithParse(t *testing.T) {
	// Every tag Parse accepts, in the forms it accepts them.
	accepted := []string{
		"en", "nb", "nb-NO", "pt-BR", "pt-br", "NB-no", "en-Latn-US",
		"zh-Hans", "zh-CN", "ar", "ja", "de-AT", "qps",
	}

	for _, in := range accepted {
		t.Run(in, func(t *testing.T) {
			parsed, err := Parse(in)
			require.NoError(t, err, "fixture must be a tag Parse accepts")

			canonical, err := Canonical(in)
			require.NoError(t, err, "Canonical must accept everything Parse does")
			assert.Equal(t, parsed, canonical,
				"Canonical and Parse must return the same form for %q", in)
		})
	}
}

// What Canonical accepts and Parse does not, with the reason each is a locale.
// These are the ingress styles; a boundary that used Parse would turn them away.
func TestCanonicalAcceptsWhatIngressReceives(t *testing.T) {
	cases := map[string]string{
		"qps-Ploc":    "qps-Ploc", // a pseudo-locale: CLDR knows qps, not Ploc
		"en_US.UTF-8": "en-US",    // POSIX, with a codeset
		"nb@bokmal":   "nb",       // POSIX, with a modifier
		"nb_NO":       "nb-NO",    // POSIX separator
	}

	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			got, err := Canonical(in)
			require.NoError(t, err)
			assert.Equal(t, want, string(got))
		})
	}

	// Only the pseudo-locale is a tag Parse rejects on membership; the rest are
	// rejected on shape. Either way none of them may reach a store unconverted.
	_, err := Parse("qps-Ploc")
	require.Error(t, err, "Parse is the strict one, and this test says why that matters")
}

// A typo is not a locale in either function. Without this the gate is a
// formatter, and a misspelled target language becomes an identity nothing
// rejects.
func TestNeitherAcceptsALocaleThatNamesNothing(t *testing.T) {
	for _, in := range []string{"xx-YY", "not a locale", ""} {
		_, perr := Parse(in)
		require.Errorf(t, perr, "Parse(%q)", in)
		_, cerr := Canonical(in)
		require.Errorf(t, cerr, "Canonical(%q)", in)
	}
}
