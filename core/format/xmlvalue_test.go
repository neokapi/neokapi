package format_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CheckXMLBlock covers every text a writer can emit for a block: the source,
// each translation, and a translation filed under the empty locale, which the
// Qt TS reader files for a file with no language attribute read with no
// source locale and the TS writer writes.
func TestCheckXMLBlock(t *testing.T) {
	tests := []struct {
		name       string
		build      func() *model.Block
		wantLocale model.LocaleID
		wantErr    bool
	}{
		{
			name:  "clean block",
			build: func() *model.Block { return translated("Hello", "fr", "Bonjour") },
		},
		{
			name:    "source",
			build:   func() *model.Block { return translated("Hal\x01lo", "fr", "Bonjour") },
			wantErr: true,
		},
		{
			name:       "translation",
			build:      func() *model.Block { return translated("Hello", "fr", "Bon\x01jour") },
			wantLocale: "fr",
			wantErr:    true,
		},
		{
			name: "same-language translation",
			build: func() *model.Block {
				b := translated("Colour", "en", "Col\x01or")
				b.SourceLocale = "en"
				return b
			},
			wantLocale: "en",
			wantErr:    true,
		},
		{
			name:    "translation under the empty locale",
			build:   func() *model.Block { return translated("Hello", "", "Hal\x01lo") },
			wantErr: true,
		},
		{
			name: "translation under the empty locale beside a source locale",
			build: func() *model.Block {
				b := translated("Hello", "", "Hal\x01lo")
				b.SourceLocale = "en"
				return b
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := format.CheckXMLBlock("ts", tt.build())
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			var uv *format.UnrepresentableValueError
			require.ErrorAs(t, err, &uv)
			assert.Equal(t, tt.wantLocale, uv.Locale)
		})
	}
}

func translated(source string, locale model.LocaleID, target string) *model.Block {
	b := model.NewBlock("tu1", source)
	b.SetTargetText(locale, target)
	return b
}
