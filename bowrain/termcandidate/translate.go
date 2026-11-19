package termcandidate

import (
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// ProposerIdentity is the identity proposed target-language terms are filed
// under. It carries the tool prefix (platauth.ToolIdentityPrefix), so the
// person who keeps a proposal reviews a tool's work.
const ProposerIdentity = "tool/term-proposal"

// A proposed target-language term is stored as a property of its concept,
// PropTranslationPrefix followed by the target locale ("term_proposal.nb"),
// whose value is the proposed text. Each target term a reviewer turned down
// is listed, lower case and one per line, under PropTranslationRejectedPrefix
// followed by the locale.
//
// It is a property because a concept's terms govern text.
// terms.Concept.PreferredTerm falls back to any term in the locale that is
// not deprecated or forbidden. A proposed target term would therefore become
// the replacement that terms.RuleForConcept hands the translate tools before
// anyone reviewed it. A property governs nothing; keeping the proposal adds
// the term. Filing a proposal leaves UpdatedAt as it was, so a kept concept
// keeps the revision the context deck attested.
const (
	PropTranslationPrefix         = "term_proposal."
	PropTranslationRejectedPrefix = "term_proposal_rejected."
)

// Translation is a proposed target-language term of a concept.
type Translation struct {
	Locale model.LocaleID
	Text   string
}

// Translations lists the proposals a concept carries, by locale.
func Translations(cp terms.Concept) []Translation {
	var out []Translation
	for _, k := range slices.Sorted(maps.Keys(cp.Properties)) {
		loc, ok := strings.CutPrefix(k, PropTranslationPrefix)
		text := strings.TrimSpace(cp.Properties[k])
		if !ok || loc == "" || text == "" {
			continue
		}
		out = append(out, Translation{Locale: model.LocaleID(loc), Text: text})
	}
	return out
}

// TranslationFor is the proposal a concept carries for target, or "".
func TranslationFor(cp terms.Concept, target model.LocaleID) string {
	return strings.TrimSpace(cp.Properties[PropTranslationPrefix+string(target)])
}

// RejectedTranslations are the target terms a reviewer turned down for the
// concept in target, lower case.
func RejectedTranslations(cp terms.Concept, target model.LocaleID) []string {
	var out []string
	for line := range strings.SplitSeq(cp.Properties[PropTranslationRejectedPrefix+string(target)], "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// NeedsTranslation reports whether a concept is waiting for a proposed term
// in target: it is a term of the project (not a candidate) with a source
// term, it may be translated, no term in target satisfies it
// (terms.RuleForConcept), and it carries no proposal for target yet.
func NeedsTranslation(cp terms.Concept, source, target model.LocaleID) bool {
	if cp.DoNotTranslate || sameLanguage(source, target) || IsCandidate(cp, source) {
		return false
	}
	if SourceText(cp, source) == "" {
		return false
	}
	if _, ok := terms.RuleForConcept(cp, source, target); ok {
		return false
	}
	return TranslationFor(cp, target) == ""
}

// SetTranslation files text as the proposal for target, and reports whether
// it did. A blank text, a text a reviewer turned down, and a concept that no
// longer needs one (NeedsTranslation) are left alone.
func SetTranslation(cp *terms.Concept, source, target model.LocaleID, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || !NeedsTranslation(*cp, source, target) {
		return false
	}
	if slices.Contains(RejectedTranslations(*cp, target), strings.ToLower(text)) {
		return false
	}
	cp.Properties = maps.Clone(cp.Properties)
	if cp.Properties == nil {
		cp.Properties = map[string]string{}
	}
	cp.Properties[PropTranslationPrefix+string(target)] = text
	return true
}

// KeepTranslation makes text the preferred term of the concept in target and
// clears the proposal. text is the proposal as filed or a reviewer's own
// wording. A term already in target with the same text (case-insensitively)
// becomes preferred rather than added twice, and any other preferred term in
// target becomes admitted, so the locale keeps one preferred term.
func KeepTranslation(cp *terms.Concept, target model.LocaleID, text string) {
	text = strings.TrimSpace(text)
	cp.Properties = maps.Clone(cp.Properties)
	delete(cp.Properties, PropTranslationPrefix+string(target))
	if text == "" {
		return
	}
	cp.Terms = slices.Clone(cp.Terms)
	found := false
	for i, t := range cp.Terms {
		if !sameLanguage(t.Locale, target) {
			continue
		}
		switch {
		case strings.EqualFold(strings.TrimSpace(t.Text), text):
			cp.Terms[i].Status = model.TermPreferred
			found = true
		case t.Status == model.TermPreferred:
			cp.Terms[i].Status = model.TermAdmitted
		}
	}
	if !found {
		cp.Terms = append(cp.Terms, terms.Term{Text: text, Locale: target, Status: model.TermPreferred})
	}
}

// RejectTranslation clears the proposal for target and records its text as
// turned down, so it is not proposed for the concept again.
func RejectTranslation(cp *terms.Concept, target model.LocaleID) {
	text := strings.ToLower(TranslationFor(*cp, target))
	cp.Properties = maps.Clone(cp.Properties)
	delete(cp.Properties, PropTranslationPrefix+string(target))
	if text == "" {
		return
	}
	set := RejectedTranslations(*cp, target)
	if !slices.Contains(set, text) {
		set = append(set, text)
	}
	slices.Sort(set)
	if cp.Properties == nil {
		cp.Properties = map[string]string{}
	}
	cp.Properties[PropTranslationRejectedPrefix+string(target)] = strings.Join(set, "\n")
}

// Examples gathers, for each source term, up to a few source texts that use
// it, matched as the Counter matches. A store can feed it only the blocks
// holding one of the terms (platstore.BlockQuery.TextAny) and stop reading
// once Full reports true.
type Examples struct {
	english bool
	max     int
	terms   []string
	got     [][]string
	full    int
}

// NewExamples collects up to max texts per term of the source language.
// Blank terms and case-insensitive repeats are dropped.
func NewExamples(source model.LocaleID, sourceTerms []string, max int) *Examples {
	e := &Examples{english: terms.BaseLanguage(source) == "en", max: max}
	seen := map[string]bool{}
	for _, t := range sourceTerms {
		t = strings.TrimSpace(t)
		key := strings.ToLower(t)
		if t == "" || seen[key] {
			continue
		}
		seen[key] = true
		e.terms = append(e.terms, t)
		e.got = append(e.got, nil)
	}
	return e
}

// Terms are the terms collected for.
func (e *Examples) Terms() []string { return slices.Clone(e.terms) }

// Full reports whether every term has its examples.
func (e *Examples) Full() bool { return e.max <= 0 || e.full == len(e.terms) }

// Add offers one source text.
func (e *Examples) Add(sourceText string) {
	text := strings.TrimSpace(sourceText)
	if text == "" || e.Full() {
		return
	}
	var p *check.PreparedText
	for i, t := range e.terms {
		if len(e.got[i]) >= e.max || slices.Contains(e.got[i], text) {
			continue
		}
		if p == nil {
			p = check.PrepareText(check.TermText(text))
		}
		if !uses(p, t, e.english) {
			continue
		}
		e.got[i] = append(e.got[i], text)
		if len(e.got[i]) == e.max {
			e.full++
		}
	}
}

// For is the texts collected for term, which compares case-insensitively.
func (e *Examples) For(term string) []string {
	key := strings.ToLower(strings.TrimSpace(term))
	for i, t := range e.terms {
		if strings.ToLower(t) == key {
			return slices.Clone(e.got[i])
		}
	}
	return nil
}
