package format_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestAuthoritativeRuns(t *testing.T) {
	tests := []struct {
		name  string
		build func() *model.Block
		want  string
	}{
		{
			name:  "a block with no language",
			build: func() *model.Block { return model.NewBlock("tu1", "Hello") },
			want:  "Hello",
		},
		{
			name: "a block with a translation",
			build: func() *model.Block {
				b := model.NewBlock("tu1", "Hello")
				b.SourceLocale = "en"
				b.SetTargetText("fr", "Bonjour")
				return b
			},
			want: "Hello",
		},
		{
			// A bilingual file from en to en files its target under the
			// source language; the edition the block was read in stays the
			// authoritative one.
			name: "a block with a same-language translation",
			build: func() *model.Block {
				b := model.NewBlock("tu1", "Colour")
				b.SourceLocale = "en"
				b.SetTargetText("en", "Color")
				return b
			},
			want: "Colour",
		},
		{
			name: "an edited block",
			build: func() *model.Block {
				b := model.NewBlock("tu1", "Hello")
				b.EditSourceText("Hi")
				return b
			},
			want: "Hi",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, model.RunsText(format.AuthoritativeRuns(tt.build())))
		})
	}
}

func TestAuthoritativeRuns_EmptySource(t *testing.T) {
	b := &model.Block{ID: "tu1"}
	assert.Empty(t, format.AuthoritativeRuns(b))
}

// A writer emits AuthoritativeRuns wherever it writes no target, so the runs
// are the block's source whatever its source locale says, including a tag
// that is not well-formed BCP-47 but that the locale gate accepts. A reader
// can copy such a tag from the document onto every block, and a project home
// stamps the project's source locale on every block before a write.
func TestAuthoritativeRuns_AnySourceLocale(t *testing.T) {
	for _, loc := range []model.LocaleID{
		"en", "en_US", "qps-ploc", "xx-YY",
		"en-u-en-en-t-nu", "AA-u-01-01-u-00-00", "en-u-en-en-u-ca-ca",
	} {
		t.Run(string(loc), func(t *testing.T) {
			for name, b := range sourceLocaleBlocks(loc) {
				assertAliasesSource(t, name, b)
			}
		})
	}
}

func FuzzAuthoritativeRuns(f *testing.F) {
	for _, s := range []string{"en", "en-u-en-en-t-nu", "AA-u-01-01-u-00-00", "AA-u-01-01-u-00-000-00", "xx-YY", "!!!"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for name, b := range sourceLocaleBlocks(model.LocaleID(s)) {
			assertAliasesSource(t, name, b)
		}
	})
}

// sourceLocaleBlocks returns blocks read in loc: one with no translation, one
// with a French translation and one with a translation filed under loc itself.
func sourceLocaleBlocks(loc model.LocaleID) map[string]*model.Block {
	plain := model.NewBlock("tu1", "Hello")
	plain.SourceLocale = loc
	translated := model.NewBlock("tu1", "Hello")
	translated.SourceLocale = loc
	translated.SetTargetText("fr", "Bonjour")
	sameLanguage := model.NewBlock("tu1", "Colour")
	sameLanguage.SourceLocale = loc
	sameLanguage.SetTargetText(loc, "Color")
	return map[string]*model.Block{"plain": plain, "translated": translated, "same-language": sameLanguage}
}

// assertAliasesSource asserts that AuthoritativeRuns returns the block's own
// source slice.
func assertAliasesSource(t *testing.T, name string, b *model.Block) {
	t.Helper()
	got, src := format.AuthoritativeRuns(b), b.SourceRuns()
	if len(got) != len(src) || (len(src) > 0 && &got[0] != &src[0]) {
		t.Fatalf("%s block with source locale %q: AuthoritativeRuns = %q, want the source %q",
			name, b.SourceLocale, model.RunsText(got), model.RunsText(src))
	}
}
