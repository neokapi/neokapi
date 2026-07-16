package brand

import (
	"context"
	"strings"

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
	persona := resolvePersona(rc)
	return ResolveProfile(profile, rc.Locale, channel, persona), nil
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
// profile. Persona is applied last, so an author persona's tone/style win over
// a channel's, while its vocabulary deltas stay bounded by the brand's
// guardrails (see PersonaOverride): Avoided terms tighten the forbidden set and
// a Preferred term the brand already forbids is dropped, never re-allowed.
func ResolveProfile(profile *VoiceProfile, loc model.LocaleID, channel, persona string) *VoiceProfile {
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

	// Apply persona override last, inside the brand's guardrails. Tone/Style
	// replace what a channel set (persona wins over channel). Vocabulary deltas
	// can only tighten: Avoided terms extend the forbidden set, and a Preferred
	// term the brand already forbids is dropped so a persona can never re-allow
	// a brand-prohibited word.
	if persona != "" {
		if override, ok := profile.Personas[persona]; ok {
			if override.Tone != nil {
				resolved.Tone = *override.Tone
			}
			if override.Style != nil {
				resolved.Style = *override.Style
			}
			resolved.Vocabulary.ForbiddenTerms = appendTermRules(
				resolved.Vocabulary.ForbiddenTerms, override.Avoided...,
			)
			for _, pref := range override.Preferred {
				if vocabularyForbids(resolved.Vocabulary, pref.Term) {
					continue
				}
				resolved.Vocabulary.PreferredTerms = appendTermRules(
					resolved.Vocabulary.PreferredTerms, pref,
				)
			}
		}
	}

	return &resolved
}

// appendTermRules returns base with extra appended, always onto a freshly
// allocated slice. ResolveProfile works on a shallow copy of the source
// profile, so a plain append could grow into (and corrupt) the source's
// backing array when it has spare capacity; copying keeps the source pristine
// across repeated resolutions with different personas.
func appendTermRules(base []TermRule, extra ...TermRule) []TermRule {
	if len(extra) == 0 {
		return base
	}
	out := make([]TermRule, 0, len(base)+len(extra))
	out = append(out, base...)
	out = append(out, extra...)
	return out
}

// vocabularyForbids reports whether the vocabulary rules already forbid term —
// as a forbidden term or a competitor term — compared case-insensitively. It is
// the guardrail that stops a persona re-allowing a brand-forbidden word through
// a Preferred rule.
func vocabularyForbids(v VocabularyRules, term string) bool {
	want := strings.ToLower(strings.TrimSpace(term))
	if want == "" {
		return false
	}
	for _, r := range v.ForbiddenTerms {
		if strings.ToLower(strings.TrimSpace(r.Term)) == want {
			return true
		}
	}
	for _, r := range v.CompetitorTerms {
		if strings.ToLower(strings.TrimSpace(r.Term)) == want {
			return true
		}
	}
	return false
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
