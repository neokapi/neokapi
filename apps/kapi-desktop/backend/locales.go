package backend

import (
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
)

// requireLocale canonicalizes a locale a method cannot proceed without.
//
// canonicalLocale (memory.go) answers an empty locale with an empty result,
// because an unscoped search is a real question. A review decision is not: it
// is about one language, and an empty locale there names no unit.
func requireLocale(s string) (model.LocaleID, error) {
	if strings.TrimSpace(s) == "" {
		return "", errors.New("no locale: this asks about one language")
	}
	loc, err := canonicalLocale(s)
	if err != nil {
		return "", fmt.Errorf("locale %q is not a locale: %w", s, err)
	}
	return loc, nil
}

// canonicalLocales normalizes the locales a filter narrows by, refusing on the
// first entry that is not one. An empty list stays empty rather than erroring:
// a filter naming no language does not narrow by language.
func canonicalLocales(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		loc, err := requireLocale(s)
		if err != nil {
			return nil, err
		}
		out = append(out, string(loc))
	}
	return out, nil
}

// GetAllLocales returns the full curated locale list without filtering.
// Used by the locale settings UI to show all locales with toggles.
func (a *App) GetAllLocales() []locale.LocaleInfo {
	return locale.WellKnownLocales()
}

// GetKnownLocales returns the curated locale list, filtered by user settings.
// Hidden locales are removed; custom locales are appended with resolved display names.
func (a *App) GetKnownLocales() []locale.LocaleInfo {
	all := locale.WellKnownLocales()

	settings := a.GetSettings()
	hidden := make(map[string]bool, len(settings.HiddenLocales))
	for _, code := range settings.HiddenLocales {
		hidden[code] = true
	}

	var result []locale.LocaleInfo
	for _, l := range all {
		if !hidden[l.Code] {
			result = append(result, l)
		}
	}

	// Append custom locales with user-provided or auto-resolved display names.
	for _, cl := range settings.CustomLocales {
		displayName := cl.DisplayName
		if displayName == "" {
			// A custom locale is typed by the person adding it, so it arrives in
			// whatever style they write and may name a pseudo-locale.
			id, err := locale.Canonical(cl.Code)
			if err != nil {
				continue
			}
			displayName = locale.DisplayName(id)
		}
		result = append(result, locale.LocaleInfo{
			Code:        cl.Code,
			DisplayName: displayName,
		})
	}

	return result
}
