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

// The cold-start drill.
//
// One question: an agent given an ordinary writing task, in a project whose
// context is empty, with the shipped skill and MCP server and nothing else,
// does it grow that context by itself in a way a person would confirm?
//
// The drill answers it in five phases. Preflight builds the fixture repository,
// runs the real `kapi init` over it and establishes that the server an agent
// host would start is this checkout's binary. Session one gives each host a
// writing task that never mentions kapi. Review puts every candidate the store
// holds in front of a person, whose confirmations and discards go through the
// shipped commands. Session two runs a fresh task in the same repository, over
// whatever the person left in force. The report renders three measures per
// host, from saved attempts, with no model calls.
//
// A session that recorded nothing is a row in the report. That outcome is the
// kill criterion for the premise, so hiding it would defeat the drill.

const coldStartSchema = 1

// coldStartStages are the two live stages, in the order they run.
const (
	coldStartStageOne = "one"
	coldStartStageTwo = "two"
)

// ColdStartManifest freezes the drill's inputs. Its fingerprint binds every
// saved attempt, so changing a task or a model means a fresh evidence
// directory rather than a silently mixed study.
type ColdStartManifest struct {
	Schema int    `json:"schema"`
	Study  string `json:"study"`
	// Hosts are the agent hosts under test, one entry each.
	Hosts []PairedAgentSpec `json:"hosts"`
	// Tasks are the session-one tasks. Exactly one of them carries a person's
	// wording correction in its prompt.
	Tasks []string `json:"tasks"`
	// SmokeTask is the one session-one task a smoke batch runs, one attempt per
	// host.
	SmokeTask string `json:"smoke_task"`
	// SessionTwoTask is the fresh task every cell runs in its second session.
	SessionTwoTask string `json:"session_two_task"`
	// Agents is the `kapi init --agents` value the fixture is wired with.
	Agents                string `json:"agents"`
	AttemptTimeoutSeconds int    `json:"attempt_timeout_seconds"`
	MaxTurns              int    `json:"max_turns"`
	Billing               string `json:"billing"`
}

// ColdStartCell is one host working one session-one task in its own copy of the
// fixture, with its own workspace. Session two runs in the same cell, so the
// second session sees whatever the first recorded and the person confirmed.
type ColdStartCell struct {
	ID   string          `json:"id"`
	Host PairedAgentSpec `json:"host"`
	Task string          `json:"task"`
}

// ColdStartSession is one live agent session: a cell at a stage.
type ColdStartSession struct {
	ID    string          `json:"id"`
	Cell  string          `json:"cell"`
	Stage string          `json:"stage"`
	Host  PairedAgentSpec `json:"host"`
	Task  string          `json:"task"`
}

func readColdStartManifest(path string) (ColdStartManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ColdStartManifest{}, err
	}
	var manifest ColdStartManifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("decode cold-start manifest: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return manifest, errors.New("manifest must contain one JSON object")
	}
	return manifest, validateColdStartManifest(manifest)
}

func validateColdStartManifest(m ColdStartManifest) error {
	if m.Schema != coldStartSchema {
		return fmt.Errorf("unsupported cold-start schema %d", m.Schema)
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
	if err := validateColdStartTasks(m); err != nil {
		return err
	}
	if m.Agents == "" {
		return errors.New("agents must name the `kapi init --agents` value the fixture is wired with")
	}
	if m.AttemptTimeoutSeconds < 1 || m.AttemptTimeoutSeconds > 3600 {
		return errors.New("attempt_timeout_seconds must be between 1 and 3600")
	}
	if m.MaxTurns < 1 {
		return errors.New("max_turns must be positive")
	}
	return nil
}

// validateColdStartTasks holds the drill's one substantive rule about its task
// set: at least three session-one tasks, exactly one of which puts a person's
// wording correction in front of the agent.
func validateColdStartTasks(m ColdStartManifest) error {
	if len(m.Tasks) < 3 {
		return errors.New("session one needs at least three tasks")
	}
	selected := map[string]bool{}
	corrections := []string{}
	for _, id := range m.Tasks {
		task, err := findColdStartTask(id)
		if err != nil {
			return err
		}
		if task.Stage != coldStartStageOne {
			return fmt.Errorf("task %q is a stage %s task and cannot run in session one", id, task.Stage)
		}
		if selected[id] {
			return fmt.Errorf("duplicate task %q", id)
		}
		selected[id] = true
		if task.Correction {
			corrections = append(corrections, id)
		}
	}
	if len(corrections) != 1 {
		return fmt.Errorf("session one needs exactly one task carrying a wording correction, and names %d", len(corrections))
	}
	if !selected[m.SmokeTask] {
		return errors.New("smoke_task must name a selected session-one task")
	}
	second, err := findColdStartTask(m.SessionTwoTask)
	if err != nil {
		return err
	}
	if second.Stage != coldStartStageTwo {
		return fmt.Errorf("session_two_task %q is a stage %s task", m.SessionTwoTask, second.Stage)
	}
	return nil
}

func (m ColdStartManifest) attemptTimeout() time.Duration {
	return time.Duration(m.AttemptTimeoutSeconds) * time.Second
}

// coldStartCells lists every cell the manifest describes, in a fixed order.
// `smoke` narrows the list to the smoke task.
func coldStartCells(m ColdStartManifest, phase string) []ColdStartCell {
	tasks := m.Tasks
	if phase == coldStartPhaseSmoke {
		tasks = []string{m.SmokeTask}
	}
	cells := []ColdStartCell{}
	for _, task := range tasks {
		for _, host := range m.Hosts {
			cells = append(cells, ColdStartCell{ID: task + "-" + host.Host, Host: host, Task: task})
		}
	}
	return cells
}

// coldStartSchedule lists the live sessions a phase would run.
func coldStartSchedule(m ColdStartManifest, phase string) []ColdStartSession {
	stage, task := coldStartStageOne, ""
	if phase == coldStartPhaseSessionTwo {
		stage, task = coldStartStageTwo, m.SessionTwoTask
	}
	sessions := []ColdStartSession{}
	for _, cell := range coldStartCells(m, phase) {
		id := cell.Task
		if task != "" {
			id = task
		}
		sessions = append(sessions, ColdStartSession{
			ID:    cell.ID + "-" + stage,
			Cell:  cell.ID,
			Stage: stage,
			Host:  cell.Host,
			Task:  id,
		})
	}
	return sessions
}

// selectColdStartSessions narrows a schedule to named session IDs, keeping the
// schedule's order. It never resets the attempt ceiling.
func selectColdStartSessions(schedule []ColdStartSession, selection string) ([]ColdStartSession, error) {
	if selection == "" {
		return schedule, nil
	}
	wanted := map[string]bool{}
	for value := range strings.SplitSeq(selection, ",") {
		id := strings.TrimSpace(value)
		if id == "" || wanted[id] {
			return nil, fmt.Errorf("empty or duplicate cold-start session %q", id)
		}
		wanted[id] = true
	}
	selected := []ColdStartSession{}
	for _, session := range schedule {
		if wanted[session.ID] {
			selected = append(selected, session)
			delete(wanted, session.ID)
		}
	}
	if len(wanted) != 0 {
		return nil, fmt.Errorf("cold-start session selection contains IDs outside this phase's schedule: %s", selection)
	}
	return selected, nil
}
