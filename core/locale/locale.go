// Package locale provides BCP-47 locale validation, normalization, and display
// name resolution for use throughout neokapi.
package locale

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// Supported locale format identifiers.
const (
	FormatBCP47 = "bcp-47"
	FormatPOSIX = "posix"
)

// LocaleInfo holds a locale code and its English display name.
type LocaleInfo struct {
	Code        string `json:"code"`
	DisplayName string `json:"display_name"`
}

// Parse validates and normalizes a BCP-47 locale string.
// It returns the canonical shortest form (e.g., "en" not "en-Latn-US").
func Parse(s string) (model.LocaleID, error) {
	tag, err := language.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid locale %q: %w", s, err)
	}
	return model.LocaleID(tag.String()), nil
}

// MustParse is like Parse but panics on invalid input.
func MustParse(s string) model.LocaleID {
	id, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}

// DisplayName returns the English display name for a locale ID.
// For example, "fr" returns "French", "de" returns "German".
// Falls back to the raw code for unrecognized tags.
func DisplayName(id model.LocaleID) string {
	tag, err := language.Parse(string(id))
	if err != nil {
		return string(id)
	}
	name := display.English.Tags().Name(tag)
	if name == "" {
		return string(id)
	}
	return name
}

// wellKnown is the curated list of common locales for UI dropdowns. It targets
// broad global coverage for a worldwide consumer + developer audience, not a
// niche or heritage set: the major world languages, the commercial regional
// variants localization teams actually request (Latin American Spanish,
// Canadian French, British English, Brazilian/European Portuguese, Simplified/
// Traditional Chinese), and the high-population languages that are commonly
// under-served (Filipino, Punjabi, Amharic, Burmese, Khmer, Sinhala, …).
//
// This is a convenience list only — Parse accepts any valid BCP-47 tag, so the
// long tail is always reachable by typing a code directly.
var wellKnown = []struct {
	code string
}{
	// Western European
	{"ca"}, {"cy"}, {"da"}, {"de"}, {"en"}, {"en-GB"}, {"es"}, {"es-419"},
	{"eu"}, {"fi"}, {"fr"}, {"fr-CA"}, {"ga"}, {"gl"}, {"is"}, {"it"},
	{"nb"}, {"nl"}, {"nn"}, {"pt"}, {"pt-BR"}, {"sv"},
	// Central / Eastern European
	{"bg"}, {"bs"}, {"cs"}, {"el"}, {"et"}, {"hr"}, {"hu"}, {"lt"}, {"lv"},
	{"mk"}, {"pl"}, {"ro"}, {"ru"}, {"sk"}, {"sl"}, {"sq"}, {"sr"}, {"uk"},
	// Middle East / Central Asia / Caucasus
	{"ar"}, {"az"}, {"fa"}, {"he"}, {"hy"}, {"ka"}, {"kk"}, {"tr"}, {"uz"},
	// South Asia
	{"bn"}, {"gu"}, {"hi"}, {"kn"}, {"ml"}, {"mr"}, {"ne"}, {"pa"}, {"si"},
	{"ta"}, {"te"}, {"ur"},
	// Southeast Asia
	{"fil"}, {"id"}, {"km"}, {"lo"}, {"ms"}, {"my"}, {"th"}, {"vi"},
	// East Asia
	{"ja"}, {"ko"}, {"mn"}, {"zh"}, {"zh-Hans"}, {"zh-Hant"},
	// Africa
	{"af"}, {"am"}, {"sw"},
}

// WellKnownLocales returns a curated list of common BCP-47 locales with
// English display names, sorted alphabetically by display name.
func WellKnownLocales() []LocaleInfo {
	result := make([]LocaleInfo, 0, len(wellKnown))
	for _, w := range wellKnown {
		tag := language.MustParse(w.code)
		result = append(result, LocaleInfo{
			Code:        tag.String(),
			DisplayName: display.English.Tags().Name(tag),
		})
	}
	slices.SortFunc(result, func(a, b LocaleInfo) int {
		return cmp.Compare(a.DisplayName, b.DisplayName)
	})
	return result
}

// ToPosix converts a BCP-47 locale code to POSIX format.
// "pt-BR" → "pt_BR", "en" → "en", "zh-Hans" → "zh_Hans".
func ToPosix(code string) string {
	return strings.ReplaceAll(code, "-", "_")
}

// FromPosix converts a POSIX locale code to BCP-47 format.
// "pt_BR" → "pt-BR", "en" → "en", "zh_Hans" → "zh-Hans".
func FromPosix(code string) string {
	return strings.ReplaceAll(code, "_", "-")
}

// FormatCode converts a BCP-47 locale code to the specified format.
// Supported formats: "bcp-47" (default, no-op) and "posix".
func FormatCode(code, format string) string {
	if format == FormatPOSIX {
		return ToPosix(code)
	}
	return code
}

// WellKnownLocalesFormatted returns the curated locale list with codes
// formatted according to the given format string.
func WellKnownLocalesFormatted(format string) []LocaleInfo {
	locales := WellKnownLocales()
	if format == FormatPOSIX {
		for i := range locales {
			locales[i].Code = ToPosix(locales[i].Code)
		}
	}
	return locales
}
