package locale

import (
	"testing"

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

	// Should have a reasonable number of locales
	assert.GreaterOrEqual(t, len(locales), 40)
	assert.LessOrEqual(t, len(locales), 60)

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
}
