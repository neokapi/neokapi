package profile

import (
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/tool"
)

// GovernanceContext resolves the provenance fields that record what governed a
// translation producer when it wrote a target: the profile's id, the revision
// pinned at production time, and a fingerprint of the rendered guidance plus the
// terminology in force. Every producer — AI, MT, recycled memory — stamps these
// onto model.Origin through this one function, so a target's governance stamp
// does not depend on which engine wrote it: two producers under one context
// yield the same fingerprint and fall stale together when that context moves.
//
// All three results are empty when the producer was ungoverned — no profile and
// no terminology — which reads correctly as an ad-hoc run.
func GovernanceContext(p *VoiceProfile, rules []TermRule) (profileID, profileVersion, contextFP string) {
	if p != nil {
		profileID = p.ID
		if p.Version > 0 {
			profileVersion = strconv.Itoa(p.Version)
		}
	}
	contextFP = tool.ContextFingerprint(RenderVoiceGuideCompact(p), TermRuleMap(rules), DoNotTranslateTerms(rules))
	return
}

// DoNotTranslateTerms projects term rules into the terms a translation producer
// must keep verbatim: the rules marked do-not-translate, sorted and
// deduplicated so one rule set yields one prompt and one fingerprint however
// the rules were ordered.
//
// It is the second half of the projection TermRuleMap begins, and it exists
// because the two carry different instructions. A rule with a replacement says
// which wording to use; a do-not-translate rule says to leave the term alone,
// and has no replacement by construction. Projected through the map alone it
// was dropped, so the prompt never asked for what the check then demanded and
// the fingerprint did not move when the flag was set.
func DoNotTranslateTerms(rules []TermRule) []string {
	seen := make(map[string]bool, len(rules))
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		term := strings.TrimSpace(r.Term)
		if !r.DoNotTranslate || term == "" || seen[term] {
			continue
		}
		seen[term] = true
		out = append(out, term)
	}
	slices.Sort(out)
	return out
}

// TermRuleMap projects term rules into the term to replacement map a
// translation producer's prompt takes.
//
// It is the one definition of that projection, and it has to be: the map is an
// input to the context fingerprint every producer stamps on what it writes, so
// the staleness gate recomputes it to ask whether the governing context has
// moved since. A second projection that dropped a different rule would make
// every target read as stale against a context nobody changed.
//
// A rule with no replacement is dropped: it says which wording to reach for
// without saying what to reach past, which a prompt line of this shape cannot
// carry. A do-not-translate rule is one of those, and DoNotTranslateTerms
// carries it instead. A
// duplicated term keeps its FIRST rule, so one set of rules resolves the same
// way on every run — a prompt has to be deterministic, and so does a
// fingerprint over it.
func TermRuleMap(rules []TermRule) map[string]string {
	terms := make(map[string]string, len(rules))
	for _, r := range rules {
		if r.Term == "" || r.Replacement == "" {
			continue
		}
		if _, dup := terms[r.Term]; !dup {
			terms[r.Term] = r.Replacement
		}
	}
	return terms
}
