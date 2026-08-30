package locale

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonical(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want model.LocaleID
	}{
		// The style the decision exists for: POSIX in, BCP-47 out.
		{"posix separator", "nb_NO", "nb-NO"},
		{"posix lowercase region", "nb_no", "nb-NO"},
		{"posix with codeset", "en_US.UTF-8", "en-US"},
		{"posix with modifier", "nb@bokmal", "nb"},
		{"posix with both", "en_US.UTF-8@euro", "en-US"},

		// Case is a style too.
		{"upper language", "EN", "en"},
		{"mixed case", "NB-no", "nb-NO"},
		{"lower script", "zh-hans", "zh-Hans"},

		// Already canonical stays put.
		{"bcp47 language", "en", "en"},
		{"bcp47 with region", "nb-NO", "nb-NO"},
		{"bcp47 with script", "zh-Hans", "zh-Hans"},
		{"numeric region", "es-419", "es-419"},
		{"three letter language", "fil", "fil"},
		{"script and region", "zh-Hant-HK", "zh-Hant-HK"},
		{"variant", "ca-ES-valencia", "ca-ES-valencia"},
		{"private use", "en-x-pseudo", "en-x-pseudo"},

		// Surrounding whitespace is a transcription artifact, not a locale.
		{"padded", "  nb-NO  ", "nb-NO"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Canonical(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// A subtag CLDR does not know is still a subtag. Canonicalizing "qps-Ploc" to
// "qps" would drop the part that says which pseudo-locale it is, so the tag is
// kept whole and only its shape is squared.
func TestCanonicalKeepsWellFormedUnknownSubtags(t *testing.T) {
	cases := []struct {
		in   string
		want model.LocaleID
	}{
		{"qps-Ploc", "qps-Ploc"},
		{"qps-ploc", "qps-Ploc"},
		{"qps_ploc", "qps-Ploc"},
		{"qps-plocm", "qps-plocm"},
		{"qps", "qps"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := Canonical(tc.in)
			require.NoError(t, err, "a pseudo-locale is a locale")
			assert.Equal(t, tc.want, got)
		})
	}
}

// The gate exists for the typo. A tag whose PRIMARY subtag names no language is
// rejected, however well-formed it looks.
func TestCanonicalRejects(t *testing.T) {
	for _, in := range []string{
		"", "   ", "!!!", "e", "toolongtagname",
		"xx", "xx-YY", "xx_YY", "not a locale",
	} {
		t.Run(in, func(t *testing.T) {
			_, err := Canonical(in)
			require.Error(t, err, "%q is not a locale", in)
			assert.Contains(t, err.Error(), "invalid locale")
		})
	}
}

func TestCanonicalAll(t *testing.T) {
	got, err := CanonicalAll([]model.LocaleID{"nb_NO", "EN", "pt_BR"})
	require.NoError(t, err)
	assert.Equal(t, []model.LocaleID{"nb-NO", "en", "pt-BR"}, got)

	_, err = CanonicalAll([]model.LocaleID{"nb_NO", "xx_YY"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "xx_YY")

	empty, err := CanonicalAll(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// Normalize is the lenient sibling and stays lenient: it is used where a
// missing answer is better than a failed lookup. Recorded here so the two are
// not confused for each other.
func TestNormalizeStaysLenient(t *testing.T) {
	assert.Equal(t, model.LocaleID("nb-NO"), Normalize("nb_NO"),
		"x/text accepts the POSIX separator, so Normalize already canonicalizes it")
	assert.Equal(t, model.LocaleID("xx_YY"), Normalize("xx_YY"),
		"Normalize passes an unparseable tag through; Canonical is the gate")
}
