package locale

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
		err   bool
	}{
		{"en", "en", false},
		{"fr", "fr", false},
		{"de", "de", false},
		{"pt-BR", "pt-BR", false},
		{"zh-Hans", "zh-Hans", false},
		{"zh-Hant", "zh-Hant", false},
		{"EN", "en", false},       // normalized casing
		{"FR-fr", "fr-FR", false}, // normalized
		{"", "", true},            // empty
		{"!!!", "", true},         // garbage
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tt.input)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, string(got))
			}
		})
	}
}

func TestMustParse(t *testing.T) {
	t.Parallel()
	assert.NotPanics(t, func() {
		id := MustParse("fr")
		assert.Equal(t, "fr", string(id))
	})

	assert.Panics(t, func() {
		MustParse("")
	})
}

func TestNormalize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"pt-BR", "pt-BR"}, // already canonical
		{"pt-br", "pt-BR"}, // region casing fixed
		{"PT-br", "pt-BR"}, // language + region casing fixed
		{"EN", "en"},       // language casing fixed
		{"fr-fr", "fr-FR"}, // region casing fixed
		{"", ""},           // empty passes through
		{"!!!", "!!!"},     // unparseable falls back to input
		{"en-US", "en-US"}, // region specificity preserved
		// The lenient form accepts every style Canonical does.
		{"nb_NO", "nb-NO"},
		{"en_US.UTF-8", "en-US"},
		{"nb@bokmal", "nb"},
		{"qps-ploc", "qps-Ploc"},
		{"xx-YY", "xx-YY"}, // not a locale: passed through, so a lookup misses
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, string(Normalize(model.LocaleID(tt.input))))
		})
	}
}

func TestMinimal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Region is the CLDR likely region → dropped.
		{"norwegian bokmal", "nb-NO", "nb"},
		{"french", "fr-FR", "fr"},
		{"german", "de-DE", "de"},
		{"japanese", "ja-JP", "ja"},
		{"italian", "it-IT", "it"},
		{"dutch", "nl-NL", "nl"},
		{"spanish", "es-ES", "es"},
		{"korean", "ko-KR", "ko"},
		{"finnish", "fi-FI", "fi"},
		{"swedish", "sv-SE", "sv"},
		{"english", "en-US", "en"},
		{"serbian implies cyrillic", "sr-RS", "sr"},
		{"casing normalized first", "NB-no", "nb"},

		// Region carries a real distinction → kept.
		{"british english", "en-GB", "en-GB"},
		{"canadian french", "fr-CA", "fr-CA"},
		{"latin american spanish", "es-419", "es-419"},
		{"mexican spanish", "es-MX", "es-MX"},
		{"austrian german", "de-AT", "de-AT"},
		{"nynorsk keeps nothing to drop", "nn", "nn"},

		// Portuguese always writes its region.
		{"brazilian portuguese", "pt-BR", "pt-BR"},
		{"european portuguese", "pt-PT", "pt-PT"},

		// Script subtags are never dropped.
		{"simplified chinese", "zh-Hans", "zh-Hans"},
		{"traditional chinese", "zh-Hant", "zh-Hant"},
		{"latin serbian", "sr-Latn", "sr-Latn"},
		{"redundant region after script", "zh-Hant-TW", "zh-Hant"},
		{"redundant region after script hans", "zh-Hans-CN", "zh-Hans"},
		{"redundant region after latin script", "sr-Latn-RS", "sr-Latn"},

		// Chinese region forms become script forms.
		{"mainland chinese", "zh-CN", "zh-Hans"},
		{"taiwan chinese", "zh-TW", "zh-Hant"},
		{"hong kong chinese kept", "zh-HK", "zh-HK"},
		{"singapore chinese kept", "zh-SG", "zh-SG"},
		{"bare chinese", "zh", "zh"},

		// Variants and extensions are left alone.
		{"variant", "ca-ES-valencia", "ca-ES-valencia"},
		{"extension", "en-US-u-ca-gregory", "en-US-u-ca-gregory"},

		// Degenerate input.
		{"empty", "", ""},
		{"unparseable", "!!!", "!!!"},
		{"posix C", "C", "C"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, string(Minimal(model.LocaleID(tt.input))))
		})
	}
}

// TestMinimalIsIdempotent guards the property callers rely on when they write
// a minimal tag back into configuration: minimizing an already-minimal tag
// must be a no-op, for every tag in the curated well-known list.
func TestMinimalIsIdempotent(t *testing.T) {
	t.Parallel()
	for _, w := range WellKnownLocales() {
		id := model.LocaleID(w.Code)
		once := Minimal(id)
		assert.Equal(t, string(once), string(Minimal(once)), "minimizing %q twice must be stable", w.Code)
	}
}

// TestWellKnownLocalesAreMinimal asserts the curated dropdown list is already
// in CLDR-minimal form, so the tag kapi suggests never carries a redundant
// region.
func TestWellKnownLocalesAreMinimal(t *testing.T) {
	t.Parallel()
	for _, w := range WellKnownLocales() {
		id := model.LocaleID(w.Code)
		assert.Equal(t, w.Code, string(Minimal(id)), "well-known locale %q is not CLDR-minimal", w.Code)
	}
}

func TestFallbacks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"already minimal", "nb", []string{"nb"}},
		{"redundant region", "nb-NO", []string{"nb-NO", "nb"}},
		{"distinct region", "en-GB", []string{"en-GB", "en"}},
		{"source english", "en-US", []string{"en-US", "en"}},
		{"portuguese keeps region then falls back", "pt-BR", []string{"pt-BR", "pt"}},
		{"script form", "zh-Hant", []string{"zh-Hant", "zh"}},
		{"script plus region", "zh-Hant-TW", []string{"zh-Hant-TW", "zh-Hant", "zh"}},
		{"region promoted to script", "zh-CN", []string{"zh-CN", "zh-Hans", "zh"}},
		{"region kept then script then base", "zh-HK", []string{"zh-HK", "zh"}},
		{"un-normalized input", "NB-no", []string{"nb-NO", "nb"}},
		{"empty", "", nil},
		{"unparseable", "!!!", []string{"!!!"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Fallbacks(model.LocaleID(tt.input))
			as := make([]string, 0, len(got))
			for _, g := range got {
				as = append(as, string(g))
			}
			if tt.want == nil {
				assert.Empty(t, as)
				return
			}
			assert.Equal(t, tt.want, as)
		})
	}
}

// TestFallbacksStartsWithCanonicalTag pins the exact-first contract: whatever
// the caller wrote is tried verbatim (canonicalized) before any fallback, so a
// resource keyed by the specific tag always wins.
func TestFallbacksStartsWithCanonicalTag(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"nb-NO", "en-GB", "pt-BR", "zh-Hant-TW", "fr"} {
		got := Fallbacks(model.LocaleID(in))
		require.NotEmpty(t, got)
		assert.Equal(t, string(Normalize(model.LocaleID(in))), string(got[0]))
	}
}

func TestDisplayName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code string
		want string
	}{
		{"en", "English"},
		{"fr", "French"},
		{"de", "German"},
		{"ja", "Japanese"},
		{"pt-BR", "Brazilian Portuguese"},
		{"zh-Hans", "Simplified Chinese"},
		{"zh-Hant", "Traditional Chinese"},
		{"ko", "Korean"},
		{"ar", "Arabic"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			t.Parallel()
			got := DisplayName(MustParse(tt.code))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWellKnownLocales(t *testing.T) {
	t.Parallel()
	locales := WellKnownLocales()

	// Should have a reasonable number of locales (broad global coverage).
	assert.GreaterOrEqual(t, len(locales), 60)
	assert.LessOrEqual(t, len(locales), 120)

	// No duplicate codes.
	seen := make(map[string]bool, len(locales))
	for _, l := range locales {
		assert.Falsef(t, seen[l.Code], "duplicate locale code %q", l.Code)
		seen[l.Code] = true
	}

	// Should be sorted by display name
	for i := 1; i < len(locales); i++ {
		assert.LessOrEqual(t, locales[i-1].DisplayName, locales[i].DisplayName,
			"expected sorted by display name: %s <= %s", locales[i-1].DisplayName, locales[i].DisplayName)
	}

	// Should contain some well-known locales
	codeSet := make(map[string]bool)
	for _, l := range locales {
		codeSet[l.Code] = true
		assert.NotEmpty(t, l.DisplayName)
		assert.NotEmpty(t, l.Code)
	}
	assert.True(t, codeSet["en"], "should contain English")
	assert.True(t, codeSet["fr"], "should contain French")
	assert.True(t, codeSet["de"], "should contain German")
	assert.True(t, codeSet["ja"], "should contain Japanese")
	assert.True(t, codeSet["pt-BR"], "should contain Brazilian Portuguese")
	// Commercial regional variants localization teams request.
	assert.True(t, codeSet["es-419"], "should contain Latin American Spanish")
	assert.True(t, codeSet["fr-CA"], "should contain Canadian French")
	assert.True(t, codeSet["en-GB"], "should contain British English")
	// High-population languages that are commonly under-served.
	assert.True(t, codeSet["fil"], "should contain Filipino")
	assert.True(t, codeSet["pa"], "should contain Punjabi")
}

func TestToPosix(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "pt_BR", ToPosix("pt-BR"))
	assert.Equal(t, "en", ToPosix("en"))
	assert.Equal(t, "zh_Hans", ToPosix("zh-Hans"))
	assert.Equal(t, "nb_NO", ToPosix("nb-NO"))
}

func TestFromPosix(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "pt-BR", FromPosix("pt_BR"))
	assert.Equal(t, "en", FromPosix("en"))
	assert.Equal(t, "zh-Hans", FromPosix("zh_Hans"))
	assert.Equal(t, "nb-NO", FromPosix("nb_NO"))
}

func TestFormatCode(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "pt-BR", FormatCode("pt-BR", FormatBCP47))
	assert.Equal(t, "pt_BR", FormatCode("pt-BR", FormatPOSIX))
	assert.Equal(t, "en", FormatCode("en", FormatPOSIX))
	assert.Equal(t, "en", FormatCode("en", ""))
}

func TestWellKnownLocalesFormatted(t *testing.T) {
	t.Parallel()
	posix := WellKnownLocalesFormatted(FormatPOSIX)
	codeSet := make(map[string]bool)
	for _, l := range posix {
		codeSet[l.Code] = true
	}
	assert.True(t, codeSet["pt_BR"], "POSIX format should use underscore")
	assert.True(t, codeSet["zh_Hans"], "POSIX format should use underscore")
	assert.True(t, codeSet["en"], "language-only codes unchanged")
}
