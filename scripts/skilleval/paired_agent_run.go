package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func runPairedAgent(ctx context.Context, prepared PairedPrepared) (PairedAgentResult, error) {
	result := PairedAgentResult{RequestedModel: prepared.Launch.Agent.Model, Tools: []string{}, QuotaStatus: "unknown", Status: "blocked"}
	if prepared.Launch.Condition == "mcp" && (prepared.MCPReadiness == nil || prepared.MCPReadiness.Status != "ready") {
		return result, errors.New("kapi MCP server readiness has not been established")
	}
	if len(prepared.Blockers) != 0 {
		return result, fmt.Errorf("agent preflight blocked: %s", strings.Join(prepared.Blockers, "; "))
	}
	if prepared.Launch.Timeout <= 0 {
		return result, errors.New("positive attempt timeout required")
	}
	ctx, cancel := context.WithTimeout(ctx, prepared.Launch.Timeout)
	defer cancel()
	transcript, err := os.OpenFile(prepared.Launch.TranscriptPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return result, err
	}
	defer transcript.Close()
	stderr, err := os.OpenFile(prepared.Launch.TranscriptPath+".stderr", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return result, err
	}
	defer stderr.Close()
	command := exec.CommandContext(ctx, prepared.Executable, prepared.Args...)
	command.Dir = prepared.Launch.Workspace
	command.Env = prepared.Env
	command.Stdin = strings.NewReader(prepared.Launch.Prompt)
	stderrFilter := newPairedRedactor(stderr, prepared.Env)
	defer stderrFilter.Flush()
	command.Stderr = stderrFilter
	command.WaitDelay = 2 * time.Second
	pairedConfigureProcess(command)
	pipe, err := command.StdoutPipe()
	if err != nil {
		return result, err
	}
	defer pipe.Close()
	// Closing stdout on cancellation also releases the scanner when a descendant
	// retains the descriptor after its parent exits. WaitDelay starts too late
	// to provide that guarantee on its own.
	stopPipeClose := context.AfterFunc(ctx, func() { _ = pipe.Close() })
	defer stopPipeClose()
	started := time.Now()
	if err := command.Start(); err != nil {
		result.Status = "launch_failed"
		result.Error = err.Error()
		return result, err
	}
	defer func() { _ = pairedStopProcess(command) }()
	transcriptFilter := newPairedRedactor(transcript, prepared.Env)
	defer transcriptFilter.Flush()
	result, parseErr := parsePairedAgentStream(io.TeeReader(pipe, transcriptFilter), prepared.Launch)
	if parseErr != nil && result.Status != "identity_unverified" {
		_ = pairedStopProcess(command)
		_ = pipe.Close()
		_, _ = io.Copy(transcriptFilter, pipe)
	}
	waitErr := command.Wait()
	if result.Status == "identity_unverified" && prepared.Launch.Agent.Host == "codex" {
		actual, identityErr := pairedCodexRolloutModel(prepared.Launch.StateDir, result.SessionID)
		if identityErr == nil {
			result.ActualModel = actual
			if actual != result.RequestedModel {
				result.Status = "model_mismatch"
				parseErr = errors.New("codex rollout model differs from requested model")
			} else if result.ProtocolCompleted {
				result.Status = "completed"
				parseErr = nil
			}
		}
	}
	result.DurationMS = time.Since(started).Milliseconds()
	switch {
	case ctx.Err() != nil:
		result.Status = "interrupted"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.Status = "timeout"
		}
		err = ctx.Err()
	case parseErr != nil:
		err = parseErr
	case waitErr != nil:
		result.Status = "process_failed"
		err = waitErr
	}
	if err != nil {
		result.Error = pairedRedactText(err.Error(), prepared.Env)
		err = errors.New(result.Error)
	}
	result.FinalText = pairedRedactText(result.FinalText, prepared.Env)
	return result, err
}

// Only host protocol fields establish identity and completion. Text in a model's
// answer cannot self-certify a requested model or successful process outcome.
func parsePairedAgentStream(reader io.Reader, launch PairedLaunch) (PairedAgentResult, error) {
	result := PairedAgentResult{RequestedModel: launch.Agent.Model, Status: "incomplete", Tools: []string{}, QuotaStatus: "unknown"}
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
						if !ok {
							continue
						}
						if pairedString(part, "type") == "tool_use" {
							tool := pairedString(part, "name")
							result.Tools = pairedUnique(result.Tools, tool)
							if violation := pairedRouteViolation(launch.Condition, tool, pairedObject(part, "input")); violation != "" {
								result.Status = "route_violation"
								return result, errors.New(violation)
							}
						}
					}
				}
			}
			if eventType == "result" {
				complete = true
				result.ProtocolCompleted = true
				result.FinalText = pairedString(event, "result")
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
				if kind == "command_execution" {
					result.Tools = pairedUnique(result.Tools, "shell")
					if violation := pairedRouteViolation(launch.Condition, "Bash", map[string]any{"command": pairedString(item, "command")}); violation != "" {
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
				}
				if kind == "agent_message" {
					result.FinalText = pairedString(item, "text")
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

func pairedCodexMCPRouteViolation(condition string, item map[string]any) string {
	server, tool := pairedString(item, "server"), pairedString(item, "tool")
	input := pairedObject(item, "arguments")
	// Codex reports its own resource-discovery helpers under server="codex"
	// when no target server was supplied. Only kapi is configured in this arm.
	if condition == "mcp" && server == "codex" {
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
	return pairedRouteViolation(condition, "mcp__"+server+"__"+tool, input)
}

func pairedRouteViolation(condition, tool string, input map[string]any) string {
	if strings.HasPrefix(tool, "mcp__") {
		if condition != "mcp" || !strings.HasPrefix(tool, "mcp__kapi__") {
			return "unexpected MCP tool route: " + tool
		}
	}
	if condition != "skill-cli" && tool == "Skill" {
		return "unexpected skill route"
	}
	if condition != "skill-cli" && (tool == "Bash" || tool == "shell") {
		command := pairedString(input, "command")
		// This is an audit tripwire, not a containment boundary. Matching command
		// text alone cannot prevent aliases or indirect execution.
		words := strings.FieldsFunc(command, func(r rune) bool { return strings.ContainsRune(" \t\n;|&()'\"", r) })
		if slices.ContainsFunc(words, pairedKapiExecutable) {
			return "unexpected kapi CLI route"
		}
	}
	return ""
}
func pairedString(m map[string]any, key string) string { value, _ := m[key].(string); return value }
func pairedObject(m map[string]any, key string) map[string]any {
	value, _ := m[key].(map[string]any)
	return value
}
func pairedNumber(m map[string]any, key string) int64 {
	value, _ := m[key].(float64)
	return int64(value)
}
func pairedUnique(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func pairedRateLimited(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "rate limit") || strings.Contains(lower, "usage limit") || strings.Contains(lower, "quota")
}

func pairedCodexRolloutModel(stateDir, sessionID string) (string, error) {
	if sessionID == "" || strings.ContainsAny(sessionID, "/\\*") {
		return "", errors.New("invalid Codex session identity")
	}
	matches, err := filepath.Glob(filepath.Join(stateDir, "codex", "sessions", "*", "*", "*", "*"+sessionID+".jsonl"))
	if err != nil {
		return "", err
	}
	models := map[string]bool{}
	for _, path := range matches {
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
		for scanner.Scan() {
			event := map[string]any{}
			if json.Unmarshal(scanner.Bytes(), &event) != nil {
				continue
			}
			if pairedString(event, "type") == "turn_context" {
				model := pairedString(pairedObject(event, "payload"), "model")
				if model != "" {
					models[model] = true
				}
			}
		}
		scanErr := scanner.Err()
		_ = file.Close()
		if scanErr != nil {
			return "", scanErr
		}
	}
	if len(models) != 1 {
		return "", errors.New("codex rollout lacks a unique model identity")
	}
	for model := range models {
		return model, nil
	}
	return "", errors.New("missing model")
}

type pairedRedactor struct {
	destination io.Writer
	pending     []byte
	secrets     []string
}

func newPairedRedactor(destination io.Writer, env []string) *pairedRedactor {
	secrets := []string{}
	for _, pair := range env {
		key, value, ok := strings.Cut(pair, "=")
		if ok && strings.Contains(key, "TOKEN") && value != "" {
			secrets = append(secrets, value)
		}
	}
	return &pairedRedactor{destination: destination, pending: []byte{}, secrets: secrets}
}
func (r *pairedRedactor) Write(data []byte) (int, error) {
	r.pending = append(r.pending, data...)
	for {
		index := strings.IndexByte(string(r.pending), '\n')
		if index < 0 {
			break
		}
		line := string(r.pending[:index+1])
		for _, secret := range r.secrets {
			line = strings.ReplaceAll(line, secret, "[REDACTED]")
		}
		if _, err := io.WriteString(r.destination, line); err != nil {
			return 0, err
		}
		r.pending = r.pending[index+1:]
	}
	if len(r.pending) > 8*1024*1024 {
		return 0, errors.New("agent output line exceeds recording limit")
	}
	return len(data), nil
}
func (r *pairedRedactor) Flush() error {
	line := string(r.pending)
	for _, secret := range r.secrets {
		line = strings.ReplaceAll(line, secret, "[REDACTED]")
	}
	r.pending = []byte{}
	_, err := io.WriteString(r.destination, line)
	return err
}

func pairedRedactText(value string, env []string) string {
	for _, pair := range env {
		key, secret, ok := strings.Cut(pair, "=")
		if ok && strings.Contains(key, "TOKEN") && secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}

func pairedKapiExecutable(word string) bool {
	switch filepath.Base(word) {
	case "kapi", "kcat", "kgrep", "ksed", "kdiff":
		return true
	}
	return false
}
