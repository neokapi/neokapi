package main

import (
	"fmt"
	"strings"
)

// Diagnostic sessions test execution with explicit integration instructions.
// Natural task prompts and their discovery outcomes remain separate.
func pairedDiagnosticSchedule(m PairedManifest) []PairedSession {
	sessions := []PairedSession{}
	for _, agent := range m.Agents {
		for _, condition := range []string{"skill-cli", "mcp"} {
			sessions = append(sessions, PairedSession{
				ID:    fmt.Sprintf("%s-%s-%s-01", m.SmokeTask, agent.Host, condition),
				Agent: agent, Condition: condition, Task: m.SmokeTask, Repetition: 1,
			})
		}
	}
	return sessions
}

func pairedDiagnosticInstruction(condition string) string {
	switch condition {
	case "skill-cli":
		return "This is an explicit CLI integration diagnostic, not a natural discovery test. " +
			"Load the installed kapi skill and follow its content editing workflow. " +
			"Before editing, run kapi context content/en/page.json and read the resolved audience guidance. " +
			"Use kapi inspect and kapi apply to edit the requested content, then run " +
			"kapi check content/en/page.json --json without profile overrides. " +
			"Report the resolved audience and which checks ran or remained unsupported. " +
			"If the skill or CLI is unavailable, report the failure and stop; do not install or repair it."
	case "mcp":
		return "This is an explicit MCP integration diagnostic, not a natural discovery test. " +
			"Use the configured kapi MCP server. Before editing, read its " +
			"context://content/en/page.json resource through the host's MCP resource interface. " +
			"After saving the requested edit, call the kapi MCP check_file tool with " +
			"file=content/en/page.json and no profile_file or profile_pack override. " +
			"Report the resolved audience and which checks ran or remained unsupported. " +
			"If those MCP capabilities are unavailable, report what is exposed and stop. " +
			"Do not install or repair the integration, invoke kapi through the shell, " +
			"or substitute a hand-written protocol client."
	default:
		return ""
	}
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
