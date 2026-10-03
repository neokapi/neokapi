package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// pairedCodexCalls is what a Codex session's rollout records of its tool
// calls: every call the model made, a rejected patch and an apply_patch body
// included, which the exec stream (transcript.jsonl) leaves out.
type pairedCodexCalls struct {
	calls   int
	inputs  []pairedCodexCall
	outputs []string
}

type pairedCodexCall struct {
	name  string
	input string
}

// pairedCodexPatchFile is a file an apply_patch body adds, updates or
// deletes.
var pairedCodexPatchFile = regexp.MustCompile(`(?m)^\*\*\* (?:Add|Update|Delete) File: (.+)$`)

// readPairedCodexRollout reads the tool calls of session sessionID from the
// rollout Codex wrote under stateDir.
func readPairedCodexRollout(stateDir, sessionID string) (pairedCodexCalls, error) {
	var out pairedCodexCalls
	if sessionID == "" || strings.ContainsAny(sessionID, "/\\*") {
		return out, errors.New("invalid Codex session identity")
	}
	matches, err := filepath.Glob(filepath.Join(stateDir, "codex", "sessions", "*", "*", "*", "*"+sessionID+".jsonl"))
	if err != nil {
		return out, err
	}
	if len(matches) == 0 {
		return out, errors.New("no rollout for the session")
	}
	seen := map[string]bool{}
	for _, path := range matches {
		file, err := os.Open(path)
		if err != nil {
			return out, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			event := map[string]any{}
			if json.Unmarshal(scanner.Bytes(), &event) != nil || pairedString(event, "type") != "response_item" {
				continue
			}
			payload := pairedObject(event, "payload")
			switch kind := pairedString(payload, "type"); kind {
			case "custom_tool_call", "function_call", "local_shell_call":
				id := pairedString(payload, "call_id")
				if id != "" && seen[id] {
					continue
				}
				seen[id] = true
				out.calls++
				input := pairedString(payload, "input")
				if input == "" {
					input = pairedString(payload, "arguments")
				}
				if input == "" {
					if action := pairedObject(payload, "action"); action != nil {
						data, _ := json.Marshal(action)
						input = string(data)
					}
				}
				out.inputs = append(out.inputs, pairedCodexCall{name: pairedString(payload, "name"), input: input})
			case "custom_tool_call_output", "function_call_output", "local_shell_call_output":
				out.outputs = append(out.outputs, pairedStrings(payload["output"])...)
			}
		}
		scanErr := scanner.Err()
		_ = file.Close()
		if scanErr != nil {
			return out, scanErr
		}
	}
	return out, nil
}

// mergePairedCodexRollout takes a Codex session's tool calls from its
// rollout where the exec stream showed fewer: the count of calls, the
// refusals their outputs carry, and the routes and task files a patch body
// names. A count the stream already has stays; the rollout only adds.
func mergePairedCodexRollout(result *PairedAgentResult, launch PairedLaunch) {
	rollout, err := readPairedCodexRollout(launch.StateDir, result.SessionID)
	if err != nil {
		return
	}
	result.ToolCallsSource = "stream"
	if rollout.calls > result.ToolCalls {
		result.ToolCalls = rollout.calls
		result.ToolCallsSource = "rollout"
	}
	counted := &pairedObserver{result: &PairedAgentResult{}}
	counted.countRefusals(rollout.outputs)
	for code, n := range counted.result.Refusals {
		if n > result.Refusals[code] {
			if result.Refusals == nil {
				result.Refusals = map[string]int{}
			}
			result.Refusals[code] = n
		}
	}
	files := pairedWritableFiles(launch.Task)
	for _, call := range rollout.inputs {
		if call.name != "apply_patch" {
			continue
		}
		for _, match := range pairedCodexPatchFile.FindAllStringSubmatch(call.input, -1) {
			if pairedIsTaskFile(strings.TrimSpace(match[1]), files, launch.Workspace) {
				result.WriteRoutes = pairedUnique(result.WriteRoutes, pairedRouteNative)
			}
		}
	}
}
