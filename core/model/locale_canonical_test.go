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

// A canonical form is its own canonical form, so a locale normalized twice
// keys the same target and the same row as one normalized once. x/text reads
// some tags it accepts into a form that it then reads differently: a repeated
// extension singleton loses one repeated subtag on each read, and a subtag it
// accepted inside one extension is unknown once the extensions are reordered.
func TestNormalizeLocale_Idempotent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   LocaleID
		want LocaleID
	}{
		{"en-u-en-en-t-nu", "en-t-NU-u-EN"},
		{"AA-u-01-01-u-00-00", "aa-u-01-u-00"},
		{"en-u-en-en-u-ca-ca", "en-u-en-u-ca"},
		{"en-u-ca-ca-u-ca-ca-u-ca-ca-u-ca-ca", "en-u-ca-u-ca-u-ca-u-ca"},
		{"en-u-01-01-u-00-00", "en-u-01-u-00"},
		// x/text rejects what it made of this one on the second read, so the
		// first read is the canonical form.
		{"AA-u-01-01-u-00-000-00", "aa-u-01-u-00-000-00"},
		// The casing fallback writes a byte that is not UTF-8 as U+FFFD, which
		// lengthens the subtag and so changes how the next read cases it.
		{"aa-t-BB-AA-0A-\x800", "aa-t-BB-AA-0A-\ufffd\ufffd\ufffd0"},
		{"en-t-ja-u-ca-japanese", "en-t-ja-u-ca-japanese"},
		{"en_US.UTF-8", "en-US"},
		{"qps-ploc", "qps-Ploc"},
		{"xx-YY", "xx-YY"},
	}
	for _, tc := range tests {
		t.Run(string(tc.in), func(t *testing.T) {
			t.Parallel()
			once := NormalizeLocale(tc.in)
			assert.Equal(t, tc.want, once)
			assert.Equal(t, once, NormalizeLocale(once), "normalizing the canonical form changes it")
		})
	}
}

func FuzzNormalizeLocale(f *testing.F) {
	for _, s := range []string{
		"en", "en_US.UTF-8", "nb@bokmal", "qps-ploc", "xx-YY", "sr-latn-rs",
		"en-t-ja-u-ca-japanese", "en-u-en-en-t-nu", "AA-u-01-01-u-00-00",
		"AA-u-01-01-u-00-000-00", "en-u-ca-ca-u-ca-ca-u-ca-ca", "aa-t-BB-AA-0A-\x800",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		once := NormalizeLocale(LocaleID(s))
		if twice := NormalizeLocale(once); twice != once {
			t.Fatalf("NormalizeLocale(%q) = %q, and NormalizeLocale(%q) = %q", s, once, once, twice)
		}
		canon, err := CanonicalLocale(s)
		if err != nil {
			return
		}
		if again, err := CanonicalLocale(string(canon)); err == nil && again != canon {
			t.Fatalf("CanonicalLocale(%q) = %q, and CanonicalLocale(%q) = %q", s, canon, canon, again)
		}
	})
}
