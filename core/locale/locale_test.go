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
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, string(Normalize(model.LocaleID(tt.input))))
		})
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
