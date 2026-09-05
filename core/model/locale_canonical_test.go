package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalLocale(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{"en", "en"},
		{"EN", "en"},
		{"en_US", "en-US"},
		{"en-us", "en-US"},
		{"NB-no", "nb-NO"},
		{"en_US.UTF-8", "en-US"},
		{"nb@bokmal", "nb"},
		{" pt-br ", "pt-BR"},
		{"sr-latn-rs", "sr-Latn-RS"},
		{"zh-hans", "zh-Hans"},
		{"qps-ploc", "qps-Ploc"},
		{"qps-Ploc", "qps-Ploc"},
		{"en-US-x-test", "en-US-x-test"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := CanonicalLocale(tc.in)
			require.NoError(t, err)
			assert.Equal(t, LocaleID(tc.want), got)
		})
	}
}

func TestCanonicalLocale_RejectsWhatIsNotALocale(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "   ", "xx-YY", "!!!", "not a locale"} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			_, err := CanonicalLocale(in)
			require.Error(t, err)
		})
	}
}

func TestNormalizeLocale(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   LocaleID
		want LocaleID
	}{
		{"", ""},
		{"en", "en"},
		{"en_US", "en-US"},
		{"EN-us", "en-US"},
		{"nb_NO", "nb-NO"},
		{"en_US.UTF-8", "en-US"},
		{"qps-ploc", "qps-Ploc"},
		{"iw", "he"},
		// Not a locale: passed through, so a store lookup misses rather than
		// erroring and a caller can still see what it was handed.
		{"xx-YY", "xx-YY"},
		{"!!!", "!!!"},
	}
	for _, tc := range tests {
		t.Run(string(tc.in), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, NormalizeLocale(tc.in))
			// Memoized answers agree with computed ones.
			assert.Equal(t, tc.want, NormalizeLocale(tc.in))
		})
	}
}

func TestNormalizeLocale_AgreesWithCanonical(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"en_US", "NB-no", "en_US.UTF-8", "qps-ploc", "sr-latn-rs", "pt-br"} {
		canon, err := CanonicalLocale(in)
		require.NoError(t, err)
		assert.Equal(t, canon, NormalizeLocale(LocaleID(in)), "the lenient form returns what the strict form does for a locale")
	}
}
