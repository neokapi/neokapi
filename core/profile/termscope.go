package profile

import (
	"maps"
	"strings"

	"github.com/neokapi/neokapi/core/edit"
)

// Sending only the term rules a piece of content can actually use.
//
// A voice profile's vocabulary is resolved at a coordinate, not at a block, so
// the rules that govern a collection govern every block in it — hundreds, in a
// project with a real terms store. Every one of them used to be rendered into
// every prompt, batched or single.
//
// Tokens are the smaller cost. The larger one is attention: a model handed four
// hundred rules attends less to the three that bite the sentence in front of it,
// and the three are the reason the other three hundred and ninety-seven were
// written down.
//
// What is SENT and what is HASHED answer different questions and are allowed to
// differ. The context fingerprint covers every rule at the coordinate because it
// is a staleness detector: a rule added about words this block does not contain
// should still re-check the block, since the block's wording may have been
// chosen under the old set. Narrowing the fingerprint to the rules that bite
// would make a governance change invisible to exactly the content it was meant
// to reach. See GovernanceContext.

// ScopeTermRules returns the rules whose term could appear in the given texts.
//
// Matching is by word, never substring: a rule for "cart" is not sent because
// the text says "cartridge". A bare word match would be too strict in the other
// direction, though — it drops the rule for "cart" from a text saying "carts" —
// so a text word also matches when the term is a prefix of it and the remainder
// is at most inflectionSuffixMax characters.
//
// The asymmetry between the two errors is what sets that bound. Sending a rule
// that turns out not to apply costs a line of prompt; dropping one that does
// apply costs the wording it existed to protect, silently, in a language whose
// morphology the rule author was not thinking about. So the bound is set to
// admit inflection and refuse compounds, and where it is unsure it over-includes.
//
// A rule with no term is kept: it cannot be matched against anything, and
// discarding it here would be a second place that decides which rules are real.
func ScopeTermRules(rules []TermRule, texts ...string) []TermRule {
	if len(rules) == 0 || len(texts) == 0 {
		return rules
	}

	present := wordSet(texts)
	if len(present) == 0 {
		return nil
	}

	out := make([]TermRule, 0, len(rules))
	for _, r := range rules {
		if strings.TrimSpace(r.Term) == "" || termAppears(r.Term, present) {
			out = append(out, r)
		}
	}
	return out
}

// ScopedTermRuleMap is TermRuleMap over the rules the texts can use: the
// projection a prompt renders.
func ScopedTermRuleMap(rules []TermRule, texts ...string) map[string]string {
	return TermRuleMap(ScopeTermRules(rules, texts...))
}

// wordSet is every word in the texts, in the classifier's comparable form, so
// this agrees with edit.ContainsWords about what a word is.
func wordSet(texts []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, t := range texts {
		for w := range edit.Words(t) {
			out[w] = struct{}{}
		}
	}
	return out
}

// termAppears reports whether a term's words are all present.
//
// A term is tried in both of its written forms, joined and split, because the
// classifier's tokenizer folds an intra-word hyphen away: "sign-in" is the
// single word "signin" to it, while a text saying "sign in" has two. That is the
// right answer for classifying an edit — splitting a word IS a change — and the
// wrong one here, where a rule about "sign-in" plainly governs a sentence that
// says "sign in".
//
// Every word must appear for a multi-word term, which is looser than requiring
// them adjacent: "log in" is kept for a text saying "log the user in". The safe
// direction again — the rule is offered and the model decides.
func termAppears(term string, present map[string]struct{}) bool {
	if wordsPresent(term, present) {
		return true
	}
	if split := strings.NewReplacer("-", " ", "\u2010", " ", "\u2011", " ").Replace(term); split != term {
		return wordsPresent(split, present)
	}
	return false
}

func wordsPresent(term string, present map[string]struct{}) bool {
	any := false
	for w := range edit.Words(term) {
		any = true
		if _, exact := present[w]; exact {
			continue
		}
		if !anyWordStartsWith(present, w) {
			return false
		}
	}
	return any
}

// inflectionSuffixMax is how much a text word may add to a term and still count
// as the same word.
//
// Three, because that covers the inflections these rules meet in practice — the
// English "-s", "-es", "-ed", "-ing" and the Norwegian "-en", "-et", "-ene" —
// and stops short of the compounds that would be false matches: "cartridge"
// adds five to "cart", "handlekurv" is not a suffix of "kurv" at all.
//
// It is a heuristic and reads as one. A rule whose term is a prefix of an
// unrelated short word ("plan" in "planet") is sent needlessly, which costs a
// line; the reverse would cost the rule.
const inflectionSuffixMax = 3

func anyWordStartsWith(present map[string]struct{}, prefix string) bool {
	for w := range maps.Keys(present) {
		if len(w) <= len(prefix) || !strings.HasPrefix(w, prefix) {
			continue
		}
		if len([]rune(w))-len([]rune(prefix)) <= inflectionSuffixMax {
			return true
		}
	}
	return false
}
