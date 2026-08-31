package backend

import (
	"fmt"

	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
)

// canonicalLocale normalizes a locale a caller supplied, and refuses one that
// is not a locale.
//
// Every desktop method taking a locale ARGUMENT goes through it: BCP-47 is the
// only representation inside kapi, so a tab passing `nb_NO` must reach the
// stores and the resolver as `nb-NO`. Without the gate a typo becomes an
// identity nothing rejects — indistinguishable from a language nobody has
// translated yet — and the raw string keys a lookup that silently matches
// nothing.
func canonicalLocale(s string) (model.LocaleID, error) {
	loc, err := locale.Canonical(s)
	if err != nil {
		return "", fmt.Errorf("locale %q is not a locale: %w", s, err)
	}
	return loc, nil
}

// canonicalLocales normalizes a list, refusing on the first entry that is not a
// locale. An empty list stays empty rather than erroring: a filter naming no
// language is a filter that does not narrow by language.
func canonicalLocales(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		loc, err := canonicalLocale(s)
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
			id, err := locale.Parse(cl.Code)
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
