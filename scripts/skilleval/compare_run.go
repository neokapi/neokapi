package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// CompareOptions controls one invocation.
type CompareOptions struct {
	ManifestPath string
	Phase        string
	Dir          string
	CellsDir     string
	RepoRoot     string
	KapiBin      string
	Live         bool
	Retry        bool
	Concurrency  int
	MaxAttempts  int
	Attempts     string
	Out          string
}

// CompareUsage is what a session's own stream says it consumed. Token counts
// are usage observations; the cost is Claude's own API-equivalent figure and
// is absent for Codex, whose stream reports none.
type CompareUsage struct {
	InputTokens      int64   `json:"input_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	Turns            int64   `json:"turns,omitempty"`
	CostUSD          float64 `json:"cost_usd,omitempty"`
}

// totalInput is every input token, cached or not.
func (u CompareUsage) totalInput() int64 {
	return u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens
}

// CompareResult is what one attempt recorded.
type CompareResult struct {
	Attempt    CompareAttempt `json:"attempt"`
	Status     string         `json:"status"`
	Error      string         `json:"error,omitempty"`
	Started    string         `json:"started"`
	DurationMS int64          `json:"duration_ms"`
	Usage      CompareUsage   `json:"usage"`
	ToolCalls  int            `json:"tool_calls"`
	// KapiCalls lists the kapi tools and commands the agent reached, in order.
	KapiCalls   []string `json:"kapi_calls"`
	Asked       bool     `json:"asked"`
	AskedFirst  bool     `json:"asked_before_writing"`
	Checked     bool     `json:"checked"`
	Recorded    int      `json:"recorded"`
	SkillLoaded bool     `json:"skill_loaded"`
	// ReadRules reports a read of CLAUDE.md or AGENTS.md by a tool, which a
	// host that loads the file itself does not need.
	ReadRules   bool   `json:"read_rules_file"`
	ActualModel string `json:"actual_model,omitempty"`
	FinalText   string `json:"final_text,omitempty"`
	// Changed lists the content files the session changed, added or removed.
	Changed []string `json:"changed"`
}

func (r CompareResult) completed() bool { return r.Status == "completed" }

// compareRetryable are the statuses that say nothing about the agent: a rate
// limit, an outage, an interruption. -compare-retry runs them again.
var compareRetryable = []string{"rate_limited", "infra_failed", "interrupted", "launch_failed", "blocked", "prepare_failed"}

func executeCompare(ctx context.Context, opts CompareOptions) error {
	m, err := readCompareManifest(opts.ManifestPath)
	if err != nil {
		return err
	}
	if opts.CellsDir == "" {
		opts.CellsDir = filepath.Join(os.TempDir(), "kapi-compare-cells")
	}
	switch opts.Phase {
	case comparePhasePreflight:
		return preflightCompare(ctx, opts, m)
	case comparePhasePilot, comparePhaseRun:
		if !opts.Live {
			return errors.New("live phases start billed agent sessions; pass -compare-live to allow them")
		}
		return runCompare(ctx, opts, m, opts.Phase)
	case comparePhaseGrade:
		return gradeCompare(ctx, opts, m)
	case comparePhaseJudge:
		if !opts.Live {
			return errors.New("judging starts billed model sessions; pass -compare-live to allow them")
		}
		return judgeCompare(ctx, opts, m)
	case comparePhaseReport:
		return reportCompare(opts, m)
	}
	return fmt.Errorf("unknown comparison phase %q", opts.Phase)
}

// comparePhaseDirs lists the evidence directories a report reads: the pilot,
// the full run, or both.
func comparePhaseDirs(opts CompareOptions) []string {
	return []string{filepath.Join(opts.Dir, comparePhasePilot), filepath.Join(opts.Dir, comparePhaseRun)}
}

// preflightCompare prepares one cell of every arm for every project and host,
// runs the gate probe in a kapi cell of each project, and prints the blockers.
// It makes no model call.
func preflightCompare(ctx context.Context, opts CompareOptions, m CompareManifest) error {
	projects := map[string]bool{}
	tasks := append(slices.Clone(m.Tasks), m.PilotTasks...)
	for _, id := range tasks {
		task, err := findCompareTask(id)
		if err != nil {
			return err
		}
		if compareKapiMention.MatchString(task.Prompt) {
			return fmt.Errorf("task %s: the prompt names the mechanism under test", task.ID)
		}
		projects[task.Project] = true
	}
	names := []string{}
	for name := range projects {
		names = append(names, name)
	}
	slices.Sort(names)
	blocked := 0
	for _, name := range names {
		project, err := loadCompareProject(name)
		if err != nil {
			return err
		}
		paths, wiring, err := prepareCompareCell(ctx, filepath.Join(opts.CellsDir, "preflight", "grader-"+name), project, compareArmKapi, opts.KapiBin)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		probes, err := compareGateProbe(ctx, paths, project, wiring.Baseline)
		if err != nil {
			return fmt.Errorf("%s gate probe: %w", name, err)
		}
		for _, probe := range probes {
			fmt.Printf("%s: gate probe in %s failed %d findings: %s\n", name, probe.File, probe.Failing, strings.Join(probe.Rules, "; "))
		}
		for _, arm := range m.Arms {
			for _, host := range m.Hosts {
				attempt := CompareAttempt{ID: "preflight-" + name + "-" + arm + "-" + host.Host, Phase: "preflight", Project: name, Arm: arm, Host: host}
				for _, id := range tasks {
					if task, _ := findCompareTask(id); task.Project == name {
						attempt.Task = id
						break
					}
				}
				p, err := prepareCompareAttempt(ctx, opts.CellsDir, opts.KapiBin, m, attempt)
				if err != nil {
					return fmt.Errorf("%s: %w", attempt.ID, err)
				}
				state := "ready"
				if len(p.Blockers) != 0 {
					state = "BLOCKED: " + strings.Join(p.Blockers, "; ")
					blocked++
				}
				tools := ""
				if p.Server != nil {
					tools = fmt.Sprintf(" (MCP tools: %s)", strings.Join(p.Server.Tools, ", "))
				}
				fmt.Printf("%s: %s, %s%s\n", attempt.ID, state, strings.Join(p.Wiring.Files, " "), tools)
			}
		}
	}
	if blocked != 0 {
		return fmt.Errorf("%d cells are blocked", blocked)
	}
	return nil
}

// runCompare runs a live phase's attempts, at most Concurrency at once and
// spread evenly over the hosts, and skips every attempt already recorded.
func runCompare(ctx context.Context, opts CompareOptions, m CompareManifest, phase string) error {
	schedule, err := compareSchedule(m, phase)
	if err != nil {
		return err
	}
	if opts.Attempts != "" {
		wanted := map[string]bool{}
		for id := range strings.SplitSeq(opts.Attempts, ",") {
			wanted[strings.TrimSpace(id)] = true
		}
		schedule = slices.DeleteFunc(schedule, func(a CompareAttempt) bool { return !wanted[a.ID] })
	}
	dir := filepath.Join(opts.Dir, phase)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := writeCompareJSON(filepath.Join(dir, "manifest.json"), m); err != nil {
		return err
	}
	todo := []CompareAttempt{}
	for _, attempt := range schedule {
		var previous CompareResult
		if readPairedJSON(filepath.Join(dir, attempt.ID, "result.json"), &previous) == nil {
			if previous.completed() || !opts.Retry || !slices.Contains(compareRetryable, previous.Status) {
				continue
			}
		}
		todo = append(todo, attempt)
	}
	if opts.MaxAttempts > 0 && len(todo) > opts.MaxAttempts {
		todo = todo[:opts.MaxAttempts]
	}
	fmt.Fprintf(os.Stderr, "%s: %d of %d attempts to run, %d at once\n", phase, len(todo), len(schedule), opts.Concurrency)
	started := time.Now()
	// One queue per host, each drained by its share of the workers, so a host
	// whose sessions run long never holds up the other's.
	queues := map[string]chan CompareAttempt{}
	for _, host := range m.Hosts {
		queues[host.Host] = make(chan CompareAttempt, len(todo))
	}
	for _, attempt := range todo {
		queues[attempt.Host.Host] <- attempt
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for _, host := range m.Hosts {
		close(queues[host.Host])
		for range max(1, opts.Concurrency/len(m.Hosts)) {
			wg.Add(1)
			go func(queue chan CompareAttempt) {
				defer wg.Done()
				for attempt := range queue {
					if ctx.Err() != nil {
						return
					}
					result := runCompareAttempt(ctx, opts, m, attempt, filepath.Join(dir, attempt.ID))
					mu.Lock()
					done++
					fmt.Fprintf(os.Stderr, "[%d/%d %s] %s: %s in %s, %d in / %d out tokens\n", done, len(todo),
						time.Since(started).Round(time.Second), attempt.ID, result.Status,
						(time.Duration(result.DurationMS) * time.Millisecond).Round(time.Second),
						result.Usage.totalInput(), result.Usage.OutputTokens)
					mu.Unlock()
				}
			}(queues[host.Host])
		}
	}
	wg.Wait()
	fmt.Fprintf(os.Stderr, "%s: finished in %s\n", phase, time.Since(started).Round(time.Second))
	return ctx.Err()
}

// runCompareAttempt prepares, runs and records one attempt. Its errors are
// recorded in the result rather than returned, so one failure leaves the
// batch running.
func runCompareAttempt(ctx context.Context, opts CompareOptions, m CompareManifest, attempt CompareAttempt, dir string) CompareResult {
	result := CompareResult{Attempt: attempt, Status: "prepare_failed", Started: pairedTimestamp(), KapiCalls: []string{}, Changed: []string{}}
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		result.Error = err.Error()
		return result
	}
	save := func() { _ = os.WriteFile(filepath.Join(dir, "result.json"), compareMustJSON(result), 0o600) }
	defer save()
	prepared, err := prepareCompareAttempt(ctx, opts.CellsDir, opts.KapiBin, m, attempt)
	_ = os.WriteFile(filepath.Join(dir, "prepared.json"), compareMustJSON(prepared), 0o600)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if len(prepared.Blockers) != 0 {
		result.Status, result.Error = "blocked", strings.Join(prepared.Blockers, "; ")
		return result
	}
	transcriptPath := filepath.Join(dir, "transcript.jsonl")
	ep := EvalPrepared{
		Session:    EvalSession{ID: attempt.ID, Host: attempt.Host, Task: attempt.Task},
		Paths:      prepared.Paths,
		Executable: prepared.Executable,
		Args:       prepared.Args,
		Env:        append(slices.Clone(prepared.Env), "KAPI_EVAL_TRANSCRIPT="+transcriptPath),
		Prompt:     prepared.Prompt,
		Timeout:    prepared.Timeout,
		Blockers:   []string{},
	}
	runCtx, cancel := context.WithTimeout(ctx, prepared.Timeout)
	defer cancel()
	begun := time.Now()
	transcript, runErr := runEvalAgent(runCtx, ep)
	result.DurationMS = time.Since(begun).Milliseconds()
	result.Status = transcript.Status
	switch {
	case ctx.Err() != nil:
		result.Status = "interrupted"
	case runCtx.Err() != nil:
		result.Status = "timeout"
	}
	if runErr != nil {
		result.Error = runErr.Error()
		if result.Status == "completed" || result.Status == "incomplete" {
			if reason := pairedInfraFailure(pairedStderrTail(transcriptPath + ".stderr")); reason != "" {
				result.Status = "infra_failed"
				result.Error += " (" + reason + ")"
			}
		}
	}
	if transcript.RateLimited {
		result.Status = "rate_limited"
	}
	result.ActualModel = transcript.ActualModel
	result.FinalText = transcript.FinalText
	result.ToolCalls = len(transcript.Calls)
	for _, call := range transcript.kapiCalls() {
		result.KapiCalls = append(result.KapiCalls, call.Tool)
	}
	result.Asked = len(transcript.ofKind(evalKindAsk)) != 0
	result.AskedFirst = transcript.AskedBeforeWriting
	result.Checked = len(transcript.ofKind(evalKindCheck)) != 0
	result.Recorded = len(transcript.ofKind(evalKindRecord))
	result.SkillLoaded = transcript.SkillLoaded
	for _, call := range transcript.Calls {
		base := filepath.Base(call.Detail)
		if base == "CLAUDE.md" || base == "AGENTS.md" || strings.Contains(call.Detail, "AGENTS.md") || strings.Contains(call.Detail, "CLAUDE.md") {
			result.ReadRules = true
		}
	}
	if usage, err := compareReadUsage(transcriptPath, attempt.Host.Host); err == nil {
		result.Usage = usage
	}
	changed, err := compareCaptureFinal(prepared.Paths.Repo, filepath.Join(dir, "final"), attempt.Project)
	if err != nil && result.Error == "" {
		result.Error = "capture: " + err.Error()
	}
	result.Changed = changed
	return result
}

// compareCaptureFinal copies the attempt's content files out of its cell and
// lists the ones that differ from the project as it started.
func compareCaptureFinal(repo, destination, projectName string) ([]string, error) {
	project, err := loadCompareProject(projectName)
	if err != nil {
		return nil, err
	}
	changed := []string{}
	seen := map[string]bool{}
	err = filepath.WalkDir(repo, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(repo, path)
		if err != nil || rel == "." {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !compareContentPath(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		seen[rel] = true
		target := filepath.Join(destination, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			return err
		}
		if original, ok := project.Files[rel]; !ok || string(original) != string(data) {
			changed = append(changed, rel)
		}
		return nil
	})
	for name := range project.Files {
		if !seen[name] {
			changed = append(changed, name+" (removed)")
		}
	}
	slices.Sort(changed)
	return changed, err
}

// compareReadUsage reads a saved stream's usage: Claude's final result event,
// or the sum of Codex's completed turns.
func compareReadUsage(path, host string) (CompareUsage, error) {
	file, err := os.Open(path)
	if err != nil {
		return CompareUsage{}, err
	}
	defer file.Close()
	usage := CompareUsage{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		event := map[string]any{}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		switch {
		case host == "claude" && pairedString(event, "type") == "result":
			u := pairedObject(event, "usage")
			usage.InputTokens = pairedNumber(u, "input_tokens")
			usage.CacheReadTokens = pairedNumber(u, "cache_read_input_tokens")
			usage.CacheWriteTokens = pairedNumber(u, "cache_creation_input_tokens")
			usage.OutputTokens = pairedNumber(u, "output_tokens")
			usage.Turns = pairedNumber(event, "num_turns")
			usage.CostUSD, _ = event["total_cost_usd"].(float64)
		case host == "codex" && pairedString(event, "type") == "turn.completed":
			u := pairedObject(event, "usage")
			cached := pairedNumber(u, "cached_input_tokens")
			usage.InputTokens += pairedNumber(u, "input_tokens") - cached
			usage.CacheReadTokens += cached
			usage.OutputTokens += pairedNumber(u, "output_tokens")
			usage.Turns++
		}
	}
	return usage, scanner.Err()
}

func compareMustJSON(value any) []byte {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return []byte(fmt.Sprintf("{\"error\": %q}\n", err.Error()))
	}
	return append(data, '\n')
}

func writeCompareJSON(path string, value any) error {
	return os.WriteFile(path, compareMustJSON(value), 0o600)
}

// compareRunCommand runs a short command and returns its stdout.
func compareRunCommand(ctx context.Context, dir string, env []string, stdin string, name string, args ...string) (string, string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Env = env
	command.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	pairedConfigureProcess(command)
	command.WaitDelay = 2 * time.Second
	err := command.Run()
	return stdout.String(), stderr.String(), err
}
