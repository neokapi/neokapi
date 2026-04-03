package brand

import "context"

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
func ResolveProfile(profile *VoiceProfile, locale string, channel string) *VoiceProfile {
	if profile == nil {
		return nil
	}
	// Create a shallow copy
	resolved := *profile

	// Apply locale override
	if locale != "" {
		if override, ok := profile.Locales[locale]; ok {
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
