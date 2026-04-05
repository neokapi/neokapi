package backend

import (
	"github.com/neokapi/neokapi/core/locale"
)

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

	// Append custom locales with resolved display names.
	for _, code := range settings.CustomLocales {
		id, err := locale.Parse(code)
		if err != nil {
			continue
		}
		result = append(result, locale.LocaleInfo{
			Code:        string(id),
			DisplayName: locale.DisplayName(id),
		})
	}

	return result
}
