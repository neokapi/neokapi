package mdspike

import (
	"maps"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
)

// merge layers a child profile over the profile it extends. It is the whole
// point of `extends:`: house rules are declared once and inherited, instead of
// being copied into every profile and drifting.
//
// The rules:
//
//   - Scalars and enums: the child's value wins when it is set, otherwise the
//     parent's stands.
//   - Personality: replaced wholesale when the child names one, because a list
//     of adjectives is a statement, not an accumulation.
//   - Pattern and term lists: the parent's entries come first, the child's
//     after, deduplicated. The child may restate an inherited rule to change
//     its severity or note; it may not silently double it. That dedupe is not
//     cosmetic — two copies of one prohibition score two penalties.
//   - Locales: merged per locale, field by field.
//
// A child cannot delete an inherited rule. That is deliberate for a house-rules
// hierarchy and is one of the things the note calls out as unproven at scale.
func merge(parent, child *profile.VoiceProfile) *profile.VoiceProfile {
	if parent == nil {
		return child
	}
	if child == nil {
		return parent
	}
	out := parent.Clone()

	out.ID = firstNonEmpty(child.ID, out.ID)
	out.Name = firstNonEmpty(child.Name, out.Name)
	out.Description = firstNonEmpty(child.Description, out.Description)
	out.WorkspaceID = firstNonEmpty(child.WorkspaceID, out.WorkspaceID)
	if child.MinScore != 0 {
		out.MinScore = child.MinScore
	}

	if len(child.Tone.Personality) > 0 {
		out.Tone.Personality = append([]string(nil), child.Tone.Personality...)
	}
	out.Tone.Formality = firstNonEmpty(child.Tone.Formality, out.Tone.Formality)
	out.Tone.Emotion = firstNonEmpty(child.Tone.Emotion, out.Tone.Emotion)
	out.Tone.Humor = firstNonEmpty(child.Tone.Humor, out.Tone.Humor)
	out.Tone.Guidelines = firstNonEmpty(child.Tone.Guidelines, out.Tone.Guidelines)

	out.Style.ActiveVoice = out.Style.ActiveVoice || child.Style.ActiveVoice
	out.Style.SentenceLength = firstNonEmpty(child.Style.SentenceLength, out.Style.SentenceLength)
	out.Style.PersonPOV = firstNonEmpty(child.Style.PersonPOV, out.Style.PersonPOV)
	out.Style.Contractions = firstNonEmpty(child.Style.Contractions, out.Style.Contractions)
	out.Style.ProhibitedPatterns = mergePatterns(out.Style.ProhibitedPatterns, child.Style.ProhibitedPatterns)
	out.Style.RequiredPatterns = mergePatterns(out.Style.RequiredPatterns, child.Style.RequiredPatterns)

	out.Vocabulary.PreferredTerms = mergeTerms(out.Vocabulary.PreferredTerms, child.Vocabulary.PreferredTerms)
	out.Vocabulary.ForbiddenTerms = mergeTerms(out.Vocabulary.ForbiddenTerms, child.Vocabulary.ForbiddenTerms)
	out.Vocabulary.CompetitorTerms = mergeTerms(out.Vocabulary.CompetitorTerms, child.Vocabulary.CompetitorTerms)
	if len(child.Vocabulary.Abbreviations) > 0 {
		if out.Vocabulary.Abbreviations == nil {
			out.Vocabulary.Abbreviations = map[string]string{}
		}
		maps.Copy(out.Vocabulary.Abbreviations, child.Vocabulary.Abbreviations)
	}

	out.Examples = append(out.Examples, child.Examples...)
	out.Locales = mergeLocales(out.Locales, child.Locales)
	return out
}

// mergePatterns appends child patterns after parent patterns, keyed by regex. A
// child restating an inherited regex replaces it in place, so the inherited
// order is stable and the prohibition is counted once.
func mergePatterns(parent, child []profile.Pattern) []profile.Pattern {
	if len(child) == 0 {
		return parent
	}
	out := append([]profile.Pattern(nil), parent...)
	index := make(map[string]int, len(out))
	for i, p := range out {
		index[p.Regex] = i
	}
	for _, p := range child {
		if i, ok := index[p.Regex]; ok {
			out[i] = p
			continue
		}
		index[p.Regex] = len(out)
		out = append(out, p)
	}
	return out
}

// mergeTerms appends child rules after parent rules, keyed by the case-folded
// term. A child restating an inherited term replaces it in place — that is how
// a profile raises "simply" from minor to major without listing it twice.
func mergeTerms(parent, child []profile.TermRule) []profile.TermRule {
	if len(child) == 0 {
		return parent
	}
	out := append([]profile.TermRule(nil), parent...)
	index := make(map[string]int, len(out))
	for i, t := range out {
		index[termKey(t.Term)] = i
	}
	for _, t := range child {
		if i, ok := index[termKey(t.Term)]; ok {
			out[i] = t
			continue
		}
		index[termKey(t.Term)] = len(out)
		out = append(out, t)
	}
	return out
}

func mergeLocales(parent, child map[model.LocaleID]profile.LocaleOverride) map[model.LocaleID]profile.LocaleOverride {
	if len(child) == 0 {
		return parent
	}
	out := make(map[model.LocaleID]profile.LocaleOverride, len(parent)+len(child))
	maps.Copy(out, parent)
	for id, c := range child {
		p, ok := out[id]
		if !ok {
			out[id] = c
			continue
		}
		p.Formality = firstNonEmpty(c.Formality, p.Formality)
		p.Humor = firstNonEmpty(c.Humor, p.Humor)
		p.PersonPOV = firstNonEmpty(c.PersonPOV, p.PersonPOV)
		p.CulturalNotes = firstNonEmpty(c.CulturalNotes, p.CulturalNotes)
		p.VocabularyOverrides = mergeTerms(p.VocabularyOverrides, c.VocabularyOverrides)
		p.ExampleOverrides = append(p.ExampleOverrides, c.ExampleOverrides...)
		out[id] = p
	}
	return out
}

func termKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
