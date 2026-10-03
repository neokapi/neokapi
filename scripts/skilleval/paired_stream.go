package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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
					} else if reason := pairedInfraFailure(result.FinalText + " " + string(errorsJSON)); reason != "" {
						result.Status = "infra_failed"
						result.InfraFailure = reason
						return result, fmt.Errorf("agent reported an unsuccessful result: %s failure", reason)
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
					observer.auditPaths(tool, pairedObject(item, "arguments"))
					observer.noteRoute(tool, pairedObject(item, "arguments"))
				}
				if kind == "file_change" {
					result.Tools = pairedUnique(result.Tools, "file_change")
					observer.scanInput("file_change", item)
					observer.auditPaths("file_change", item)
					observer.noteRoute("file_change", item)
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
				} else if reason := pairedInfraFailure(message); reason != "" {
					result.Status = "infra_failed"
					result.InfraFailure = reason
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
