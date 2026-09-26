package store

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// RecipeSettingsOf returns the recipe-owned settings a project holds, each at
// its effective value: the converge policy (on-push when unset) and the
// translate_after level (written when unset). It is what the push negotiation
// reports, so a producer compares its recipe with the values the server runs
// by rather than with whatever raw text happens to be stored.
func RecipeSettingsOf(p *Project) venue.ProjectSettings {
	return venue.ProjectSettings{
		venue.SettingConvergePolicy: NormalizeConvergePolicy(projectConvergePolicy(p)),
		venue.SettingTranslateAfter: string(TranslateAfterFor(p)),
	}
}

func projectConvergePolicy(p *Project) string {
	if p == nil {
		return ""
	}
	return p.ConvergePolicy
}

// ValidateRecipeSettings rejects a value the recipe schema does not allow for a
// setting the server knows. A key it does not know is left alone, so a newer
// producer's settings reach an older server without failing the push.
func ValidateRecipeSettings(s venue.ProjectSettings) error {
	if v, ok := s[venue.SettingConvergePolicy]; ok {
		if v != ConvergePolicyOnPush && v != ConvergePolicyManual {
			return fmt.Errorf("settings.%s: %q is not a converge policy. Use %s or %s",
				venue.SettingConvergePolicy, v, ConvergePolicyOnPush, ConvergePolicyManual)
		}
	}
	if v, ok := s[venue.SettingTranslateAfter]; ok {
		if _, known := model.ResolveTranslateAfter(v); !known {
			return fmt.Errorf("settings.%s: %q is not a source level. Use %s, %s or %s",
				venue.SettingTranslateAfter, v, model.TranslateAfterWritten,
				model.TranslateAfterEstablished, model.TranslateAfterNone)
		}
	}
	return nil
}

// ApplyRecipeSettings writes the recipe-owned settings in s onto p and reports
// whether any stored value changed. Call ValidateRecipeSettings first; a value
// it would reject is ignored here. Keys the server does not know are ignored.
func ApplyRecipeSettings(p *Project, s venue.ProjectSettings) bool {
	if p == nil {
		return false
	}
	changed := false
	if v, ok := s[venue.SettingConvergePolicy]; ok &&
		(v == ConvergePolicyOnPush || v == ConvergePolicyManual) && p.ConvergePolicy != v {
		p.ConvergePolicy = v
		changed = true
	}
	if v, ok := s[venue.SettingTranslateAfter]; ok {
		if level, known := model.ResolveTranslateAfter(v); known && p.Properties[TranslateAfterProperty] != string(level) {
			if p.Properties == nil {
				p.Properties = map[string]string{}
			}
			p.Properties[TranslateAfterProperty] = string(level)
			changed = true
		}
	}
	return changed
}
