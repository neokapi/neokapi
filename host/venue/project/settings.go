package project

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// ProjectSettings returns the recipe-owned project settings a push carries to
// the venue, each resolved to its effective value: bowrain.converge (on-push
// when unset), defaults.translate_after (written when unset) and the term rules
// the recipe declares ("" when it declares none). Sending the default for an
// unset key is what returns the venue to the default when a recipe drops a
// setting it used to declare.
//
// root is the recipe's directory, which a `flows_dir:` holding flow steps with
// term rules is relative to.
//
// Nil for a recipe with no venue binding, which has no venue to hold them.
func (r *Recipe) ProjectSettings(root string) (venue.ProjectSettings, error) {
	if r == nil || r.Server == nil {
		return nil, nil
	}
	translateAfter, _ := model.ResolveTranslateAfter(r.Defaults.TranslateAfter)
	declared, err := r.DeclaredTermRules(root)
	if err != nil {
		return nil, err
	}
	termRules, err := declared.Encode()
	if err != nil {
		return nil, err
	}
	return venue.ProjectSettings{
		venue.SettingConvergePolicy: string(r.Server.ResolvedConverge()),
		venue.SettingTranslateAfter: string(translateAfter),
		venue.SettingTermRules:      termRules,
	}, nil
}
