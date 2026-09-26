package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// The agent evaluation.
//
// It measures whether coding agents find, apply and grow a project's context
// through kapi, on a generated documentation repository whose conventions are
// written down as an answer key (eval_fixture.go).
//
//   - Measure 1, agents apply context. The eleven planted conventions are held
//     as rules, loaded by a person. An agent does an ordinary writing task, and
//     the check's failing findings are counted on its first saved version and
//     on its final one, with whether it checked before reporting done.
//   - Measure 2, agents grow context. Nothing is held. The same tasks run in a
//     fresh cell, and what the agent recorded is read back and scored against
//     the key: recall on the names and spellings, precision, and whether the
//     decoy was ever proposed as a rule.
//   - Measure 3, settling. Scripted logs from four machines (sessions, a merge,
//     corrections, a person's digest) are merged in every order, settling after
//     each merge: the expected rules are established, no other, and every rule
//     ends at the same status whatever the order. No model call; a Go test runs
//     it in CI (eval_settle.go).
//   - Measure 4, review is worth it. The grow runs' records become a review
//     sheet a person reads in a few minutes, and the report records their
//     answers.
//
// Every run is one host working one task in a cell of its own, fully isolated.
// Preflight and report make no model call; smoke, apply and grow run live
// sessions against a signed-in subscription under a persistent attempt
// ceiling, with no automatic retries.

const evalSchema = 1

// The two measures a live run belongs to.
const (
	evalMeasureApply = "apply"
	evalMeasureGrow  = "grow"
)

// EvalManifest freezes the evaluation's inputs. Its fingerprint binds every
// saved attempt, so changing a task, a host or a model means a fresh evidence
// directory rather than a silently mixed study.
type EvalManifest struct {
	Schema int    `json:"schema"`
	Study  string `json:"study"`
	// Hosts are the agent hosts under test, one entry each.
	Hosts []PairedAgentSpec `json:"hosts"`
	// Tasks are the writing tasks, by id. Each measure runs every task once
	// per host.
	Tasks []string `json:"tasks"`
	// SmokeTask is the one task a smoke batch runs, on Measure 2, one attempt
	// per host.
	SmokeTask string `json:"smoke_task"`
	// Agents is the `kapi init --agents` value every cell is wired with.
	Agents                string `json:"agents"`
	AttemptTimeoutSeconds int    `json:"attempt_timeout_seconds"`
	MaxTurns              int    `json:"max_turns"`
	Billing               string `json:"billing"`
}

// EvalSession is one live agent session: one host working one task in a cell
// of its own. The session's id is its cell's name.
type EvalSession struct {
	ID      string          `json:"id"`
	Phase   string          `json:"phase"`
	Measure string          `json:"measure"`
	Host    PairedAgentSpec `json:"host"`
	Task    string          `json:"task"`
}

func readEvalManifest(path string) (EvalManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EvalManifest{}, err
	}
	var manifest EvalManifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("decode evaluation manifest: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return manifest, errors.New("manifest must contain one JSON object")
	}
	return manifest, validateEvalManifest(manifest)
}

func validateEvalManifest(m EvalManifest) error {
	if m.Schema != evalSchema {
		return fmt.Errorf("unsupported evaluation schema %d", m.Schema)
	}
	if !pairedIDPattern.MatchString(m.Study) {
		return errors.New("study must be a lowercase path-safe identifier")
	}
	if m.Billing != "subscription-only" {
		return errors.New("billing must be subscription-only; API spending is disabled")
	}
	if len(m.Hosts) == 0 {
		return errors.New("name at least one agent host")
	}
	seenHost := map[string]bool{}
	for _, host := range m.Hosts {
		if host.Host != "claude" && host.Host != "codex" {
			return fmt.Errorf("unsupported host %q", host.Host)
		}
		if seenHost[host.Host] {
			return fmt.Errorf("duplicate host %q", host.Host)
		}
		seenHost[host.Host] = true
		if host.Model == "" || host.Effort == "" {
			return errors.New("each host needs an explicit model and effort")
		}
	}
	if len(m.Tasks) == 0 {
		return errors.New("name at least one task")
	}
	selected := map[string]bool{}
	for _, id := range m.Tasks {
		if _, err := findEvalTask(id); err != nil {
			return err
		}
		if selected[id] {
			return fmt.Errorf("duplicate task %q", id)
		}
		selected[id] = true
	}
	if !selected[m.SmokeTask] {
		return errors.New("smoke_task must name a selected task")
	}
	if m.Agents == "" {
		return errors.New("agents must name the `kapi init --agents` value the cells are wired with")
	}
	if m.AttemptTimeoutSeconds < 1 || m.AttemptTimeoutSeconds > 3600 {
		return errors.New("attempt_timeout_seconds must be between 1 and 3600")
	}
	if m.MaxTurns < 1 {
		return errors.New("max_turns must be positive")
	}
	return nil
}

func (m EvalManifest) attemptTimeout() time.Duration {
	return time.Duration(m.AttemptTimeoutSeconds) * time.Second
}

// evalSchedule lists the live sessions a phase runs, in a fixed order. Smoke is
// one Measure 2 session per host on the smoke task, in cells of its own so a
// smoke run never leaves anything behind in a measured cell.
func evalSchedule(m EvalManifest, phase string) []EvalSession {
	measure, tasks := "", m.Tasks
	switch phase {
	case evalPhaseSmoke:
		measure, tasks = evalMeasureGrow, []string{m.SmokeTask}
	case evalPhaseApply:
		measure = evalMeasureApply
	case evalPhaseGrow:
		measure = evalMeasureGrow
	default:
		return []EvalSession{}
	}
	sessions := []EvalSession{}
	for _, task := range tasks {
		for _, host := range m.Hosts {
			sessions = append(sessions, EvalSession{
				ID: phase + "-" + task + "-" + host.Host, Phase: phase, Measure: measure, Host: host, Task: task,
			})
		}
	}
	return sessions
}

// selectEvalSessions narrows a schedule to named session IDs, keeping the
// schedule's order. It never resets the attempt ceiling.
func selectEvalSessions(schedule []EvalSession, selection string) ([]EvalSession, error) {
	if selection == "" {
		return schedule, nil
	}
	wanted := map[string]bool{}
	for value := range strings.SplitSeq(selection, ",") {
		id := strings.TrimSpace(value)
		if id == "" || wanted[id] {
			return nil, fmt.Errorf("empty or duplicate evaluation session %q", id)
		}
		wanted[id] = true
	}
	selected := []EvalSession{}
	for _, session := range schedule {
		if wanted[session.ID] {
			selected = append(selected, session)
			delete(wanted, session.ID)
		}
	}
	if len(wanted) != 0 {
		return nil, fmt.Errorf("evaluation session selection contains IDs outside this phase's schedule: %s", selection)
	}
	return selected, nil
}
