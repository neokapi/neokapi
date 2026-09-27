package project

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// ProjectSettings returns the recipe-owned project settings a push carries to
// the venue, each resolved to its effective value: bowrain.converge (on-push
// when unset) and defaults.translate_after (written when unset). Sending the
// default for an unset key is what returns the venue to the default when a
// recipe drops a setting it used to declare.
//
// Nil for a recipe with no venue binding, which has no venue to hold them.
func (r *Recipe) ProjectSettings() venue.ProjectSettings {
	if r == nil || r.Server == nil {
		return nil
	}
	translateAfter, _ := model.ResolveTranslateAfter(r.Defaults.TranslateAfter)
	return venue.ProjectSettings{
		venue.SettingConvergePolicy: string(r.Server.ResolvedConverge()),
		venue.SettingTranslateAfter: string(translateAfter),
	}
}
