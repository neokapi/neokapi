package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A rule set with no do-not-translate rule fingerprints exactly as it always
// has. The golden is the value this repository produced before the flag entered
// the fingerprint, and it is pinned because the cost of moving it is paid by
// users: every governed target whose stored stamp no longer matches reads as
// stale and is drafted again. Widening the fingerprint is allowed to retire the
// targets a change actually governs, and nobody else's.
func TestGovernanceContextUnchangedWithoutDoNotTranslate(t *testing.T) {
	t.Parallel()

	rules := []TermRule{
		{Term: "cart", Replacement: "kurv"},
		{Term: "workspace", Replacement: "arbeidsomrade", Forms: []string{"workspaces"}},
	}

	_, _, fp := GovernanceContext(nil, rules)
	assert.Equal(t, "8acdbd20c5dab3de", fp,
		"a project with no do-not-translate concept keeps the fingerprint it had")
}

// Setting the flag changes what the drafter is asked for, so it moves the
// fingerprint the staleness gate recomputes. Before this, a do-not-translate
// rule carried no replacement, TermRuleMap dropped it, and the governed
// change-set that set the flag marked nothing stale.
func TestGovernanceContextMovesWhenDoNotTranslateChanges(t *testing.T) {
	t.Parallel()

	plain := []TermRule{{Term: "kapi", ConceptID: "c-kapi"}}
	kept := []TermRule{{Term: "kapi", ConceptID: "c-kapi", DoNotTranslate: true}}

	_, _, plainFP := GovernanceContext(nil, plain)
	_, _, keptFP := GovernanceContext(nil, kept)

	assert.NotEqual(t, plainFP, keptFP, "the flag governs generation, so it moves the stamp")
	assert.NotEmpty(t, keptFP, "a do-not-translate rule is governance, not an ad-hoc run")
	assert.Empty(t, plainFP, "a bare term with no replacement still governs nothing")
}

// Clearing the flag returns the fingerprint to what it was, so a flag toggled
// back does not leave content permanently stale.
func TestGovernanceContextReturnsWhenDoNotTranslateIsCleared(t *testing.T) {
	t.Parallel()

	rules := []TermRule{{Term: "save", Replacement: "lagre"}}
	_, _, before := GovernanceContext(nil, rules)

	kept := []TermRule{{Term: "save", Replacement: "lagre", DoNotTranslate: true}}
	_, _, during := GovernanceContext(nil, kept)

	_, _, after := GovernanceContext(nil, rules)

	assert.NotEqual(t, before, during)
	assert.Equal(t, before, after)
}

// DoNotTranslateTerms is the projection the prompt and the fingerprint read:
// the terms of the rules marked do-not-translate, sorted and deduplicated, so
// one rule set renders one prompt and one fingerprint however it was ordered.
func TestDoNotTranslateTerms(t *testing.T) {
	t.Parallel()

	rules := []TermRule{
		{Term: "kapi", DoNotTranslate: true},
		{Term: "save", Replacement: "lagre"},
		{Term: "Bowrain", DoNotTranslate: true},
		{Term: "kapi", DoNotTranslate: true},
		{Term: "   ", DoNotTranslate: true},
		{Term: "", DoNotTranslate: true},
	}

	assert.Equal(t, []string{"Bowrain", "kapi"}, DoNotTranslateTerms(rules))
	assert.Empty(t, DoNotTranslateTerms([]TermRule{{Term: "save", Replacement: "lagre"}}),
		"an ordinary rule is a translation decision, not a do-not-translate claim")
	assert.Empty(t, DoNotTranslateTerms(nil))
}

// A term rule's replacement map and its do-not-translate list answer different
// questions, and a rule belongs to exactly one of them.
func TestTermRuleMapLeavesDoNotTranslateAlone(t *testing.T) {
	t.Parallel()

	rules := []TermRule{
		{Term: "kapi", DoNotTranslate: true},
		{Term: "save", Replacement: "lagre"},
	}

	assert.Equal(t, map[string]string{"save": "lagre"}, TermRuleMap(rules),
		"a do-not-translate rule names no replacement, so the prompt's map cannot carry it")
}
