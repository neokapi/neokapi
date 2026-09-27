package venue

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
)

// Recipe-owned project settings: values the recipe declares once for the whole
// project and the venue applies to its own runs. A push carries them, and the
// venue applies each one the pusher may set. A setting that tightens what the
// project allows takes effect from any push to the project's default stream; a
// setting that loosens it takes effect only from a pusher who may manage the
// project, and is otherwise held back and reported.
//
// Each value travels resolved, never as the raw recipe text: an unset key is
// sent as its default, so a recipe that removes a setting returns the venue to
// the default rather than leaving the last explicit value in place.
const (
	// SettingConvergePolicy is the recipe's bowrain.converge policy: on-push
	// or manual.
	SettingConvergePolicy = "converge_policy"

	// SettingTranslateAfter is the recipe's defaults.translate_after level:
	// written, established or none.
	SettingTranslateAfter = "translate_after"
)

// ProjectSettings maps a recipe-owned setting key to its resolved value.
type ProjectSettings map[string]string

// Hash is a stable digest of the settings, over sorted keys, so a producer can
// remember which settings a venue last confirmed. Empty settings hash to "".
func (s ProjectSettings) Hash() string {
	if len(s) == 0 {
		return ""
	}
	h := sha256.New()
	for _, k := range slices.Sorted(maps.Keys(s)) {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(s[k]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Differing returns the settings in s whose value the venue does not hold, as
// reported in held. A key held does not report is included, since the venue
// has said nothing about it. Nil when every setting already matches.
func (s ProjectSettings) Differing(held ProjectSettings) ProjectSettings {
	var out ProjectSettings
	for k, v := range s {
		if hv, ok := held[k]; ok && hv == v {
			continue
		}
		if out == nil {
			out = ProjectSettings{}
		}
		out[k] = v
	}
	return out
}

// Why a venue kept its own value for a setting a push asked it to change.
const (
	// SettingLoosens: the requested value allows more than the value in force,
	// and the pusher may not manage the project.
	SettingLoosens = "loosens"

	// SettingNotDefaultStream: the push went to a stream other than the
	// project's default, and settings apply only from the default stream.
	SettingNotDefaultStream = "not_default_stream"
)

// SettingRefusal reports a recipe-owned setting the venue kept at its own value
// although a push asked for another.
type SettingRefusal struct {
	Setting   string `json:"setting"`
	Requested string `json:"requested"`
	InForce   string `json:"in_force"`
	Reason    string `json:"reason"`

	// Requires names the permission that may apply the requested value, for a
	// refusal whose Reason is SettingLoosens.
	Requires string `json:"requires,omitempty"`

	// DefaultStream names the stream a push applies settings from, for a
	// refusal whose Reason is SettingNotDefaultStream.
	DefaultStream string `json:"default_stream,omitempty"`
}

// String describes the refusal in one line, naming what would apply it.
func (r SettingRefusal) String() string {
	switch r.Reason {
	case SettingLoosens:
		return fmt.Sprintf("%s stays %s: the recipe asks for %s, which allows more than the project does. "+
			"A workspace owner or admin (%s) applies it by pushing the recipe",
			r.Setting, r.InForce, r.Requested, r.Requires)
	case SettingNotDefaultStream:
		return fmt.Sprintf("%s stays %s: the recipe asks for %s, and settings apply only from a push to the default stream %q",
			r.Setting, r.InForce, r.Requested, r.DefaultStream)
	default:
		return fmt.Sprintf("%s stays %s: the recipe asks for %s (%s)", r.Setting, r.InForce, r.Requested, r.Reason)
	}
}
