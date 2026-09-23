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
const evalHeldStatus = "confirmed"

// evalCandidateStatus is the status of a record nobody has decided on.
const evalCandidateStatus = "candidate"

// evalBearingKinds are the operations that carry a subject of their own, which
// is what an agent records and what evidence belongs to.
var evalBearingKinds = []string{"observe", "propose", "correct"}

// evalRuleKinds are the bearing operations that state a rule, as opposed to an
// observation about the prose.
var evalRuleKinds = []string{"propose", "correct"}

// The actor kinds the context log states, as `kapi context log --json` writes
// them and `kapi context log --actor` matches them.
const (
	evalActorAgent  = "agent"
	evalActorPerson = "person"
)

// evalFindingFails reports whether a check finding at this severity fails.
// A rule's severity decides it: minor and neutral report, everything else
// fails, and an unconfirmed suggestion reports at neutral.
func evalFindingFails(severity string) bool {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "minor", "neutral", "info":
		return false
	}
	return true
}

// evalLoadHeldRules puts every planted convention's rules in force in a cell,
// as a person would: each rule is proposed and then confirmed, with the actor
// set to a person. The decoy is never loaded. It returns what the context log
// holds afterwards, which the caller verifies.
func evalLoadHeldRules(ctx context.Context, paths EvalPaths, key EvalKey) ([]string, error) {
	loaded := []string{}
	for _, convention := range key.planted() {
		for _, rule := range convention.Rules {
			out, err := evalRunKapiAs(ctx, paths, evalActorPerson, "context", "propose", rule.Term,
				"--use", rule.Use, "--list", "forbidden", "--note", convention.Summary, "--json")
			if err != nil {
				return loaded, fmt.Errorf("propose %q for %s: %w", rule.Term, convention.ID, err)
			}
			var proposed struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(out), &proposed); err != nil || proposed.ID == "" {
				return loaded, fmt.Errorf("read the proposal of %q: %v: %s", rule.Term, err, strings.TrimSpace(out))
			}
			if _, err := evalRunKapiAs(ctx, paths, evalActorPerson, "context", "confirm", proposed.ID, "--json"); err != nil {
				return loaded, fmt.Errorf("confirm %q for %s: %w", rule.Term, convention.ID, err)
			}
			loaded = append(loaded, rule.Term)
		}
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
