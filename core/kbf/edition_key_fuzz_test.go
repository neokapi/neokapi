package kbf

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// FuzzEditionKeyText checks that every reader of the edition key text form
// reads back what the printer wrote, for arbitrary key parts: the lenient
// model.EditionKey.UnmarshalText, the strict ReadKey a bundle is read with,
// and the strict model.ParseEditionKey a command line or a change reference
// is read with. A strict reader refuses some keys outright (no language, or
// for ParseEditionKey a language that is not a locale); a key it accepts must
// come back unchanged.
func FuzzEditionKeyText(f *testing.F) {
	for _, s := range [][3]string{
		{"fr", "", ""}, {"nb", "formal", ""}, {"en", "casual", "short"},
		{";", "", ""}, {"50%", "", ""}, {"fr", "a%b", "c;d"}, {"fr", "a=b", ""},
		{"%3B", "%25", "%3D"}, {"", "x", ""},
	} {
		f.Add(s[0], s[1], s[2])
	}
	f.Fuzz(func(t *testing.T, locale, tone, channel string) {
		key := model.EditionKey{Locale: model.NormalizeLocale(model.LocaleID(locale)), Tone: tone, Channel: channel}
		raw, err := key.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText(%+v): %v", key, err)
		}
		text := string(raw)

		var lenient model.EditionKey
		if err := lenient.UnmarshalText(raw); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", text, err)
		}
		if lenient != key {
			t.Fatalf("UnmarshalText(%q) = %+v, want %+v", text, lenient, key)
		}

		if key.IsZero() {
			return // the zero key's text is the source edition, not a key
		}
		if strings.TrimSpace(string(key.Locale)) != "" {
			got, err := ReadKey(text)
			if err != nil {
				t.Fatalf("ReadKey(%q): %v", text, err)
			}
			if got != key {
				t.Fatalf("ReadKey(%q) = %+v, want %+v", text, got, key)
			}
		}
		if canon, err := model.CanonicalLocale(string(key.Locale)); err == nil && canon == key.Locale {
			got, err := model.ParseEditionKey(text)
			if err != nil {
				t.Fatalf("ParseEditionKey(%q): %v", text, err)
			}
			if got != key {
				t.Fatalf("ParseEditionKey(%q) = %+v, want %+v", text, got, key)
			}
		}
	})
}
