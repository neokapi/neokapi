package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Reading a cell back through the surfaces a person uses.
//
// What the evaluation reports about a session's effect is whatever `kapi
// context log --json` and `kapi check --json` say, run in that cell by the
// binary under test. Nothing here opens a database or reaches inside the
// product.

// EvalOperation is one entry of the context log, in the fields the evaluation
// scores. The log carries more; these are what a report needs.
type EvalOperation struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	// Subject renders the entry for a reader.
	Subject string `json:"subject"`
	// Term and Replacement are the rule's two sides, on an entry that states
	// one: a proposed term or voice rule, or a correction's from and to.
	Term        string `json:"term,omitempty"`
	Replacement string `json:"replacement,omitempty"`
	// List is the voice-profile list a voice rule was proposed into.
	List string `json:"list,omitempty"`
	// Text is an observation's own words.
	Text string `json:"text,omitempty"`
	// Actor is the actor kind the store holds: "agent" or "person".
	Actor string `json:"actor"`
	// AgentName is the agent the store names, empty on a person's entry.
	AgentName string `json:"agent_name,omitempty"`
	Session   string `json:"session"`
	// Evidence is where the actor says it saw the wording, one path and quote
	// per entry. A record with none is the one a person cannot judge.
	Evidence []EvalEvidence `json:"evidence"`
	Note     string         `json:"note,omitempty"`
	At       string         `json:"at"`
}

// EvalEvidence is one place an actor says it saw the wording.
type EvalEvidence struct {
	Path  string `json:"path,omitempty"`
	Quote string `json:"quote,omitempty"`
}

func (e EvalEvidence) String() string { return strings.TrimSpace(e.Path + " " + e.Quote) }

// bearing reports an entry that carries a subject of its own.
func (op EvalOperation) bearing() bool { return slices.Contains(evalBearingKinds, op.Kind) }

// statesRule reports an entry that states a rule rather than an observation.
func (op EvalOperation) statesRule() bool {
	return slices.Contains(evalRuleKinds, op.Kind) && (op.Term != "" || op.Replacement != "")
}

// EvalStore is a cell's context log, summarized.
type EvalStore struct {
	Operations []EvalOperation `json:"operations"`
	// ByKind counts operations by what they did.
	ByKind map[string]int `json:"by_kind"`
}

// evalReadStore runs the shipped log command and folds the answer.
func evalReadStore(ctx context.Context, paths EvalPaths) (EvalStore, error) {
	out, err := evalRunKapi(ctx, paths, "context", "log", "--json")
	if err != nil {
		return EvalStore{Operations: []EvalOperation{}, ByKind: map[string]int{}}, err
	}
	return evalParseStore([]byte(out))
}

// evalLogPayload is the shape of `kapi context log --json`, in the fields the
// evaluation reads.
type evalLogPayload struct {
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
		Evidence []EvalEvidence `json:"evidence"`
		Target   string         `json:"target"`
		Note     string         `json:"note"`
		At       string         `json:"at"`
		Status   string         `json:"status"`
	} `json:"operations"`
}

// evalParseStore reads a saved or live context log.
func evalParseStore(data []byte) (EvalStore, error) {
	store := EvalStore{Operations: []EvalOperation{}, ByKind: map[string]int{}}
	var payload evalLogPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return store, fmt.Errorf("read the context log: %w", err)
	}
	for _, op := range payload.Operations {
		entry := EvalOperation{
			ID: op.ID, Kind: op.Kind, Status: op.Status, Actor: op.Actor.Kind, AgentName: op.Actor.Name,
			Session: op.Actor.Session, Note: op.Note, At: op.At, Evidence: []EvalEvidence{},
		}
		for _, evidence := range op.Evidence {
			if evidence.String() != "" {
				entry.Evidence = append(entry.Evidence, evidence)
			}
		}
		switch {
		case op.Correction != nil:
			entry.Term, entry.Replacement = op.Correction.From, op.Correction.To
			entry.Subject = fmt.Sprintf("%q became %q", op.Correction.From, op.Correction.To)
		case op.Subject.Term != nil:
			entry.Term, entry.Replacement = op.Subject.Term.Term, op.Subject.Term.Replacement
			entry.Subject = fmt.Sprintf("term %q, use %q", entry.Term, entry.Replacement)
		case op.Subject.Voice != nil:
			entry.Term, entry.Replacement = op.Subject.Voice.Rule.Term, op.Subject.Voice.Rule.Replacement
			entry.List = op.Subject.Voice.List
			entry.Subject = fmt.Sprintf("voice %s %q, use %q", entry.List, entry.Term, entry.Replacement)
		case op.Subject.Text != "":
			entry.Text = op.Subject.Text
			entry.Subject = op.Subject.Text
		case op.Target != "":
			entry.Subject = "of #" + op.Target
		}
		store.Operations = append(store.Operations, entry)
		store.ByKind[entry.Kind]++
	}
	return store, nil
}

// evalRecordedSince lists the operations one session added, matched by id
// rather than by count, so the report can say who each of them belongs to.
func evalRecordedSince(before, after EvalStore) []EvalOperation {
	held := map[string]bool{}
	for _, op := range before.Operations {
		held[op.ID] = true
	}
	added := []EvalOperation{}
	for _, op := range after.Operations {
		if !held[op.ID] {
			added = append(added, op)
		}
	}
	return added
}

// evalActorLabel renders one entry's attribution the way the store holds it:
// the actor kind, the agent it names, and the session that groups the run.
func evalActorLabel(op EvalOperation) string {
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

// EvalCheck is what `kapi check` reported over a version of a session's work.
type EvalCheck struct {
	Ran      bool               `json:"ran"`
	Pass     bool               `json:"pass"`
	Score    int                `json:"score"`
	Findings []EvalCheckFinding `json:"findings"`
	Error    string             `json:"error,omitempty"`
}

// EvalCheckFinding is one finding, in the fields the evaluation scores.
type EvalCheckFinding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	File     string `json:"file"`
	// Term is the rule term a vocabulary finding names, which is how a finding
	// is traced back to a convention.
	Term string `json:"term,omitempty"`
}

// failing lists the findings whose severity fails.
func (c EvalCheck) failing() []EvalCheckFinding {
	out := []EvalCheckFinding{}
	for _, finding := range c.Findings {
		if evalFindingFails(finding.Severity) {
			out = append(out, finding)
		}
	}
	return out
}

// evalCheckAgainst runs the shipped check over everything that differs from a
// revision, named the way the skill names it.
func evalCheckAgainst(ctx context.Context, paths EvalPaths, revision string) EvalCheck {
	out, err := evalRunKapi(ctx, paths, "check", "--diff-against", revision, "--json")
	if err != nil && strings.TrimSpace(out) == "" {
		return EvalCheck{Findings: []EvalCheckFinding{}, Error: err.Error()}
	}
	return evalParseCheck([]byte(out))
}

// evalParseCheck reads a saved or live check report. A check exits non-zero
// when its gate fails, so the report is read whatever the exit status was.
func evalParseCheck(data []byte) EvalCheck {
	result := EvalCheck{Findings: []EvalCheckFinding{}}
	var payload struct {
		Pass    bool `json:"pass"`
		Summary struct {
			Score int `json:"score"`
		} `json:"summary"`
		Findings []struct {
			Rule     string `json:"rule"`
			Severity string `json:"severity"`
			Message  string `json:"message"`
			Location struct {
				File string `json:"file"`
			} `json:"location"`
			Metadata map[string]any `json:"metadata"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		result.Error = fmt.Sprintf("read the check report: %v", err)
		return result
	}
	result.Ran, result.Pass, result.Score = true, payload.Pass, payload.Summary.Score
	for _, finding := range payload.Findings {
		term, _ := finding.Metadata["term"].(string)
		result.Findings = append(result.Findings, EvalCheckFinding{
			Rule: finding.Rule, Severity: finding.Severity, Message: finding.Message,
			File: finding.Location.File, Term: term,
		})
	}
	return result
}
