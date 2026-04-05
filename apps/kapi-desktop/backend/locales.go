package backend

import (
	"github.com/neokapi/neokapi/core/locale"
)

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
