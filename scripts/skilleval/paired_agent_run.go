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
	"regexp"
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
	if prepared.Launch.Agent.Host == "codex" && result.SessionID != "" {
		mergePairedCodexRollout(&result, prepared.Launch)
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
	// A host that stopped without a usable stream often says why only on
	// standard error: an expired login, an overloaded API, a lost network. A
	// session that ended with its own result was classified from that result.
	if err != nil && ctx.Err() == nil && slices.Contains([]string{"process_failed", "malformed_stream", "incomplete", "identity_unverified"}, result.Status) {
		stderrFilter.Flush()
		if reason := pairedInfraFailure(pairedStderrTail(prepared.Launch.TranscriptPath + ".stderr")); reason != "" {
			result.Status = "infra_failed"
			result.InfraFailure = reason
			err = fmt.Errorf("%w (%s failure)", err, reason)
		}
	}
	if err != nil {
		result.Error = pairedRedactText(err.Error(), prepared.Env)
		err = errors.New(result.Error)
	}
	result.FinalText = pairedRedactText(result.FinalText, prepared.Env)
	return result, err
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

// pairedInfraPatterns recognise a failure of the service or the machine
// rather than of the agent: a refused or expired login, an overloaded or
// failing API, a dropped network, a full disk.
var pairedInfraPatterns = []struct {
	reason  string
	pattern *regexp.Regexp
}{
	{"auth", regexp.MustCompile(`(?i)oauth token (?:has )?expired|token (?:has |is )?(?:expired|revoked)|invalid (?:api key|bearer token|x-api-key)|authentication[_ ](?:error|failed)|\b401\b|unauthori[sz]ed|please run /login|not logged in|login (?:is )?required|refresh token|failed to refresh|re-?authenticate`)},
	{"disk", pairedDiskFullPattern},
	{"overload", regexp.MustCompile(`(?i)\b529\b|overloaded|at capacity|api error: 5\d\d|\b50[0234]\b (?:internal server error|bad gateway|service unavailable|gateway timeout)|internal server error|service unavailable|bad gateway|gateway timeout|server is busy`)},
	{"network", regexp.MustCompile(`(?i)ECONNRESET|ECONNREFUSED|ETIMEDOUT|ENOTFOUND|EAI_AGAIN|ENETUNREACH|network (?:error|is unreachable)|connection (?:reset|refused|closed|error|timed out)|request timed out|socket hang up|fetch failed|stream (?:disconnected|error)|error sending request|could not resolve host|tls handshake|unable to connect`)},
}

// pairedDiskFullPattern is a machine out of disk space, as a host, a tool or
// the system reports it.
var pairedDiskFullPattern = regexp.MustCompile(`(?i)no space left on device|\bENOSPC\b|disk quota exceeded|not enough space on the disk`)

// pairedDiskFull reports whether a session's transcript or standard error
// says the machine ran out of disk space during it. A full disk fails the
// tools the agent runs, so the attempt says nothing about the agent, however
// it ended.
func pairedDiskFull(paths ...string) bool {
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64<<10), 16<<20)
		found := false
		for scanner.Scan() {
			if pairedDiskFullPattern.Match(scanner.Bytes()) {
				found = true
				break
			}
		}
		_ = file.Close()
		if found {
			return true
		}
	}
	return false
}

// pairedInfraFailure returns the kind of infrastructure failure text
// describes, or "".
func pairedInfraFailure(text string) string {
	for _, candidate := range pairedInfraPatterns {
		if candidate.pattern.MatchString(text) {
			return candidate.reason
		}
	}
	return ""
}

// pairedStderrTail returns the end of a session's recorded standard error,
// where a host prints why it stopped.
func pairedStderrTail(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	const tail = 64 << 10
	if info, err := file.Stat(); err == nil && info.Size() > tail {
		if _, err := file.Seek(info.Size()-tail, io.SeekStart); err != nil {
			return ""
		}
	}
	data, _ := io.ReadAll(io.LimitReader(file, tail))
	return string(data)
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
