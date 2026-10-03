package main

import (
	"fmt"
	"slices"
	"strings"
)

// Diagnostic sessions test execution with explicit integration instructions.
// Natural task prompts and their discovery outcomes remain separate.
func pairedDiagnosticSchedule(m PairedManifest) []PairedSession {
	sessions := []PairedSession{}
	for _, agent := range m.Agents {
		for _, condition := range m.Conditions {
			if condition == "baseline" {
				continue
			}
			sessions = append(sessions, PairedSession{
				ID:    fmt.Sprintf("%s-%s-%s-01", m.SmokeTask, agent.Host, condition),
				Agent: agent, Condition: condition, Task: m.SmokeTask, Repetition: 1,
			})
		}
	}
	return sessions
}

// pairedDiagnosticInstruction asks for the arm's own route through the task:
// read the blocks, apply a change set, and report a refusal rather than work
// around it. files are the paths the task edits or creates.
func pairedDiagnosticInstruction(condition string, files []string) string {
	named := strings.Join(files, " and ")
	switch condition {
	case "skill-cli":
		return "This is an explicit CLI integration diagnostic, not a natural discovery test. " +
			"Load the installed kapi skill and follow its content editing workflow. " +
			"Before editing, run kapi context on " + named + " and read the guidance that applies. " +
			"Read the blocks with kapi inspect and make the change with kapi apply, then run " +
			"kapi check on " + named + " --json. " +
			"Report which checks ran or remained unsupported, and every refusal kapi apply returned. " +
			"If the skill or CLI is unavailable, report the failure and stop; do not install or repair it."
	case "mcp":
		return "This is an explicit MCP integration diagnostic, not a natural discovery test. " +
			"Use the configured kapi MCP server. Before editing, read the context:// resource for " + named +
			" through the host's MCP resource interface. Read the blocks with read_blocks and make the change " +
			"with apply_edits, then call check_file on " + named + ". " +
			"Report which checks ran or remained unsupported, and every refusal apply_edits returned. " +
			"If those MCP capabilities are unavailable, report what is exposed and stop. " +
			"Do not install or repair the integration or substitute a hand-written protocol client."
	case "project-free":
		return "This is an explicit CLI integration diagnostic, not a natural discovery test. " +
			"Load the installed kapi-files skill and follow its editing loop: read the blocks of " + named +
			" with kapi-files inspect and make the change with kapi-files apply. " +
			"Report every refusal kapi-files apply returned, including an operation it reported as unsupported. " +
			"If the skill or the command is unavailable, report the failure and stop; do not install or repair it."
	default:
		return ""
	}
}

func pairedDiagnosticFiles(task PairedTask) []string {
	files := slices.Concat(task.spec.Editable, task.spec.Creates)
	if len(files) == 0 {
		return []string{"the files the task names"}
	}
	return files
}

func selectPairedSessions(schedule []PairedSession, selection string) ([]PairedSession, error) {
	if selection == "" {
		return schedule, nil
	}
	wanted := map[string]bool{}
	for value := range strings.SplitSeq(selection, ",") {
		id := strings.TrimSpace(value)
		if id == "" || wanted[id] {
			return nil, fmt.Errorf("empty or duplicate paired session %q", id)
		}
		wanted[id] = true
	}
	selected := []PairedSession{}
	for _, session := range schedule {
		if wanted[session.ID] {
			selected = append(selected, session)
			delete(wanted, session.ID)
		}
	}
	if len(wanted) != 0 {
		return nil, fmt.Errorf("paired session selection contains IDs outside this phase's schedule: %s", selection)
	}
	return selected, nil
}
