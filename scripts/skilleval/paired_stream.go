package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Only host protocol fields establish identity and completion. Text in a model's
// answer cannot self-certify a requested model or successful process outcome.
func parsePairedAgentStream(reader io.Reader, launch PairedLaunch) (PairedAgentResult, error) {
	result := PairedAgentResult{RequestedModel: launch.Agent.Model, Status: "incomplete", Tools: []string{}, QuotaStatus: "unknown"}
	observer := newPairedObserver(launch, &result)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	complete := false
	models := map[string]bool{}
	for scanner.Scan() {
		if len(strings.TrimSpace(scanner.Text())) == 0 {
			continue
		}
		event := map[string]any{}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			result.Status = "malformed_stream"
			return result, fmt.Errorf("invalid agent event: %w", err)
		}
		eventType := pairedString(event, "type")
		if eventType == "" {
			result.Status = "malformed_stream"
			return result, errors.New("agent event has no type")
		}
		switch launch.Agent.Host {
		case "claude":
			if eventType == "system" && pairedString(event, "subtype") == "init" {
				result.SessionID = pairedString(event, "session_id")
				if model := pairedString(event, "model"); model != "" {
					models[model] = true
				}
			}
			if eventType == "assistant" {
				msg := pairedObject(event, "message")
				if model := pairedString(msg, "model"); model != "" && model != "<synthetic>" {
					models[model] = true
				}
				if content, ok := msg["content"].([]any); ok {
					for _, raw := range content {
						part, ok := raw.(map[string]any)
						if !ok || pairedString(part, "type") != "tool_use" {
							continue
						}
						tool := pairedString(part, "name")
						result.Tools = pairedUnique(result.Tools, tool)
						if launch.NoTools {
							result.Status = "tool_use_violation"
							return result, errors.New("tool use violates fixed-input review protocol")
						}
						if violation := observer.toolUse(pairedString(part, "id"), tool, pairedObject(part, "input")); violation != "" {
							result.Status = "route_violation"
							return result, errors.New(violation)
						}
					}
				}
			}
			if eventType == "user" {
				msg := pairedObject(event, "message")
				if content, ok := msg["content"].([]any); ok {
					for _, raw := range content {
						part, ok := raw.(map[string]any)
						if ok && pairedString(part, "type") == "tool_result" {
							observer.toolResult(pairedString(part, "tool_use_id"), pairedStrings(part["content"]))
						}
					}
				}
			}
			if eventType == "result" {
				complete = true
				result.ProtocolCompleted = true
				result.FinalText = pairedString(event, "result")
				if turns, ok := event["num_turns"].(float64); ok {
					n := int64(turns)
					result.Turns = &n
				}
				usage := pairedObject(event, "usage")
				result.UsageObserved = usage != nil
				result.InputTokens = pairedNumber(usage, "input_tokens")
				result.CacheReadTokens = pairedNumber(usage, "cache_read_input_tokens")
				result.CacheWriteTokens = pairedNumber(usage, "cache_creation_input_tokens")
				result.InputTokens += result.CacheReadTokens + result.CacheWriteTokens
				result.OutputTokens = pairedNumber(usage, "output_tokens")
				if failed, _ := event["is_error"].(bool); failed || pairedString(event, "subtype") != "success" {
					result.Status = "agent_failed"
					errorsJSON, _ := json.Marshal(event["errors"])
					if result.RateLimited || pairedRateLimited(result.FinalText+" "+string(errorsJSON)) {
						result.RateLimited = true
						result.Status = "rate_limited"
						result.QuotaStatus = "exhausted_or_throttled"
					}
					return result, errors.New("agent reported an unsuccessful result")
				}
			}
		case "codex":
			switch eventType {
			case "thread.started":
				result.SessionID = pairedString(event, "thread_id")
				if model := pairedString(event, "model"); model != "" {
					models[model] = true
				}
			case "turn.started", "turn_context":
				if model := pairedString(event, "model"); model != "" {
					models[model] = true
				}
			case "item.started", "item.completed":
				item := pairedObject(event, "item")
				kind := pairedString(item, "type")
				isTool := kind == "command_execution" || kind == "mcp_tool_call" || kind == "file_change" ||
					kind == "web_search" || kind == "collab_tool_call"
				if launch.NoTools && isTool {
					result.Tools = pairedUnique(result.Tools, kind)
					result.Status = "tool_use_violation"
					return result, errors.New("tool use violates fixed-input review protocol")
				}
				if kind == "command_execution" {
					result.Tools = pairedUnique(result.Tools, "shell")
					if violation := observer.toolUse("", "shell", map[string]any{"command": pairedString(item, "command")}); violation != "" {
						result.Status = "route_violation"
						return result, errors.New(violation)
					}
				}
				if kind == "mcp_tool_call" {
					tool := "mcp__" + pairedString(item, "server") + "__" + pairedString(item, "tool")
					result.Tools = pairedUnique(result.Tools, tool)
					if violation := pairedCodexMCPRouteViolation(launch.Condition, item); violation != "" {
						result.Status = "route_violation"
						return result, errors.New(violation)
					}
					observer.scanInput(tool, pairedObject(item, "arguments"))
				}
				if kind == "file_change" {
					result.Tools = pairedUnique(result.Tools, "file_change")
					observer.scanInput("file_change", item)
				}
				if kind == "agent_message" {
					result.FinalText = pairedString(item, "text")
				}
				if eventType == "item.completed" && isTool {
					observer.codexCompleted(kind, item)
				}
			case "turn.completed":
				complete = true
				result.ProtocolCompleted = true
				usage := pairedObject(event, "usage")
				result.UsageObserved = usage != nil
				result.InputTokens = pairedNumber(usage, "input_tokens")
				// Codex includes cache reads in its input total; Claude reports them
				// separately. Preserve cache counts without adding them twice.
				result.CacheReadTokens = pairedNumber(usage, "cached_input_tokens")
				result.OutputTokens = pairedNumber(usage, "output_tokens")
			case "error", "turn.failed":
				message := pairedString(event, "message") + " " + pairedString(pairedObject(event, "error"), "message")
				result.RateLimited = pairedRateLimited(message)
				result.Status = "agent_failed"
				if result.RateLimited {
					result.Status = "rate_limited"
					result.QuotaStatus = "exhausted_or_throttled"
				}
				return result, fmt.Errorf("agent error: %s", strings.TrimSpace(message))
			}
		default:
			return result, errors.New("unknown agent host")
		}
		if eventType == "rate_limit_event" {
			info := pairedObject(event, "rate_limit_info")
			status := pairedString(info, "status")
			result.QuotaStatus = status
			if status == "rejected" {
				result.RateLimited = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		result.Status = "malformed_stream"
		return result, err
	}
	if len(models) == 1 {
		for model := range models {
			result.ActualModel = model
		}
	}
	if len(models) > 1 {
		result.Status = "model_mismatch"
		return result, errors.New("agent used multiple model identities")
	}
	if result.ActualModel == "" {
		result.Status = "identity_unverified"
		return result, errors.New("agent stream did not report model identity")
	}
	if result.ActualModel != launch.Agent.Model {
		result.Status = "model_mismatch"
		return result, fmt.Errorf("requested model %q; observed %q", launch.Agent.Model, result.ActualModel)
	}
	if !complete {
		return result, errors.New("agent stream ended without completion")
	}
	if result.RateLimited {
		result.Status = "rate_limited"
		return result, errors.New("subscription rate limit reached")
	}
	result.Status = "completed"
	return result, nil
}

// pairedObserver reads each tool call and tool result as the stream arrives:
// it audits the route, counts calls and refusals, records override attempts,
// and lands a task's interference after the agent's first read.
type pairedObserver struct {
	launch       PairedLaunch
	arm          pairedArm
	result       *PairedAgentResult
	started      time.Time
	pendingReads map[string]string
	interference *pairedInterferer
}

func newPairedObserver(launch PairedLaunch, result *PairedAgentResult) *pairedObserver {
	arm, _ := pairedArmFor(launch.Condition)
	o := &pairedObserver{launch: launch, arm: arm, result: result, started: time.Now(), pendingReads: map[string]string{}}
	if launch.Interference != nil && launch.Workspace != "" {
		o.interference = newPairedInterferer(launch.Workspace, *launch.Interference)
		result.Interference = &PairedInterferenceRecord{}
	}
	return o
}

// toolUse audits one proposed tool call. It returns a violation for a route
// the condition forbids, which ends the attempt.
func (o *pairedObserver) toolUse(id, tool string, input map[string]any) string {
	if id != "" {
		o.result.ToolCalls++
	}
	if violation := pairedRouteViolation(o.launch, o.arm, tool, input); violation != "" {
		return violation
	}
	if tool == "Bash" || tool == "shell" {
		for _, attempt := range pairedRouteAttempts(o.arm, pairedString(input, "command")) {
			o.result.RouteAttempts = pairedUnique(o.result.RouteAttempts, attempt)
		}
	}
	o.scanInput(tool, input)
	if id != "" && o.interference != nil && o.interference.mentioned(input) {
		o.pendingReads[id] = tool
	}
	return ""
}

// scanInput records override attempts in any value a tool call carries,
// including the content of a file the agent writes.
func (o *pairedObserver) scanInput(tool string, input any) {
	for _, text := range pairedStrings(input) {
		for _, label := range pairedOverrides(text) {
			o.result.OverrideAttempts = pairedUnique(o.result.OverrideAttempts, label+" ("+tool+")")
		}
	}
}

// toolResult reads a Claude tool result.
func (o *pairedObserver) toolResult(id string, texts []string) {
	o.countRefusals(texts)
	if tool, ok := o.pendingReads[id]; ok {
		delete(o.pendingReads, id)
		o.interfere(tool)
	}
}

// codexCompleted reads a finished Codex tool item.
func (o *pairedObserver) codexCompleted(kind string, item map[string]any) {
	o.result.ToolCalls++
	texts := pairedStrings(item)
	o.countRefusals(texts)
	if kind == "file_change" && pairedString(item, "status") == "failed" {
		o.addRefusal("host:patch_failed")
	}
	if o.interference != nil && o.interference.mentioned(item) {
		o.interfere(kind)
	}
}

var (
	pairedKapiRefusal = regexp.MustCompile(`(?:refused: |"code":\s*")(stale|gate_failed|guard|not_found|ambiguous|unsupported|not_permitted|invalid)\b`)
	pairedHostStale   = regexp.MustCompile(`(?i)has been modified since (?:it was )?read|modified since read`)
	pairedHostPatch   = regexp.MustCompile(`(?i)failed to find expected lines|patch (?:did not apply|failed)`)
)

func (o *pairedObserver) countRefusals(texts []string) {
	codes := map[string]bool{}
	for _, text := range texts {
		for _, match := range pairedKapiRefusal.FindAllStringSubmatch(text, -1) {
			codes[match[1]] = true
		}
		if pairedHostStale.MatchString(text) {
			codes["host:stale"] = true
		}
		if pairedHostPatch.MatchString(text) {
			codes["host:patch_failed"] = true
		}
	}
	for code := range codes {
		o.addRefusal(code)
	}
}

func (o *pairedObserver) addRefusal(code string) {
	if o.result.Refusals == nil {
		o.result.Refusals = map[string]int{}
	}
	o.result.Refusals[code]++
}

func (o *pairedObserver) interfere(trigger string) {
	if o.interference == nil || o.result.Interference.Triggered {
		return
	}
	record := o.interference.apply()
	record.Trigger = trigger
	record.AfterMS = time.Since(o.started).Milliseconds()
	o.result.Interference = &record
}

var (
	pairedGateReport  = regexp.MustCompile(`--gate[= ]+["']?report|"gate"\s*:\s*"report"`)
	pairedActorPerson = regexp.MustCompile(`KAPI_ACTOR\s*=\s*["']?person`)
	pairedBlindWrite  = regexp.MustCompile(`"if_match"\s*:\s*"\*"`)
)

// pairedOverrides names each way text tries to land an edit over a check.
func pairedOverrides(text string) []string {
	var out []string
	if pairedGateReport.MatchString(text) {
		out = append(out, "gate report")
	}
	if pairedActorPerson.MatchString(text) {
		out = append(out, "actor person")
	}
	if pairedBlindWrite.MatchString(text) {
		out = append(out, "blind write")
	}
	return out
}

// pairedStrings collects every string a decoded JSON value holds.
func pairedStrings(value any) []string {
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch typed := v.(type) {
		case string:
			out = append(out, typed)
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case map[string]any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	return out
}

func pairedCodexMCPRouteViolation(condition string, item map[string]any) string {
	server, tool := pairedString(item, "server"), pairedString(item, "tool")
	input := pairedObject(item, "arguments")
	arm, _ := pairedArmFor(condition)
	// Codex reports its own resource-discovery helpers under server="codex"
	// when no target server was supplied. Only kapi is configured in this arm.
	if arm.MCP && server == "codex" {
		target := pairedString(input, "server")
		switch tool {
		case "list_mcp_resources", "list_mcp_resource_templates":
			if target == "" || target == "kapi" {
				return ""
			}
		case "read_mcp_resource":
			if target == "kapi" {
				return ""
			}
		}
	}
	if !arm.MCP || server != "kapi" {
		return "unexpected MCP tool route: mcp__" + server + "__" + tool
	}
	return ""
}

// pairedRouteViolation is the audit's tripwire against a surface the condition
// does not hold: an MCP tool outside the MCP arm, a skill other than the arm's,
// or a kapi binary named by a path outside the cell, which would run a build
// other than the one under test. It cannot prevent aliases or indirect
// execution; it detects the accidental mix.
func pairedRouteViolation(launch PairedLaunch, arm pairedArm, tool string, input map[string]any) string {
	if strings.HasPrefix(tool, "mcp__") {
		if !arm.MCP || !strings.HasPrefix(tool, "mcp__kapi__") {
			return "unexpected MCP tool route: " + tool
		}
	}
	if tool == "Skill" {
		name := pairedString(input, "skill")
		if name == "" {
			name = pairedString(input, "command")
		}
		if arm.Skill == "" || (name != "" && strings.TrimPrefix(name, "/") != arm.Skill) {
			return "unexpected skill route: " + name
		}
	}
	if tool == "Bash" || tool == "shell" {
		bin := filepath.Join(launch.StateDir, "bin")
		for _, word := range pairedCommandWords(pairedString(input, "command")) {
			if !pairedKapiExecutable(word) || !strings.Contains(word, "/") {
				continue
			}
			if launch.StateDir != "" && filepath.Dir(word) == bin && slices.Contains(arm.Executables, path.Base(word)) {
				continue
			}
			return "unexpected kapi CLI route: " + word
		}
	}
	return ""
}

// pairedRouteAttempts lists the bare kapi names a command uses that the cell's
// PATH does not hold. A toolbox name right after one of the arm's own names is
// that command's subcommand, not an attempt.
func pairedRouteAttempts(arm pairedArm, command string) []string {
	var out []string
	words := pairedCommandWords(command)
	for i, word := range words {
		if strings.Contains(word, "/") || !pairedKapiExecutable(word) || slices.Contains(arm.Executables, word) {
			continue
		}
		if i > 0 && slices.Contains(pairedToolboxNames, word) && slices.Contains(arm.Executables, words[i-1]) {
			continue
		}
		out = pairedUnique(out, word)
	}
	return out
}

func pairedCommandWords(command string) []string {
	return strings.FieldsFunc(command, func(r rune) bool { return strings.ContainsRune(" \t\n;|&()'\"`$<>", r) })
}

// pairedInterferer is the other editor of a stale-recovery task.
type pairedInterferer struct {
	workspace string
	spec      PairedInterference
	original  []byte
	readErr   error
}

func newPairedInterferer(workspace string, spec PairedInterference) *pairedInterferer {
	original, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(spec.Path)))
	return &pairedInterferer{workspace: workspace, spec: spec, original: original, readErr: err}
}

// mentioned reports whether a tool call or its result names the file.
func (i *pairedInterferer) mentioned(value any) bool {
	base := path.Base(i.spec.Path)
	for _, text := range pairedStrings(value) {
		if strings.Contains(text, base) {
			return true
		}
	}
	return false
}

// apply makes the other editor's change to the file as it stands now, keeping
// whatever the agent already wrote there.
func (i *pairedInterferer) apply() PairedInterferenceRecord {
	record := PairedInterferenceRecord{Triggered: true}
	if i.readErr != nil {
		record.Error = "read the original: " + i.readErr.Error()
		return record
	}
	file := filepath.Join(i.workspace, filepath.FromSlash(i.spec.Path))
	current, err := os.ReadFile(file)
	if err != nil {
		record.Error = err.Error()
		return record
	}
	record.AgentWroteFirst = !bytes.Equal(current, i.original)
	if !bytes.Contains(current, []byte(i.spec.Find)) {
		record.Error = "the text the other editor changes is no longer in the file"
		return record
	}
	changed := bytes.Replace(current, []byte(i.spec.Find), []byte(i.spec.Replace), 1)
	if err := os.WriteFile(file, changed, 0o600); err != nil {
		record.Error = err.Error()
		return record
	}
	record.Applied = true
	return record
}
