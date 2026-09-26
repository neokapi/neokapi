package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
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

// Measure 3's scripted logs, in the product's own operation vocabulary.
//
// One origin log holds what the first agent session suggested. Four machines
// start from a copy of it and each adds its own part of the story, then
// settles: a second session that repeats one rule and suggests a rival for
// another; CI recording a merge to the default branch; a person's corrections;
// and a person's digest, keeping one suggestion and dropping another. The
// measure merges the four logs in every order.

// evalSettleRules names the rules of the scenario by the form each avoids, with
// the form to write instead.
var evalSettleRules = map[string]string{
	"Quick cast": "Quickcast", // merged: established
	"log in":     "sign in",   // corrected toward: established
	"dash board": "dashboard", // kept in the digest: established
	"click here": "select",    // dropped in the digest: never established
	"utilise":    "use",       // merged, but a rival names another form: contested
	"e-mail":     "email",     // merged, then corrected away: contested
	"whitelist":  "allowlist", // seen in two sessions, no person's signal: suggested
}

// evalSettleExpected are the rules the scenario must establish, and no other.
var evalSettleExpected = []string{"Quick cast", "dash board", "log in"}

// evalSettleProject is the project every scripted operation belongs to.
const evalSettleProject = "prj_settle"

// evalSettleEstablished is the status settling gives an established rule.
const evalSettleEstablished = string(contextop.StatusEstablished)

// evalSettleOrigin records the first session's suggestions and returns each
// one's operation id by the form it avoids.
func evalSettleOrigin(ctx context.Context, ledger *contextop.Ledger) (map[string]string, error) {
	ids := map[string]string{}
	for _, term := range slices.Sorted(maps.Keys(evalSettleRules)) {
		r, err := ledger.Append(ctx, evalSettleObserve(evalSettleAgent("s1"), term, evalSettleRules[term]))
		if err != nil {
			return nil, err
		}
		ids[term] = r.ID
	}
	return ids, nil
}

// evalSettleMachine is one machine's own part of the story. It appends to a log
// that already holds the origin, whose suggestions ids names.
type evalSettleMachine struct {
	Name   string
	Record func(ctx context.Context, ledger *contextop.Ledger, ids map[string]string) error
}

// evalSettleMachines are the four machines of the scenario.
var evalSettleMachines = []evalSettleMachine{
	{"second session", func(ctx context.Context, l *contextop.Ledger, _ map[string]string) error {
		for _, r := range []contextop.Record{
			evalSettleObserve(evalSettleAgent("s2"), "whitelist", "allowlist"),
			evalSettleObserve(evalSettleAgent("s2"), "utilise", "employ"),
		} {
			if _, err := l.Append(ctx, r); err != nil {
				return err
			}
		}
		return nil
	}},
	{"merge", func(ctx context.Context, l *contextop.Ledger, ids map[string]string) error {
		for _, term := range []string{"Quick cast", "e-mail", "utilise"} {
			if _, err := l.Append(ctx, contextop.Record{
				Project: evalSettleProject, Actor: contextop.Actor{Kind: contextop.ActorTool, Name: "merge"},
				Kind: contextop.KindSignal, Target: ids[term],
				Signal: &contextop.Signal{Source: contextop.SignalMerge, Commit: "c0ffee1", PR: 412, Preferred: 1},
			}); err != nil {
				return err
			}
		}
		return nil
	}},
	{"corrections", func(ctx context.Context, l *contextop.Ledger, _ map[string]string) error {
		for _, c := range [][2]string{{"log in", "sign in"}, {"email", "e-mail"}} {
			if _, err := l.Append(ctx, contextop.Record{
				Project: evalSettleProject, Actor: evalSettlePerson,
				Kind: contextop.KindCorrect, Correction: &contextop.Correction{From: c[0], To: c[1]},
			}); err != nil {
				return err
			}
		}
		return nil
	}},
	{"digest", func(ctx context.Context, l *contextop.Ledger, ids map[string]string) error {
		if _, err := l.Append(ctx, contextop.Record{Project: evalSettleProject, Actor: evalSettlePerson,
			Kind: contextop.KindKeep, Target: ids["dash board"]}); err != nil {
			return err
		}
		_, err := l.Append(ctx, contextop.Record{Project: evalSettleProject, Actor: evalSettlePerson,
			Kind: contextop.KindDrop, Target: ids["click here"]})
		return err
	}},
}

var evalSettlePerson = contextop.Actor{Kind: contextop.ActorPerson, Name: "reviewer"}

func evalSettleAgent(session string) contextop.Actor {
	return contextop.Actor{Kind: contextop.ActorAgent, Name: "agent", Session: session}
}

func evalSettleObserve(actor contextop.Actor, term, use string) contextop.Record {
	rule := contextop.ObservedRule(use, []string{term})
	return contextop.Record{Project: evalSettleProject, Actor: actor, Kind: contextop.KindObserve,
		Subject: contextop.Subject{Kind: contextop.SubjectTerm, Term: &rule}}
}

// evalSettleOutcome settles a log and reads it back as the status of each
// scripted rule, by the form the rule avoids. A rule recorded more than once
// reports the strongest status any of its records holds.
func evalSettleOutcome(ctx context.Context, ledger *contextop.Ledger) (map[string]string, error) {
	if _, err := ledger.Settle(ctx, contextop.Filter{}); err != nil {
		return nil, err
	}
	records, err := ledger.Records(ctx, contextop.Filter{Subjects: true, Kinds: []contextop.Kind{contextop.KindObserve}})
	if err != nil {
		return nil, err
	}
	rank := map[contextop.Status]int{contextop.StatusEstablished: 3, contextop.StatusContested: 2, contextop.StatusSuggested: 1}
	out := map[string]string{}
	for _, r := range records {
		rule, ok := r.Rule()
		if !ok || rule.Replacement != evalSettleRules[rule.Term] {
			continue
		}
		if held, seen := out[rule.Term]; !seen || rank[r.Status] > rank[contextop.Status(held)] {
			out[rule.Term] = string(r.Status)
		}
	}
	return out, nil
}
