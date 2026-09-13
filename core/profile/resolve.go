package profile

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

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
// so a persona's win over a channel's. Their vocabulary only tightens what the
// earlier layers resolved (see tightenVocabulary): a channel can add rules but
// never relax the profile's or a locale's, and a persona stays bounded by all
// three.
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
			resolved.Vocabulary.PreferredTerms = appendTermRules(
				resolved.Vocabulary.PreferredTerms, override.VocabularyOverrides...,
			)
			if len(override.ExampleOverrides) > 0 {
				resolved.Examples = slices.Concat(resolved.Examples, override.ExampleOverrides)
			}
		}
	}

	// Apply channel override. Tone and style replace; vocabulary tightens.
	if channel != "" {
		if override, ok := profile.Channels[channel]; ok {
			if override.Tone != nil {
				resolved.Tone = *override.Tone
			}
			if override.Style != nil {
				resolved.Style = *override.Style
			}
			if override.Vocabulary != nil {
				resolved.Vocabulary, _ = tightenVocabulary(resolved.Vocabulary, *override.Vocabulary)
			}
		}
	}

	// Apply persona override last, inside the guardrails every earlier layer
	// set. Tone/Style replace what a channel set (persona wins over channel).
	if persona != "" {
		if override, ok := profile.Personas[persona]; ok {
			if override.Tone != nil {
				resolved.Tone = *override.Tone
			}
			if override.Style != nil {
				resolved.Style = *override.Style
			}
			resolved.Vocabulary, _ = tightenVocabulary(resolved.Vocabulary, override.vocabulary())
		}
	}

	return &resolved
}

// tightenVocabulary layers an override's vocabulary onto the rules resolved so
// far. It returns the merged rules and the indices of the override's preferred
// terms it dropped.
//
// It can only add. Forbidden and competitor terms extend their lists, so every
// earlier rule keeps firing at its own severity whatever the override says
// about the same term. A preferred term is dropped when an earlier rule already
// governs one of its forms: a forbidden or competitor rule, which it would
// otherwise re-allow, or a preferred rule, whose wording it would otherwise
// contradict. The override's own forbidden terms count as earlier here. An
// abbreviation is added only where none is defined yet.
//
// Every list it changes is freshly allocated, because ResolveProfile works on
// a shallow copy of the source profile.
func tightenVocabulary(resolved, override VocabularyRules) (VocabularyRules, []int) {
	out := resolved
	out.ForbiddenTerms = appendTermRules(resolved.ForbiddenTerms, override.ForbiddenTerms...)
	out.CompetitorTerms = appendTermRules(resolved.CompetitorTerms, override.CompetitorTerms...)

	var kept []TermRule
	var dropped []int
	for i, pref := range override.PreferredTerms {
		if vocabularyForbids(out, pref) || rulesCover(resolved.PreferredTerms, pref) {
			dropped = append(dropped, i)
			continue
		}
		kept = append(kept, pref)
	}
	out.PreferredTerms = appendTermRules(resolved.PreferredTerms, kept...)

	if len(override.Abbreviations) > 0 {
		merged := make(map[string]string, len(resolved.Abbreviations)+len(override.Abbreviations))
		maps.Copy(merged, override.Abbreviations)
		maps.Copy(merged, resolved.Abbreviations)
		out.Abbreviations = merged
	}
	return out, dropped
}

// appendTermRules returns base with extra appended, always onto a freshly
// allocated slice. ResolveProfile works on a shallow copy of the source
// profile, so a plain append could grow into (and corrupt) the source's
// backing array when it has spare capacity; copying keeps the source pristine
// across repeated resolutions with different locales, channels and personas.
func appendTermRules(base []TermRule, extra ...TermRule) []TermRule {
	if len(extra) == 0 {
		return base
	}
	out := make([]TermRule, 0, len(base)+len(extra))
	out = append(out, base...)
	out = append(out, extra...)
	return out
}

// vocabularyForbids reports whether v already forbids one of rule's forms, as a
// forbidden term or a competitor term. It is the guardrail that stops a channel
// or a persona re-allowing a forbidden word through a preferred rule.
func vocabularyForbids(v VocabularyRules, rule TermRule) bool {
	return rulesCover(v.ForbiddenTerms, rule) || rulesCover(v.CompetitorTerms, rule)
}

// rulesCover reports whether a rule in rules names one of rule's forms. A form
// is compared the way the matcher reads the earlier rule: in its own casing
// when that rule is case-sensitive, and case-insensitively otherwise.
func rulesCover(rules []TermRule, rule TermRule) bool {
	forms := rule.AllForms()
	for _, r := range rules {
		for _, have := range r.AllForms() {
			for _, want := range forms {
				if have == want || (!r.CaseSensitive && strings.EqualFold(have, want)) {
					return true
				}
			}
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
