package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// The product's vocabulary, each part in one place.
//
// The evaluation measures kapi as it stands when the evaluation runs, and kapi
// is changing underneath it: how a person puts a rule in force, what a held
// rule's status is called, which severities fail a check. Everything that
// depends on those words lives in this file, so a change in the product is a
// change here and nowhere else.

// evalHeldStatus is the status the context log gives a rule a person holds.
const evalHeldStatus = "established"

// evalBearingKinds are the operations that carry a subject of their own, which
// is what an agent records and what evidence belongs to.
var evalBearingKinds = []string{"observe", "correct"}

// evalRuleKinds are the bearing operations that can state a rule: an
// observation with a term, and a correction. An observation with no term is a
// note about the prose, which statesRule tells apart by the term it lacks.
var evalRuleKinds = []string{"observe", "correct"}

// The actor kinds the context log states, as `kapi context log --json` writes
// them and `kapi context log --actor` matches them.
const (
	evalActorAgent  = "agent"
	evalActorPerson = "person"
)

// evalFindingFails reports whether a check finding fails. The check report
// says so on each finding: a rule fails unless it is advisory, and an
// unconfirmed suggestion never fails.
func evalFindingFails(finding EvalCheckFinding) bool {
	return finding.Fails && !finding.Suggested
}

// evalLoadHeldRules puts every planted convention's rules in force in a cell,
// as a person would: each rule is recorded as an observation with the term the
// project uses and the form it avoids, and then all of them are kept in one
// call, with the actor set to a person. The decoy is never loaded. It returns
// the rule terms it recorded, which the caller verifies against the log.
func evalLoadHeldRules(ctx context.Context, paths EvalPaths, key EvalKey) ([]string, error) {
	loaded := []string{}
	ids := []string{}
	for _, convention := range key.planted() {
		for _, rule := range convention.Rules {
			out, err := evalRunKapiAs(ctx, paths, evalActorPerson, "context", "observe", convention.Summary,
				"--term", rule.Use, "--instead-of", rule.Term, "--json")
			if err != nil {
				return loaded, fmt.Errorf("record %q for %s: %w", rule.Term, convention.ID, err)
			}
			var recorded struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(out), &recorded); err != nil {
				return loaded, fmt.Errorf("read the record of %q: %w: %s", rule.Term, err, strings.TrimSpace(out))
			}
			if recorded.ID == "" {
				return loaded, fmt.Errorf("the record of %q carries no id: %s", rule.Term, strings.TrimSpace(out))
			}
			ids = append(ids, recorded.ID)
			loaded = append(loaded, rule.Term)
		}
	}
	if len(ids) == 0 {
		return loaded, nil
	}
	args := append([]string{"context", "keep"}, ids...)
	if _, err := evalRunKapiAs(ctx, paths, evalActorPerson, append(args, "--json")...); err != nil {
		return loaded, fmt.Errorf("keep the %d recorded rules: %w", len(ids), err)
	}
	return loaded, nil
}

// evalHeldTerms reads back which rule terms the store holds at the held status
// and were put there by a person, so a cell is never run over rules that did
// not load.
func evalHeldTerms(store EvalStore) []string {
	held := []string{}
	for _, op := range store.Operations {
		if op.Actor == evalActorPerson && op.Status == evalHeldStatus && op.Term != "" &&
			slices.Contains(evalRuleKinds, op.Kind) {
			held = pairedUnique(held, strings.ToLower(op.Term))
		}
	}
	return held
}
