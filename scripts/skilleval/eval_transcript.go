package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Reading a saved session back.
//
// Every measure the evaluation reports about a session comes from the transcript on
// disk, so the report is reproducible from saved attempts with no model call
// and a scoring change can be applied to attempts already run.
//
// What a call is depends on the surface it arrived on. Over MCP a tool has a
// name. From a shell it is a command line, and the same four habits are spelled
// as kapi subcommands. Both are classified into the same small set, because the
// question is what the agent did rather than which door it used.

// Call kinds, in the order the habits run.
const (
	evalKindAsk    = "ask"
	evalKindRecord = "record"
	evalKindCheck  = "check"
	evalKindWrite  = "write"
	evalKindOther  = "other"
)

// EvalCall is one tool call, in the order the transcript holds it.
type EvalCall struct {
	Order   int    `json:"order"`
	Surface string `json:"surface"`
	Tool    string `json:"tool"`
	Kind    string `json:"kind"`
	Detail  string `json:"detail,omitempty"`
}

// EvalTranscript is everything a saved session says.
type EvalTranscript struct {
	Host        string     `json:"host"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	ActualModel string     `json:"actual_model,omitempty"`
	SessionID   string     `json:"session_id,omitempty"`
	Calls       []EvalCall `json:"calls"`
	// SkillLoaded reports the shipped skill being loaded by name.
	SkillLoaded bool `json:"skill_loaded"`
	// AskedBeforeWriting reports a context read that came before the first
	// change to a file. A session that wrote nothing has nothing to ask before,
	// and reports false.
	AskedBeforeWriting bool   `json:"asked_before_writing"`
	InputTokens        int64  `json:"input_tokens"`
	OutputTokens       int64  `json:"output_tokens"`
	UsageObserved      bool   `json:"usage_observed"`
	RateLimited        bool   `json:"rate_limited"`
	FinalText          string `json:"final_text,omitempty"`
}

// kapiCalls returns the calls that reached kapi, by either surface.
func (t EvalTranscript) kapiCalls() []EvalCall {
	out := []EvalCall{}
	for _, call := range t.Calls {
		if call.Surface == "mcp" || call.Surface == "cli" {
			out = append(out, call)
		}
	}
	return out
}

// ofKind returns the calls of one kind.
func (t EvalTranscript) ofKind(kind string) []EvalCall {
	out := []EvalCall{}
	for _, call := range t.Calls {
		if call.Kind == kind {
			out = append(out, call)
		}
	}
	return out
}

// toolNames lists the distinct tool names of a call set, in first-seen order.
func evalToolNames(calls []EvalCall) []string {
	out := []string{}
	for _, call := range calls {
		out = pairedUnique(out, call.Tool)
	}
	return out
}

// scanEvalTranscript reads a saved stream and classifies every call.
func scanEvalTranscript(path, host string) (EvalTranscript, error) {
	file, err := os.Open(path)
	if err != nil {
		return EvalTranscript{}, err
	}
	defer file.Close()
	return readEvalTranscript(file, host)
}

func readEvalTranscript(reader io.Reader, host string) (EvalTranscript, error) {
	transcript := EvalTranscript{Host: host, Status: "incomplete", Calls: []EvalCall{}}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	models := map[string]bool{}
	for scanner.Scan() {
		if len(strings.TrimSpace(scanner.Text())) == 0 {
			continue
		}
		event := map[string]any{}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			transcript.Status = "malformed_stream"
			return transcript, fmt.Errorf("invalid agent event: %w", err)
		}
		switch host {
		case "claude":
			readEvalClaudeEvent(event, &transcript, models)
		case "codex":
			readEvalCodexEvent(event, &transcript, models)
		default:
			return transcript, errors.New("unknown agent host")
		}
	}
	if err := scanner.Err(); err != nil {
		transcript.Status = "malformed_stream"
		return transcript, err
	}
	if len(models) == 1 {
		for model := range models {
			transcript.ActualModel = model
		}
	}
	if len(models) > 1 {
		transcript.Status = "model_mismatch"
	}
	transcript.AskedBeforeWriting = evalAskedFirst(transcript.Calls)
	return transcript, nil
}

func readEvalClaudeEvent(event map[string]any, t *EvalTranscript, models map[string]bool) {
	switch pairedString(event, "type") {
	case "system":
		if pairedString(event, "subtype") == "init" {
			t.SessionID = pairedString(event, "session_id")
			if model := pairedString(event, "model"); model != "" {
				models[model] = true
			}
		}
	case "assistant":
		message := pairedObject(event, "message")
		if model := pairedString(message, "model"); model != "" && model != "<synthetic>" {
			models[model] = true
		}
		content, ok := message["content"].([]any)
		if !ok {
			return
		}
		for _, raw := range content {
			part, ok := raw.(map[string]any)
			if !ok || pairedString(part, "type") != "tool_use" {
				continue
			}
			t.addClaudeCall(pairedString(part, "name"), pairedObject(part, "input"))
		}
	case "result":
		t.Status = "completed"
		t.FinalText = pairedString(event, "result")
		usage := pairedObject(event, "usage")
		t.UsageObserved = usage != nil
		t.InputTokens = pairedNumber(usage, "input_tokens") + pairedNumber(usage, "cache_read_input_tokens") + pairedNumber(usage, "cache_creation_input_tokens")
		t.OutputTokens = pairedNumber(usage, "output_tokens")
		if failed, _ := event["is_error"].(bool); failed || pairedString(event, "subtype") != "success" {
			t.Status = "agent_failed"
			t.Error = pairedString(event, "subtype")
			if pairedRateLimited(t.FinalText + " " + t.Error) {
				t.Status, t.RateLimited = "rate_limited", true
			}
		}
	}
}

func (t *EvalTranscript) addClaudeCall(name string, input map[string]any) {
	switch {
	case name == "Bash":
		t.addShellCall(pairedString(input, "command"))
	case strings.HasPrefix(name, "mcp__kapi__"):
		t.add("mcp", strings.TrimPrefix(name, "mcp__kapi__"), "")
	case strings.Contains(name, "McpResource"):
		// A host reads an MCP resource through a tool of its own, naming the
		// server it wants. Only kapi's answers count as a context read.
		if pairedString(input, "server") == "kapi" || strings.HasPrefix(pairedString(input, "uri"), "context://") {
			t.add("mcp", evalResourceTool(pairedString(input, "uri")), pairedString(input, "uri"))
			return
		}
		t.add("host", name, pairedString(input, "server"))
	case name == "Skill":
		skill := pairedString(input, "skill")
		if skill == "" {
			skill = pairedString(input, "command")
		}
		if strings.Contains(skill, "kapi") {
			t.SkillLoaded = true
		}
		t.add("host", "Skill", skill)
	case name == "Read" && evalIsSkillPath(pairedString(input, "file_path")):
		t.SkillLoaded = true
		t.add("host", name, pairedString(input, "file_path"))
	default:
		t.add("host", name, evalPathOf(input))
	}
}

func readEvalCodexEvent(event map[string]any, t *EvalTranscript, models map[string]bool) {
	switch pairedString(event, "type") {
	case "thread.started":
		t.SessionID = pairedString(event, "thread_id")
		if model := pairedString(event, "model"); model != "" {
			models[model] = true
		}
	case "turn.started", "turn_context":
		if model := pairedString(event, "model"); model != "" {
			models[model] = true
		}
	case "item.completed":
		item := pairedObject(event, "item")
		switch pairedString(item, "type") {
		case "command_execution":
			t.addShellCall(pairedString(item, "command"))
		case "mcp_tool_call":
			server, tool := pairedString(item, "server"), pairedString(item, "tool")
			arguments := pairedObject(item, "arguments")
			if server == "kapi" {
				t.add("mcp", tool, "")
				return
			}
			// Codex reads a resource through a helper of its own, under the
			// server name `codex`, naming the target server in its arguments.
			if strings.Contains(tool, "mcp_resource") && pairedString(arguments, "server") == "kapi" {
				t.add("mcp", evalResourceTool(pairedString(arguments, "uri")), pairedString(arguments, "uri"))
				return
			}
			t.add("host", server+"."+tool, "")
		case "file_change":
			t.add("host", "file_change", evalCodexChangedPaths(item))
		case "agent_message":
			t.FinalText = pairedString(item, "text")
		}
	case "turn.completed":
		t.Status = "completed"
		usage := pairedObject(event, "usage")
		t.UsageObserved = usage != nil
		t.InputTokens = pairedNumber(usage, "input_tokens")
		t.OutputTokens = pairedNumber(usage, "output_tokens")
	case "error", "turn.failed":
		message := pairedString(event, "message") + " " + pairedString(pairedObject(event, "error"), "message")
		t.Status, t.Error = "agent_failed", strings.TrimSpace(message)
		if pairedRateLimited(message) {
			t.Status, t.RateLimited = "rate_limited", true
		}
	}
}

func evalCodexChangedPaths(item map[string]any) string {
	changes, ok := item["changes"].([]any)
	if !ok {
		return ""
	}
	paths := []string{}
	for _, raw := range changes {
		change, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if path := pairedString(change, "path"); path != "" {
			paths = append(paths, path)
		}
	}
	return strings.Join(paths, " ")
}

// addShellCall reads a shell command for the kapi commands the skill drives.
// One command line can hold several, and each is recorded where it appears.
func (t *EvalTranscript) addShellCall(command string) {
	found := false
	words := evalShellWords(command)
	for index, word := range words {
		if !pairedKapiExecutable(word) {
			continue
		}
		found = true
		t.add("cli", evalKapiRoute(filepath.Base(word), words[index+1:]), strings.TrimSpace(command))
	}
	if !found {
		t.add("host", "shell", strings.TrimSpace(command))
	}
}

// evalShellWords splits a command line into words, dropping the shell
// punctuation that separates commands within one line.
func evalShellWords(command string) []string {
	return strings.FieldsFunc(command, func(r rune) bool {
		return strings.ContainsRune(" \t\n;|&()'\"", r)
	})
}

// evalKapiRoute names the kapi command a word sequence runs: the binary,
// plus the subcommand path up to the first flag or argument that is not one.
func evalKapiRoute(binary string, rest []string) string {
	route := []string{binary}
	for _, word := range rest {
		if strings.HasPrefix(word, "-") {
			break
		}
		if len(route) >= 3 || !evalSubcommandWord(word) {
			break
		}
		route = append(route, word)
	}
	return strings.Join(route, " ")
}

// evalSubcommandWord tells a subcommand from a path or free text. A kapi
// subcommand is a lowercase word, and the arguments these commands take are
// paths or sentences.
func evalSubcommandWord(word string) bool {
	if word == "" || strings.ContainsAny(word, "/.\\=") {
		return false
	}
	for _, r := range word {
		if r != '-' && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func (t *EvalTranscript) add(surface, tool, detail string) {
	t.Calls = append(t.Calls, EvalCall{
		Order: len(t.Calls) + 1, Surface: surface, Tool: tool,
		Kind: evalCallKind(surface, tool, detail), Detail: detail,
	})
}

// evalCallKind places a call among the four habits, or outside them.
// A command line asking for help reads its own documentation, so it counts as
// neither a write nor an ask however the command would otherwise be read.
func evalCallKind(surface, tool, detail string) string {
	if surface == "cli" && strings.Contains(detail, tool+" --help") {
		return evalKindOther
	}
	switch surface {
	case "mcp":
		if kind, ok := evalMCPToolKinds[tool]; ok {
			return kind
		}
		return evalKindOther
	case "cli":
		switch {
		case strings.HasPrefix(tool, "kapi context observe"),
			strings.HasPrefix(tool, "kapi context propose"),
			strings.HasPrefix(tool, "kapi context correct"):
			return evalKindRecord
		case strings.HasPrefix(tool, "kapi context"), strings.HasPrefix(tool, "kapi voice guide"):
			return evalKindAsk
		case strings.HasPrefix(tool, "kapi check"):
			return evalKindCheck
		case strings.HasPrefix(tool, "kapi apply"), strings.HasPrefix(tool, "ksed"):
			return evalKindWrite
		}
		return evalKindOther
	}
	switch tool {
	case "Write", "Edit", "MultiEdit", "NotebookEdit", "file_change":
		return evalKindWrite
	}
	return evalKindOther
}

// evalMCPToolKinds places each kapi MCP tool among the habits. It is the
// transcript reader's half of the product vocabulary: a tool the server gains
// or renames shows up in a preparation record as unclassified until it is
// placed here.
var evalMCPToolKinds = map[string]string{
	"context_observe":         evalKindRecord,
	"context_propose":         evalKindRecord,
	"context_correct":         evalKindRecord,
	"context_withdraw":        evalKindRecord,
	"context_search":          evalKindAsk,
	"context_session_summary": evalKindAsk,
	evalContextResource:       evalKindAsk,
	"check_file":              evalKindCheck,
	"check_text":              evalKindCheck,
	"apply_edits":             evalKindWrite,
	"edit_file":               evalKindWrite,
}

// evalToolCoverage compares a server's tool inventory with the reader: the
// habits the evaluation measures, recording and checking, for which the server
// offers no tool the reader knows, and the tools the server offers that the
// reader cannot place.
func evalToolCoverage(tools []string) (missing, unclassified []string) {
	missing, unclassified = []string{}, []string{}
	offered := map[string]bool{}
	for _, name := range tools {
		kind, ok := evalMCPToolKinds[name]
		if !ok {
			unclassified = append(unclassified, name)
			continue
		}
		offered[kind] = true
	}
	for _, kind := range []string{evalKindRecord, evalKindCheck} {
		if !offered[kind] {
			missing = append(missing, kind)
		}
	}
	slices.Sort(unclassified)
	return missing, unclassified
}

// evalAskedFirst reports whether a context read came before the session's
// first change to a file. Reading a resource is an ask on both surfaces, and a
// session that changed nothing has no first write to be before.
func evalAskedFirst(calls []EvalCall) bool {
	for _, call := range calls {
		switch call.Kind {
		case evalKindAsk:
			return true
		case evalKindWrite:
			return false
		}
	}
	return false
}

// evalIsSkillPath reports a read of the shipped skill by path, which is
// how a host that loads no skill of its own still meets the guidance.
func evalIsSkillPath(path string) bool {
	slashed := filepath.ToSlash(path)
	return strings.Contains(slashed, "/skills/kapi/") && strings.Contains(slashed, "SKILL.md")
}

// evalContextResource is the name a context resource read is recorded
// under, so a resource and a tool call read the same way in a report.
const evalContextResource = "context resource"

func evalResourceTool(uri string) string {
	if strings.HasPrefix(uri, "context://") {
		return evalContextResource
	}
	return "resource read"
}

func evalPathOf(input map[string]any) string {
	for _, key := range []string{"file_path", "path", "pattern", "notebook_path"} {
		if value := pairedString(input, key); value != "" {
			return value
		}
	}
	return ""
}
