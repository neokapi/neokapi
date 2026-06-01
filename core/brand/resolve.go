package brand

import (
	"context"

	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
)

// ResolveProfileFromContext resolves the most specific brand voice profile
// from the organizational hierarchy and applies locale + channel overrides.
//
// Resolution order (most specific wins):
//  1. ExplicitProfileID (from tool config or MCP parameter)
//  2. Collection-level: CollectionConfig["brand_voice_profile_id"]
//  3. Stream-level: StreamProperties["brand_voice_profile_id"]
//  4. Project-level: ProjectProperties["brand_voice_profile_id"]
//  5. Workspace-level: WorkspaceProfileID
//
// Returns nil if no profile is bound at any level.
func ResolveProfileFromContext(ctx context.Context, rc ResolveContext, store BrandStore) (*VoiceProfile, error) {
	profileID := resolveProfileID(rc)
	if profileID == "" {
		return nil, nil
	}

	profile, err := store.GetProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}

	channel := resolveChannel(rc)
	return ResolveProfile(profile, rc.Locale, channel), nil
}

// resolveProfileID walks the inheritance chain to find the most specific profile ID.
func resolveProfileID(rc ResolveContext) string {
	if rc.ExplicitProfileID != "" {
		return rc.ExplicitProfileID
	}
	if id := rc.CollectionConfig[PropertyProfileID]; id != "" {
		return id
	}
	if id := rc.StreamProperties[PropertyProfileID]; id != "" {
		return id
	}
	if id := rc.ProjectProperties[PropertyProfileID]; id != "" {
		return id
	}
	return rc.WorkspaceProfileID
}

// resolveChannel walks the inheritance chain to find the most specific channel key.
func resolveChannel(rc ResolveContext) string {
	if ch := rc.CollectionConfig[PropertyChannel]; ch != "" {
		return ch
	}
	if ch := rc.StreamProperties[PropertyChannel]; ch != "" {
		return ch
	}
	return rc.ProjectProperties[PropertyChannel]
}

// ResolveProfile returns the most specific profile configuration for a given scope.
// It applies locale and channel overrides to the base profile.
func ResolveProfile(profile *VoiceProfile, loc model.LocaleID, channel string) *VoiceProfile {
	if profile == nil {
		return nil
	}
	// Create a shallow copy
	resolved := *profile

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
			if len(override.VocabularyOverrides) > 0 {
				resolved.Vocabulary.PreferredTerms = append(
					resolved.Vocabulary.PreferredTerms,
					override.VocabularyOverrides...,
				)
			}
			if len(override.ExampleOverrides) > 0 {
				resolved.Examples = append(resolved.Examples, override.ExampleOverrides...)
			}
		}
	}

	// Apply channel override
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
