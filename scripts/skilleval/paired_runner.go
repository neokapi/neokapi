package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runPairedSchedule(
	ctx context.Context,
	opts PairedOptions,
	record pairedStudyRecord,
	schedule []PairedSession,
	deps pairedDependencies,
) error {
	used, err := pairedAttemptsUsed(opts.Dir)
	if err != nil {
		return err
	}
	if used >= opts.MaxAttempts {
		fmt.Printf("paired: paused at persistent ceiling (%d/%d attempts); review saved results before authorizing more\n",
			used, opts.MaxAttempts)
		return scorePaired(opts.Dir)
	}
	versions, err := pairedObservedVersions(opts.Dir)
	if err != nil {
		return err
	}
	pending := []PairedSession{}
	prepared := map[string]PairedPrepared{}
	for _, session := range schedule {
		dir := filepath.Join(opts.Dir, opts.Phase, session.ID)
		if _, err := os.Stat(filepath.Join(dir, "started.json")); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		pending = append(pending, session)
	}
	// Prepare only the authorized batch. Blockers prevent any inference in it.
	pending = pending[:min(len(pending), opts.MaxAttempts-used)]
	blockers := []string{}
	for _, session := range pending {
		if err := ctx.Err(); err != nil {
			return err
		}
		dir := filepath.Join(opts.Dir, opts.Phase, session.ID)
		// Only unstarted preparation directories are disposable. Started attempts
		// were filtered above and remain immutable, including failed ones.
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		launch, err := materializePairedLaunch(opts, record.Manifest, session, dir)
		if err != nil {
			return err
		}
		ready, err := deps.prepare(ctx, launch)
		// Keep capability evidence even when this batch is blocked before an
		// attempt starts. This file contains no inherited environment values.
		if writeErr := writePairedJSON(filepath.Join(dir, "preparation.json"), ready); writeErr != nil {
			return writeErr
		}
		if err != nil {
			blockers = append(blockers, session.ID+": "+err.Error())
		}
		for _, blocker := range ready.Blockers {
			blockers = append(blockers, session.ID+": "+blocker)
		}
		if ready.Version == "" {
			blockers = append(blockers, session.ID+": agent version is unverified")
		}
		if version := versions[session.Agent.Host]; version != "" && version != ready.Version {
			blockers = append(blockers, session.ID+": agent version changed; use a new study directory")
		}
		versions[session.Agent.Host] = ready.Version
		prepared[session.ID] = ready
	}
	if len(blockers) > 0 {
		if err := writePairedJSON(filepath.Join(opts.Dir, "blocked-"+pairedTimestamp()+".json"), blockers); err != nil {
			return err
		}
		return fmt.Errorf("live study blocked before inference: %s", strings.Join(blockers, "; "))
	}
	for _, session := range pending {
		if used >= opts.MaxAttempts {
			break
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		dir := filepath.Join(opts.Dir, opts.Phase, session.ID)
		ready := prepared[session.ID]
		attempt := pairedAttempt{
			Schema: pairedSchema, Session: session, Phase: opts.Phase, StartedAt: time.Now().UTC(),
			Fingerprint: record.Fingerprint, Prepared: ready,
		}
		// Reserve before launching. A crash, timeout or malformed stream consumes one
		// attempt and leaves this immutable record. Resume never retries it implicitly.
		if err := writePairedJSON(filepath.Join(dir, "started.json"), attempt); err != nil {
			return err
		}
		used++
		fmt.Printf("paired: %s (%d/%d authorized attempts)\n", session.ID, used, opts.MaxAttempts)
		result := executePairedAttempt(ctx, ready, session, deps)
		if err := writePairedJSON(filepath.Join(dir, "result.json"), result); err != nil {
			return err
		}
		if result.Agent.RateLimited {
			fmt.Println("paired: provider rate limit; paused without automatic retry")
			break
		}
	}
	if err := scorePaired(opts.Dir); err != nil {
		return err
	}
	if used >= opts.MaxAttempts {
		fmt.Printf("paired: batch ceiling reached (%d); review before increasing paired-max-attempts\n", used)
	}
	return nil
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
	validation, validationErr := validatePairedTask(ready.Launch.Workspace, task)
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
