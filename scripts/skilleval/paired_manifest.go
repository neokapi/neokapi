package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"regexp"
	"time"
)

const pairedSchema = 1

var pairedIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// PairedManifest freezes the inputs shared by every condition in the study.
type PairedManifest struct {
	Schema                int               `json:"schema"`
	Study                 string            `json:"study"`
	Seed                  int64             `json:"seed"`
	Agents                []PairedAgentSpec `json:"agents"`
	Conditions            []string          `json:"conditions"`
	Tasks                 []string          `json:"tasks"`
	SmokeTask             string            `json:"smoke_task"`
	Repetitions           int               `json:"repetitions"`
	AttemptTimeoutSeconds int               `json:"attempt_timeout_seconds"`
	MaxTurns              int               `json:"max_turns"`
	Billing               string            `json:"billing"`
}

// PairedSession identifies a task/model/repetition and one mutually exclusive integration.
type PairedSession struct {
	ID         string          `json:"id"`
	Agent      PairedAgentSpec `json:"agent"`
	Condition  string          `json:"condition"`
	Task       string          `json:"task"`
	Repetition int             `json:"repetition"`
}

func readPairedManifest(path string) (PairedManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PairedManifest{}, err
	}
	var manifest PairedManifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("decode paired manifest: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return manifest, errors.New("manifest must contain one JSON object")
	}
	return manifest, validatePairedManifest(manifest)
}

func validatePairedManifest(m PairedManifest) error {
	if m.Schema != pairedSchema {
		return fmt.Errorf("unsupported paired schema %d", m.Schema)
	}
	if !pairedIDPattern.MatchString(m.Study) {
		return errors.New("study must be a lowercase path-safe identifier")
	}
	if m.Billing != "subscription-only" {
		return errors.New("billing must be subscription-only; API spending is disabled")
	}
	if len(m.Agents) != 2 {
		return errors.New("study requires exactly two agent hosts")
	}
	hosts := map[string]bool{}
	for _, a := range m.Agents {
		if a.Host != "claude" && a.Host != "codex" {
			return fmt.Errorf("unsupported host %q", a.Host)
		}
		if hosts[a.Host] {
			return fmt.Errorf("duplicate host %q", a.Host)
		}
		hosts[a.Host] = true
		if a.Model == "" || a.Effort == "" {
			return errors.New("each agent needs an explicit model and effort")
		}
	}
	if len(m.Conditions) != 3 {
		return errors.New("study requires baseline, skill-cli and mcp")
	}
	conditions := map[string]bool{}
	for _, condition := range m.Conditions {
		if condition != "baseline" && condition != "skill-cli" && condition != "mcp" {
			return fmt.Errorf("unsupported condition %q", condition)
		}
		if conditions[condition] {
			return fmt.Errorf("duplicate condition %q", condition)
		}
		conditions[condition] = true
	}
	known := map[string]bool{}
	for _, task := range pairedTasks() {
		known[task.ID] = true
	}
	selected := map[string]bool{}
	for _, task := range m.Tasks {
		if !known[task] {
			return fmt.Errorf("unknown task %q", task)
		}
		if selected[task] {
			return fmt.Errorf("duplicate task %q", task)
		}
		selected[task] = true
	}
	if len(selected) == 0 || !selected[m.SmokeTask] {
		return errors.New("smoke_task must name a selected task")
	}
	if m.Repetitions < 1 || m.Repetitions > 100 {
		return errors.New("repetitions must be between 1 and 100")
	}
	if m.AttemptTimeoutSeconds < 1 || m.AttemptTimeoutSeconds > 3600 {
		return errors.New("attempt_timeout_seconds must be between 1 and 3600")
	}
	if m.MaxTurns < 1 {
		return errors.New("max_turns must be positive")
	}
	return nil
}

func pairedSchedule(m PairedManifest, phase string) []PairedSession {
	if phase == "diagnostic" {
		return pairedDiagnosticSchedule(m)
	}
	tasks, repetitions := m.Tasks, m.Repetitions
	if phase == "smoke" {
		tasks, repetitions = []string{m.SmokeTask}, 1
	}
	// Shuffle task/host/repetition blocks, then conditions within each block.
	// All conditions therefore share the same task and replicate without a fixed arm order.
	blocks := [][]PairedSession{}
	rng := rand.New(rand.NewSource(m.Seed))
	for _, task := range tasks {
		for _, agent := range m.Agents {
			for repetition := 1; repetition <= repetitions; repetition++ {
				block := make([]PairedSession, 0, len(m.Conditions))
				for _, condition := range m.Conditions {
					id := fmt.Sprintf("%s-%s-%s-%02d", task, agent.Host, condition, repetition)
					block = append(block, PairedSession{
						ID: id, Agent: agent, Condition: condition, Task: task, Repetition: repetition,
					})
				}
				rng.Shuffle(len(block), func(i, j int) { block[i], block[j] = block[j], block[i] })
				blocks = append(blocks, block)
			}
		}
	}
	rng.Shuffle(len(blocks), func(i, j int) { blocks[i], blocks[j] = blocks[j], blocks[i] })
	schedule := []PairedSession{}
	for _, block := range blocks {
		schedule = append(schedule, block...)
	}
	return schedule
}

func pairedHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (m PairedManifest) attemptTimeout() time.Duration {
	return time.Duration(m.AttemptTimeoutSeconds) * time.Second
}
