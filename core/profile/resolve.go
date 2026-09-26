package profile

import (
	"context"
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
)

// ResolveProfileFromContext resolves the most specific voice profile
// from the organizational hierarchy and applies locale + channel overrides.
//
// Resolution order (most specific wins):
//  1. ExplicitProfileID (from tool config or MCP parameter)
//  2. Collection-level: CollectionProfile (already loaded — a kapi recipe's
//     `profiles:` match), else CollectionConfig["voice_profile_id"]
//  3. Stream-level: StreamProperties["voice_profile_id"]
//  4. Project-level: ProjectProperties["voice_profile_id"]
//  5. Root-level: RootProfileID
//
// This is the single chain: a recipe-governed project and a server-governed one
// differ in which tiers they populate, never in how the tiers are ranked.
//
// Returns nil if no profile is bound at any level.
func ResolveProfileFromContext(ctx context.Context, rc ResolveContext, store Store) (*VoiceProfile, error) {
	profile, err := resolveBoundProfile(ctx, rc, store)
	if err != nil || profile == nil {
		return nil, err
	}

	if err := constraintError(profile); err != nil {
		return nil, err
	}
	channel := resolveChannel(rc)
	persona := resolvePersona(rc)
	return ResolveProfile(profile, rc.Locale, channel, persona), nil
}

// resolveBoundProfile walks the inheritance chain and returns the profile bound
// at the most specific tier. A tier binds either a store id, fetched here, or —
// at the collection tier — a profile the caller has already loaded from a
// source the store cannot name, such as a profile file or a starter pack.
func resolveBoundProfile(ctx context.Context, rc ResolveContext, store Store) (*VoiceProfile, error) {
	if rc.ExplicitProfileID != "" {
		return fetchProfile(ctx, store, rc.ExplicitProfileID)
	}
	if rc.CollectionProfile != nil {
		return rc.CollectionProfile, nil
	}
	for _, id := range []string{
		rc.CollectionConfig[PropertyProfileID],
		rc.StreamProperties[PropertyProfileID],
		rc.ProjectProperties[PropertyProfileID],
		rc.RootProfileID,
	} {
		if id != "" {
			return fetchProfile(ctx, store, id)
		}
	}
	return nil, nil
}

// fetchProfile loads a bound profile id from the store. A caller whose every
// tier carries an already-loaded profile passes no store; an id with nowhere to
// resolve it from is a configuration error rather than a silent miss, which
// would leave the content ungoverned and read as if nothing were bound.
func fetchProfile(ctx context.Context, store Store, id string) (*VoiceProfile, error) {
	if store == nil {
		return nil, fmt.Errorf("voice profile %q is bound but no voice store was supplied to resolve it", id)
	}
	return store.GetProfile(ctx, id)
}

// resolveChannel walks the inheritance chain to find the most specific channel
// key. An explicit rc.Channel (supplied at check time, e.g. `--channel`) wins,
// mirroring persona resolution.
func resolveChannel(rc ResolveContext) string {
	if rc.Channel != "" {
		return rc.Channel
	}
	if ch := rc.CollectionConfig[PropertyChannel]; ch != "" {
		return ch
	}
	if ch := rc.StreamProperties[PropertyChannel]; ch != "" {
		return ch
	}
	return rc.ProjectProperties[PropertyChannel]
}

// resolvePersona finds the most specific author persona key. An explicit
// rc.Persona (supplied at check time) wins; otherwise the collection, stream,
// and project scope maps are consulted in specificity order, mirroring channel
// resolution.
func resolvePersona(rc ResolveContext) string {
	if rc.Persona != "" {
		return rc.Persona
	}
	if p := rc.CollectionConfig[PropertyPersona]; p != "" {
		return p
	}
	if p := rc.StreamProperties[PropertyPersona]; p != "" {
		return p
	}
	return rc.ProjectProperties[PropertyPersona]
}

// ResolveProfile returns the most specific profile configuration for a given
// scope. It layers, in order, locale → channel → persona overrides on the base
// profile. A channel's or persona's tone and style replace the resolved ones,
// so a persona's win over a channel's.
func ResolveProfile(profile *VoiceProfile, loc model.LocaleID, channel, persona string) *VoiceProfile {
	if profile == nil {
		return nil
	}
	// Create a shallow copy
	resolved := *profile
	resolved.constraintScope = ConstraintScope{Locale: loc, Channel: channel, Persona: persona}

	// Apply locale override
	if loc != "" {
		if override, ok := matchLocaleOverride(profile.Locales, loc); ok {
			if override.Formality != "" {
				resolved.Tone.Formality = override.Formality
			}
			if override.Humor != "" {
				resolved.Tone.Humor = override.Humor
			}
			if override.PersonPOV != "" {
				resolved.Style.PersonPOV = override.PersonPOV
			}
			if len(override.ExampleOverrides) > 0 {
				resolved.Examples = slices.Concat(resolved.Examples, override.ExampleOverrides)
			}
		}
	}

	// Apply channel override. Tone and style replace.
	if channel != "" {
		if override, ok := profile.Channels[channel]; ok {
			if override.Tone != nil {
				resolved.Tone = *override.Tone
			}
			if override.Style != nil {
				resolved.Style = *override.Style
			}
		}
	}

	// Apply persona override last. Tone/Style replace what a channel set
	// (persona wins over channel).
	if persona != "" {
		if override, ok := profile.Personas[persona]; ok {
			if override.Tone != nil {
				resolved.Tone = *override.Tone
			}
			if override.Style != nil {
				resolved.Style = *override.Style
			}
		}
	}

	return &resolved
}

// matchLocaleOverride finds the override whose key matches loc, tolerating
// BCP-47 formatting differences between the profile's keys and the requested
// locale. An exact key match wins first (cheap, and preserves any tag the
// author wrote verbatim); otherwise keys are compared in canonical form, so a
// profile keyed "pt-BR" still matches a "pt-br" lookup and "EN" matches "en".
// Region specificity is preserved — "en" never matches "en-US" — because
// canonicalization normalizes form, not granularity.
func matchLocaleOverride(overrides map[model.LocaleID]LocaleOverride, loc model.LocaleID) (LocaleOverride, bool) {
	if override, ok := overrides[loc]; ok {
		return override, true
	}
	want := locale.Normalize(loc)
	for key, override := range overrides {
		if locale.Normalize(key) == want {
			return override, true
		}
	}
	return LocaleOverride{}, false
}
