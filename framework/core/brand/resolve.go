package brand

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
