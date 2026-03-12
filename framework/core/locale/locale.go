// Package locale provides BCP-47 locale validation, normalization, and display
// name resolution for use throughout gokapi.
package locale

import (
	"fmt"
	"sort"

	"github.com/gokapi/gokapi/core/model"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
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

// wellKnown is the curated list of common locales for UI dropdowns.
var wellKnown = []struct {
	code string
}{
	{"af"}, {"ar"}, {"bg"}, {"bn"}, {"ca"}, {"cs"}, {"da"}, {"de"},
	{"el"}, {"en"}, {"es"}, {"et"}, {"fa"}, {"fi"}, {"fr"}, {"gu"},
	{"he"}, {"hi"}, {"hr"}, {"hu"}, {"id"}, {"it"}, {"ja"}, {"kn"},
	{"ko"}, {"lt"}, {"lv"}, {"ml"}, {"mr"}, {"ms"}, {"nb"}, {"nl"},
	{"pl"}, {"pt"}, {"pt-BR"}, {"ro"}, {"ru"}, {"sk"}, {"sl"}, {"sr"},
	{"sv"}, {"sw"}, {"ta"}, {"te"}, {"th"}, {"tr"}, {"uk"}, {"ur"},
	{"vi"}, {"zh"}, {"zh-Hans"}, {"zh-Hant"},
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
	sort.Slice(result, func(i, j int) bool {
		return result[i].DisplayName < result[j].DisplayName
	})
	return result
}
