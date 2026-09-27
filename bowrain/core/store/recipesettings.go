package store

import (
	"fmt"
	"maps"
	"slices"
	"strings"

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

// DefaultStreamOf is the stream a project's recipe-owned settings are applied
// from: the project's default stream, or main when none is recorded.
func DefaultStreamOf(p *Project) string {
	if p != nil && p.DefaultStream != "" {
		return p.DefaultStream
	}
	return "main"
}

// ValidateRecipeSettings rejects a value the recipe schema does not allow for a
// setting the server knows. A key it does not know is left alone, so a newer
// producer's settings reach an older server without failing the push.
func ValidateRecipeSettings(s venue.ProjectSettings) error {
	for _, k := range slices.Sorted(maps.Keys(s)) {
		if _, known := settingStrictness(k, s[k]); known {
			continue
		}
		switch k {
		case venue.SettingConvergePolicy:
			return fmt.Errorf("settings.%s: %q is not a converge policy. Use %s or %s",
				k, s[k], ConvergePolicyOnPush, ConvergePolicyManual)
		case venue.SettingTranslateAfter:
			return fmt.Errorf("settings.%s: %q is not a source level. Use %s, %s or %s",
				k, s[k], model.TranslateAfterWritten, model.TranslateAfterEstablished, model.TranslateAfterNone)
		}
	}
	return nil
}

// settingStrictness ranks a value of a recipe-owned setting by how much it
// holds back: a higher rank allows less. Moving to a higher rank tightens the
// setting and moving to a lower one loosens it.
//
//	translate_after: none < written < established. A higher level holds more
//	                 source back from translation.
//	converge_policy: on-push < manual. Under manual the server starts no run of
//	                 its own; every run is started by someone.
//
// known is false for a key the server does not know or a value outside the
// recipe schema.
func settingStrictness(key, value string) (rank int, known bool) {
	switch key {
	case venue.SettingTranslateAfter:
		switch model.TranslateAfterLevel(value) {
		case model.TranslateAfterNone:
			return 0, true
		case model.TranslateAfterWritten:
			return 1, true
		case model.TranslateAfterEstablished:
			return 2, true
		}
	case venue.SettingConvergePolicy:
		switch value {
		case ConvergePolicyOnPush:
			return 0, true
		case ConvergePolicyManual:
			return 1, true
		}
	}
	return 0, false
}

// Loosens reports whether moving a setting from one value to another allows
// more than before. False for a key or value the server does not know.
func Loosens(key, from, to string) bool {
	fromRank, okFrom := settingStrictness(key, from)
	toRank, okTo := settingStrictness(key, to)
	return okFrom && okTo && toRank < fromRank
}

// SettingsPusher is what the server knows about a push when it decides which
// recipe-owned settings the push may apply.
type SettingsPusher struct {
	// Stream is the stream the push goes to.
	Stream string
	// MayLoosen is whether the pusher may manage the project, which is what
	// applying a looser setting takes.
	MayLoosen bool
}

// DecideRecipeSettings splits the settings a push requests into the ones it
// applies and the ones the project keeps. Only settings that differ from what
// the project holds are considered, and only keys and values the server knows.
//
// A push to a stream other than the project's default applies none of them.
// On the default stream, a setting that tightens is applied for any pusher and
// a setting that loosens only for one who may manage the project.
func DecideRecipeSettings(p *Project, requested venue.ProjectSettings, pusher SettingsPusher) (venue.ProjectSettings, []venue.SettingRefusal) {
	held := RecipeSettingsOf(p)
	defaultStream := DefaultStreamOf(p)
	onDefault := pusher.Stream == "" || pusher.Stream == defaultStream

	var apply venue.ProjectSettings
	var refused []venue.SettingRefusal
	for key, want := range requested.Differing(held) {
		if _, known := settingStrictness(key, want); !known {
			continue
		}
		refusal := venue.SettingRefusal{Setting: key, Requested: want, InForce: held[key]}
		switch {
		case !onDefault:
			refusal.Reason = venue.SettingNotDefaultStream
			refusal.DefaultStream = defaultStream
		case Loosens(key, held[key], want) && !pusher.MayLoosen:
			refusal.Reason = venue.SettingLoosens
			refusal.Requires = "manage_project"
		default:
			if apply == nil {
				apply = venue.ProjectSettings{}
			}
			apply[key] = want
			continue
		}
		refused = append(refused, refusal)
	}
	slices.SortFunc(refused, func(a, b venue.SettingRefusal) int { return strings.Compare(a.Setting, b.Setting) })
	return apply, refused
}

// SettingChange is one recipe-owned setting a push changed on a project.
type SettingChange struct {
	Setting string
	From    string
	To      string
}

// ApplyRecipeSettings writes the settings in s onto p and returns each stored
// value it changed, with the effective value it replaced, sorted by setting. A
// project that never stored a level holds the default, so writing the default
// explicitly is reported as a change from the default to itself.
//
// It writes what it is given: DecideRecipeSettings decides which settings a
// push may apply. Keys and values the server does not know are ignored.
func ApplyRecipeSettings(p *Project, s venue.ProjectSettings) []SettingChange {
	if p == nil {
		return nil
	}
	held := RecipeSettingsOf(p)
	var changes []SettingChange
	for _, key := range []string{venue.SettingConvergePolicy, venue.SettingTranslateAfter} {
		want, ok := s[key]
		if !ok {
			continue
		}
		if _, known := settingStrictness(key, want); !known {
			continue
		}
		switch key {
		case venue.SettingConvergePolicy:
			if p.ConvergePolicy == want {
				continue
			}
			p.ConvergePolicy = want
		case venue.SettingTranslateAfter:
			if p.Properties[TranslateAfterProperty] == want {
				continue
			}
			if p.Properties == nil {
				p.Properties = map[string]string{}
			}
			p.Properties[TranslateAfterProperty] = want
		}
		changes = append(changes, SettingChange{Setting: key, From: held[key], To: want})
	}
	return changes
}
