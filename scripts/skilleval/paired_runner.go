package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// pairedRetryStatuses are the attempt outcomes -paired-retry runs again: the
// session never had its full chance, through a subscription limit, an
// interruption or a failed launch. A timeout, a failed agent and a refused
// route are outcomes and stay.
var pairedRetryStatuses = []string{"rate_limited", "interrupted", "launch_failed"}

type pairedSessionDone struct {
	host  string
	pause string
	err   error
}

// pairedRunState is what the concurrent sessions of one run share.
type pairedRunState struct {
	mu       sync.Mutex
	used     int
	versions map[string]string
}

func runPairedSchedule(
	ctx context.Context,
	opts PairedOptions,
	record pairedStudyRecord,
	schedule []PairedSession,
	deps pairedDependencies,
) error {
	if opts.Retry {
		retired, err := retirePairedAttempts(opts.Dir, opts.Phase, schedule)
		if err != nil {
			return err
		}
		if retired > 0 {
			fmt.Printf("paired: %d attempts will run again (their records are kept as superseded)\n", retired)
		}
	}
	used, err := pairedAttemptsUsed(opts.Dir)
	if err != nil {
		return err
	}
	versions, err := pairedObservedVersions(opts.Dir)
	if err != nil {
		return err
	}
	pending := []PairedSession{}
	for _, session := range schedule {
		dir := filepath.Join(opts.Dir, opts.Phase, session.ID)
		if _, err := os.Stat(filepath.Join(dir, "started.json")); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		pending = append(pending, session)
	}
	concurrency := max(1, opts.Concurrency)
	remaining := max(0, opts.MaxAttempts-used)
	batch := pending[:min(len(pending), remaining)]
	fmt.Printf("paired: %s phase plans %d sessions: %d started, %d pending. Ceiling %d attempts with %d used; "+
		"this run starts at most %d live sessions, %d at a time.\n",
		opts.Phase, len(schedule), len(schedule)-len(pending), len(pending), opts.MaxAttempts, used, len(batch), concurrency)
	if remaining == 0 {
		fmt.Printf("paired: paused at persistent ceiling (%d/%d attempts); review saved results before authorizing more\n",
			used, opts.MaxAttempts)
		return scorePaired(opts.Dir)
	}
	if len(batch) == 0 {
		fmt.Println("paired: every session in this phase has started")
		return scorePaired(opts.Dir)
	}
	state := &pairedRunState{used: used, versions: versions}

	// No inference starts until the first pending cell of every host and
	// condition in the batch has been prepared and its surface probed.
	prepared := map[string]PairedPrepared{}
	blockers := []string{}
	gated := map[string]bool{}
	for _, session := range batch {
		key := session.Agent.Host + "/" + session.Condition
		if gated[key] {
			continue
		}
		gated[key] = true
		if err := ctx.Err(); err != nil {
			return err
		}
		ready, problems, err := preparePairedSession(ctx, opts, record, session, deps, state)
		if err != nil {
			return err
		}
		blockers = append(blockers, problems...)
		prepared[session.ID] = ready
	}
	if len(blockers) > 0 {
		if err := writePairedJSON(filepath.Join(opts.Dir, "blocked-"+pairedTimestamp()+".json"), blockers); err != nil {
			return err
		}
		return fmt.Errorf("live study blocked before inference: %s", strings.Join(blockers, "; "))
	}

	// Each host's share of the concurrency holds for the whole run, so the
	// host whose sessions remain after the other's finish never takes the
	// other's share: its subscription sees the same load throughout.
	hosts := max(1, len(record.Manifest.Agents))
	perHost := max(1, (concurrency+hosts-1)/hosts)
	queue := slices.Clone(batch)
	busy, paused := map[string]int{}, map[string]string{}
	done := make(chan pairedSessionDone)
	running := 0
	var stopErr error
	for {
		for running < concurrency && ctx.Err() == nil && stopErr == nil {
			index := slices.IndexFunc(queue, func(s PairedSession) bool {
				return paused[s.Agent.Host] == "" && busy[s.Agent.Host] < perHost
			})
			if index < 0 {
				break
			}
			session := queue[index]
			queue = slices.Delete(queue, index, index+1)
			ready, ok := prepared[session.ID]
			busy[session.Agent.Host]++
			running++
			go func() {
				done <- runPairedSession(ctx, opts, record, session, ready, ok, deps, state)
			}()
		}
		if running == 0 {
			break
		}
		result := <-done
		running--
		busy[result.host]--
		if result.pause != "" && paused[result.host] == "" {
			paused[result.host] = result.pause
			fmt.Printf("paired: %s paused: %s\n", result.host, result.pause)
		}
		if result.err != nil && stopErr == nil {
			stopErr = result.err
		}
	}
	if err := scorePaired(opts.Dir); err != nil {
		return err
	}
	if stopErr != nil {
		return stopErr
	}
	if len(paused) > 0 {
		fmt.Println("paired: a paused host left sessions unstarted; run the same command again to resume, " +
			"with PAIRED_EVAL_RETRY=1 to run rate-limited or interrupted attempts again")
	}
	if state.used >= opts.MaxAttempts {
		fmt.Printf("paired: batch ceiling reached (%d); review before increasing paired-max-attempts\n", state.used)
	}
	return ctx.Err()
}

// preparePairedSession builds one cell, prepares its agent and probes its
// surface. It returns the problems that forbid inference in it; an error is a
// failure of the evaluator itself.
func preparePairedSession(
	ctx context.Context,
	opts PairedOptions,
	record pairedStudyRecord,
	session PairedSession,
	deps pairedDependencies,
	state *pairedRunState,
) (PairedPrepared, []string, error) {
	dir := filepath.Join(opts.Dir, opts.Phase, session.ID)
	// Only unstarted preparation directories are disposable. Started attempts
	// were filtered out and remain immutable, including failed ones.
	if err := os.RemoveAll(dir); err != nil {
		return PairedPrepared{}, nil, err
	}
	launch, err := materializePairedLaunch(opts, record.Manifest, session, dir)
	if err != nil {
		return PairedPrepared{}, nil, err
	}
	ready, err := deps.prepare(ctx, launch)
	problems := []string{}
	if err != nil {
		problems = append(problems, session.ID+": "+err.Error())
	}
	for _, blocker := range ready.Blockers {
		problems = append(problems, session.ID+": "+blocker)
	}
	if len(problems) == 0 && deps.probe != nil {
		surface := deps.probe(ctx, ready)
		ready.Surface = &surface
		for _, problem := range surface.Problems {
			problems = append(problems, session.ID+": surface: "+problem)
		}
	}
	if ready.Version == "" {
		problems = append(problems, session.ID+": agent version is unverified")
	}
	state.mu.Lock()
	if version := state.versions[session.Agent.Host]; version != "" && version != ready.Version {
		problems = append(problems, session.ID+": agent version changed; use a new study directory")
	} else if ready.Version != "" {
		state.versions[session.Agent.Host] = ready.Version
	}
	state.mu.Unlock()
	// Keep capability evidence even when the session is blocked before an
	// attempt starts. This file contains no inherited environment values.
	if err := writePairedJSON(filepath.Join(dir, "preparation.json"), ready); err != nil {
		return ready, problems, err
	}
	return ready, problems, nil
}

func runPairedSession(
	ctx context.Context,
	opts PairedOptions,
	record pairedStudyRecord,
	session PairedSession,
	ready PairedPrepared,
	havePrepared bool,
	deps pairedDependencies,
	state *pairedRunState,
) pairedSessionDone {
	done := pairedSessionDone{host: session.Agent.Host}
	if !havePrepared {
		var problems []string
		var err error
		ready, problems, err = preparePairedSession(ctx, opts, record, session, deps, state)
		if err != nil {
			done.err = err
			return done
		}
		if len(problems) > 0 {
			if err := writePairedJSON(filepath.Join(opts.Dir, "blocked-"+pairedTimestamp()+".json"), problems); err != nil {
				done.err = err
				return done
			}
			done.pause = "blocked before inference: " + strings.Join(problems, "; ")
			return done
		}
	}
	dir := filepath.Join(opts.Dir, opts.Phase, session.ID)
	state.mu.Lock()
	if state.used >= opts.MaxAttempts || ctx.Err() != nil {
		state.mu.Unlock()
		return done
	}
	attempt := pairedAttempt{
		Schema: pairedSchema, Session: session, Phase: opts.Phase, StartedAt: time.Now().UTC(),
		Fingerprint: record.Fingerprint, Prepared: ready,
	}
	// Reserve before launching. A crash, timeout or malformed stream consumes one
	// attempt and leaves this immutable record. Resume never retries it implicitly.
	if err := writePairedJSON(filepath.Join(dir, "started.json"), attempt); err != nil {
		state.mu.Unlock()
		done.err = err
		return done
	}
	state.used++
	count := state.used
	state.mu.Unlock()
	fmt.Printf("paired: %s (%d/%d authorized attempts)\n", session.ID, count, opts.MaxAttempts)
	result := executePairedAttempt(ctx, ready, session, deps)
	if err := writePairedJSON(filepath.Join(dir, "result.json"), result); err != nil {
		done.err = err
		return done
	}
	fmt.Printf("paired: %s %s in %.0fs, objective %s\n", session.ID, result.Agent.Status,
		float64(result.Agent.DurationMS)/1000, pairedObjectiveWord(result.Validation))
	if result.Agent.RateLimited {
		done.pause = "provider rate limit; paused without automatic retry"
	}
	return done
}

func pairedObjectiveWord(validation *PairedValidation) string {
	switch {
	case validation == nil:
		return "unscored"
	case validation.ObjectivePassed:
		return "passed"
	}
	return "not passed"
}

func executePairedAttempt(ctx context.Context, ready PairedPrepared, session PairedSession, deps pairedDependencies) pairedAttemptResult {
	attemptCtx, cancel := context.WithTimeout(ctx, ready.Launch.Timeout)
	defer cancel()
	agent, err := deps.run(attemptCtx, ready)
	result := pairedAttemptResult{FinishedAt: time.Now().UTC(), Agent: agent}
	if err != nil {
		result.Error = err.Error()
	}
	if attemptCtx.Err() != nil {
		result.Error = attemptCtx.Err().Error()
		result.Agent.Status = "interrupted"
		if errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
			result.Agent.Status = "timeout"
		}
	}
	switch {
	case agent.ActualModel == session.Agent.Model:
		result.IdentityStatus = "verified"
	case agent.ActualModel != "":
		result.IdentityStatus = "mismatch"
		result.Agent.Status = "invalid-identity"
		identityError := fmt.Sprintf("requested model %q; observed %q", session.Agent.Model, agent.ActualModel)
		result.Error = strings.Trim(result.Error+"; "+identityError, "; ")
	default:
		result.IdentityStatus = "unavailable"
		if result.Agent.Status == "completed" {
			result.Agent.Status = "invalid-identity"
			result.Error = "completed session did not report model identity"
		}
	}

	task, taskErr := findPairedTask(session.Task)
	if taskErr != nil {
		result.Error = taskErr.Error()
		return result
	}
	validation, validationErr := validatePairedTask(ready.Launch.Workspace, task, &result.Agent)
	if validationErr != nil {
		result.Error = strings.TrimSpace(result.Error + "; validation: " + validationErr.Error())
	} else {
		result.Validation = &validation
	}
	result.ArtifactHash, err = pairedTreeHash(ready.Launch.Workspace)
	if err != nil {
		result.Error = strings.TrimSpace(result.Error + "; artifact hash: " + err.Error())
	}
	return result
}

// retirePairedAttempts moves aside each attempt of the phase whose outcome
// pairedRetryStatuses lists, or that never recorded a result, so the schedule
// runs it again. The moved record keeps counting against the ceiling and is
// scored as superseded.
func retirePairedAttempts(dir, phase string, schedule []PairedSession) (int, error) {
	retired := 0
	for _, session := range schedule {
		attemptDir := filepath.Join(dir, phase, session.ID)
		if _, err := os.Stat(filepath.Join(attemptDir, "started.json")); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return retired, err
		}
		var result pairedAttemptResult
		err := readPairedJSON(filepath.Join(attemptDir, "result.json"), &result)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return retired, err
		}
		if err == nil && !slices.Contains(pairedRetryStatuses, result.Agent.Status) {
			continue
		}
		for n := 1; ; n++ {
			target := fmt.Sprintf("%s.retired-%d", attemptDir, n)
			if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
				if err := os.Rename(attemptDir, target); err != nil {
					return retired, err
				}
				break
			}
		}
		retired++
	}
	return retired, nil
}

func pairedAttemptsUsed(dir string) (int, error) {
	attempts, err := pairedAttemptPaths(dir)
	return len(attempts), err
}

func pairedAttemptPaths(dir string) ([]string, error) {
	attempts := []string{}
	for _, phase := range []string{"diagnostic", "smoke", "pilot"} {
		paths, err := filepath.Glob(filepath.Join(dir, phase, "*", "started.json"))
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, paths...)
	}
	return attempts, nil
}

func pairedObservedVersions(dir string) (map[string]string, error) {
	paths, err := pairedAttemptPaths(dir)
	if err != nil {
		return nil, err
	}
	versions := map[string]string{}
	for _, path := range paths {
		var attempt pairedAttempt
		if err := readPairedJSON(path, &attempt); err != nil {
			return nil, err
		}
		host, version := attempt.Session.Agent.Host, attempt.Prepared.Version
		if previous := versions[host]; previous != "" && previous != version {
			return nil, fmt.Errorf("study contains different versions of %s", host)
		}
		versions[host] = version
	}
	return versions, nil
}
