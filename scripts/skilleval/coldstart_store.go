package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Reading the store back through the surface a person uses.
//
// What the drill reports about a session's effect is whatever `kapi context
// log --json` says, run against that cell's data root by the binary under test.
// Nothing here opens a database or reaches inside the product.

// ColdStartOperation is one entry of the context log, in the fields the drill
// measures. The log carries more; these are what a report needs.
type ColdStartOperation struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Subject string `json:"subject"`
	// Actor is the actor kind the store holds: "agent" or "person".
	Actor string `json:"actor"`
	// AgentName is the agent the store names, empty on a person's entry.
	AgentName string `json:"agent_name,omitempty"`
	Session   string `json:"session"`
	// Evidence is where the actor says it saw the wording. A candidate with
	// none is the one a person cannot judge.
	Evidence []string `json:"evidence"`
	Note     string   `json:"note,omitempty"`
	At       string   `json:"at"`
}

// ColdStartStore is a cell's context log, summarized.
type ColdStartStore struct {
	Operations []ColdStartOperation `json:"operations"`
	// ByKind counts operations by what they did.
	ByKind map[string]int `json:"by_kind"`
	// Candidates are the operations waiting for a person's decision.
	Candidates []ColdStartOperation `json:"candidates"`
	// WithEvidence and WithoutEvidence split the subject-bearing operations by
	// whether the actor said where it saw the wording.
	WithEvidence    int `json:"with_evidence"`
	WithoutEvidence int `json:"without_evidence"`
	// Confirmed and Discarded count a person's decisions.
	Confirmed int `json:"confirmed"`
	Discarded int `json:"discarded"`
	// AgentRecorded and PersonRecorded split the subject-bearing operations by
	// the actor kind the store holds.
	AgentRecorded  int `json:"agent_recorded"`
	PersonRecorded int `json:"person_recorded"`
}

// coldStartReadStore runs the shipped log command and folds the answer.
func coldStartReadStore(ctx context.Context, paths ColdStartPaths) (ColdStartStore, error) {
	store := ColdStartStore{Operations: []ColdStartOperation{}, ByKind: map[string]int{}, Candidates: []ColdStartOperation{}}
	out, err := coldStartRunKapi(ctx, paths, "context", "log", "--json")
	if err != nil {
		return store, err
	}
	var payload struct {
		Operations []struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			Actor struct {
				Kind    string `json:"kind"`
				Name    string `json:"name"`
				Session string `json:"session"`
			} `json:"actor"`
			Subject struct {
				Kind string `json:"kind"`
				Term *struct {
					Term        string `json:"term"`
					Replacement string `json:"replacement"`
				} `json:"term"`
				Voice *struct {
					List string `json:"list"`
					Rule struct {
						Term        string `json:"term"`
						Replacement string `json:"replacement"`
					} `json:"rule"`
				} `json:"voice"`
				Text string `json:"text"`
			} `json:"subject"`
			Correction *struct {
				From string `json:"from"`
				To   string `json:"to"`
			} `json:"correction"`
			Evidence []struct {
				Path  string `json:"path"`
				Quote string `json:"quote"`
			} `json:"evidence"`
			Target string `json:"target"`
			Note   string `json:"note"`
			At     string `json:"at"`
			Status string `json:"status"`
		} `json:"operations"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return store, fmt.Errorf("read the context log: %w", err)
	}
	for _, op := range payload.Operations {
		entry := ColdStartOperation{
			ID: op.ID, Kind: op.Kind, Status: op.Status, Actor: op.Actor.Kind, AgentName: op.Actor.Name,
			Session: op.Actor.Session, Note: op.Note, At: op.At, Evidence: []string{},
		}
		for _, evidence := range op.Evidence {
			entry.Evidence = append(entry.Evidence, strings.TrimSpace(evidence.Path+" "+evidence.Quote))
		}
		switch {
		case op.Correction != nil:
			entry.Subject = fmt.Sprintf("%q became %q", op.Correction.From, op.Correction.To)
		case op.Subject.Term != nil:
			entry.Subject = fmt.Sprintf("term %q, use %q", op.Subject.Term.Term, op.Subject.Term.Replacement)
		case op.Subject.Voice != nil:
			entry.Subject = fmt.Sprintf("voice %s %q, use %q", op.Subject.Voice.List, op.Subject.Voice.Rule.Term, op.Subject.Voice.Rule.Replacement)
		case op.Subject.Text != "":
			entry.Subject = op.Subject.Text
		case op.Target != "":
			entry.Subject = "of #" + op.Target
		}
		store.Operations = append(store.Operations, entry)
	}
	store.fold()
	return store, nil
}

// coldStartBearingKinds are the operations that carry a subject of their own,
// which is what a person judges and what evidence belongs to.
var coldStartBearingKinds = []string{"observe", "propose", "correct"}

// The actor kinds the context log states, as `kapi context log --json` writes
// them and `kapi context log --actor` matches them.
const (
	coldStartActorAgent  = "agent"
	coldStartActorPerson = "person"
)

// coldStartRecordedSince lists the operations one session added, matched by id
// rather than by count, so the report can say who each of them belongs to.
func coldStartRecordedSince(before, after ColdStartStore) []ColdStartOperation {
	held := map[string]bool{}
	for _, op := range before.Operations {
		held[op.ID] = true
	}
	added := []ColdStartOperation{}
	for _, op := range after.Operations {
		if !held[op.ID] {
			added = append(added, op)
		}
	}
	return added
}

// coldStartActorLabel renders one entry's attribution the way the store holds
// it: the actor kind, the agent it names, and the session that groups the run.
func coldStartActorLabel(op ColdStartOperation) string {
	label := op.Actor
	if label == "" {
		label = "unstated"
	}
	if op.AgentName != "" {
		label += " " + op.AgentName
	}
	if op.Session != "" {
		label += ", session " + op.Session
	}
	return label
}

func (s *ColdStartStore) fold() {
	for _, op := range s.Operations {
		s.ByKind[op.Kind]++
		if slices.Contains(coldStartBearingKinds, op.Kind) {
			if len(op.Evidence) != 0 {
				s.WithEvidence++
			} else {
				s.WithoutEvidence++
			}
			switch op.Actor {
			case coldStartActorAgent:
				s.AgentRecorded++
			case coldStartActorPerson:
				s.PersonRecorded++
			}
		}
		switch {
		case op.Kind == "confirm":
			s.Confirmed++
		case op.Kind == "discard":
			s.Discarded++
		case op.Status == "candidate":
			s.Candidates = append(s.Candidates, op)
		}
	}
	sort.Slice(s.Candidates, func(i, j int) bool { return s.Candidates[i].ID < s.Candidates[j].ID })
}

// kindSummary renders the counts by kind in a fixed order, so an empty store
// reads as a row rather than a blank.
func (s ColdStartStore) kindSummary() string {
	parts := []string{}
	for _, kind := range []string{"observe", "propose", "correct", "confirm", "discard", "revert", "widen"} {
		if count := s.ByKind[kind]; count != 0 {
			parts = append(parts, fmt.Sprintf("%s %d", kind, count))
		}
	}
	if len(parts) == 0 {
		return "nothing recorded"
	}
	return strings.Join(parts, ", ")
}

// ColdStartCheck is what `kapi check` reported over a session's own changes.
type ColdStartCheck struct {
	Ran      bool     `json:"ran"`
	Pass     bool     `json:"pass"`
	Findings int      `json:"findings"`
	Score    int      `json:"score"`
	Rules    []string `json:"rules"`
	Messages []string `json:"messages"`
	Error    string   `json:"error,omitempty"`
}

// coldStartCheckChanges runs the shipped check over everything a session
// changed, named the way the skill names it.
func coldStartCheckChanges(ctx context.Context, paths ColdStartPaths, revision string) ColdStartCheck {
	result := ColdStartCheck{Rules: []string{}, Messages: []string{}}
	out, err := coldStartRunKapi(ctx, paths, "check", "--diff-against", revision, "--json")
	if err != nil && strings.TrimSpace(out) == "" {
		result.Error = err.Error()
		return result
	}
	var payload struct {
		Pass    bool `json:"pass"`
		Summary struct {
			Findings int `json:"findings"`
			Score    int `json:"score"`
		} `json:"summary"`
		Findings []struct {
			Rule     string `json:"rule"`
			Severity string `json:"severity"`
			Message  string `json:"message"`
			Location struct {
				File string `json:"file"`
			} `json:"location"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		result.Error = fmt.Sprintf("read the check report: %v", err)
		return result
	}
	result.Ran, result.Pass = true, payload.Pass
	result.Findings, result.Score = payload.Summary.Findings, payload.Summary.Score
	for _, finding := range payload.Findings {
		result.Rules = pairedUnique(result.Rules, finding.Rule)
		result.Messages = append(result.Messages, fmt.Sprintf("%s %s: %s (%s)",
			finding.Severity, finding.Rule, finding.Message, finding.Location.File))
	}
	return result
}
