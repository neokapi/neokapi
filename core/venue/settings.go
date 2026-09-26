package venue

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
)

// Recipe-owned project settings: values the recipe declares once for the whole
// project and the venue applies to its own runs. A push carries them, so whoever
// may push the recipe may put its settings in force, and the venue holds the
// values the checkout converges by.
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
