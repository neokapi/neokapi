package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Phases of the drill. Preflight, review and report make no model call; smoke,
// session one and session two run live sessions against a signed-in
// subscription under a persistent attempt ceiling.
const (
	coldStartPhasePreflight  = "preflight"
	coldStartPhaseSmoke      = "smoke"
	coldStartPhaseSessionOne = "session-one"
	coldStartPhaseReview     = "review"
	coldStartPhaseSessionTwo = "session-two"
	coldStartPhaseReport     = "report"
)

// coldStartLivePhases are the phases that reserve attempts.
var coldStartLivePhases = []string{coldStartPhaseSmoke, coldStartPhaseSessionOne, coldStartPhaseSessionTwo}

// ColdStartOptions controls one invocation.
type ColdStartOptions struct {
	ManifestPath string
	Manifest     ColdStartManifest
	Phase        string
	Dir          string
	SandboxRoot  string
	RepoRoot     string
	KapiBin      string
	Live         bool
	MaxAttempts  int
	Sessions     string
}

type coldStartDependencies struct {
	prepare func(context.Context, ColdStartOptions, ColdStartSession) (ColdStartPrepared, error)
	run     func(context.Context, ColdStartPrepared) (ColdStartTranscript, error)
}

type coldStartStudyRecord struct {
	Schema       int               `json:"schema"`
	Fingerprint  string            `json:"fingerprint"`
	Manifest     ColdStartManifest `json:"manifest"`
	FixtureHash  string            `json:"fixture_hash"`
	CodeHash     string            `json:"code_hash"`
	SkillHash    string            `json:"skill_hash"`
	KapiHash     string            `json:"kapi_hash"`
	KapiVersion  string            `json:"kapi_version"`
	KapiCommit   string            `json:"kapi_commit"`
	SandboxRoot  string            `json:"sandbox_root"`
	UserDataRoot string            `json:"user_data_root"`
	CreatedAt    time.Time         `json:"created_at"`
}

type coldStartAttempt struct {
	Schema      int               `json:"schema"`
	Session     ColdStartSession  `json:"session"`
	Phase       string            `json:"phase"`
	StartedAt   time.Time         `json:"started_at"`
	Fingerprint string            `json:"fingerprint"`
	HostVersion string            `json:"host_version"`
	Prepared    ColdStartPrepared `json:"prepared"`
}

type coldStartAttemptResult struct {
	FinishedAt     time.Time           `json:"finished_at"`
	Status         string              `json:"status"`
	IdentityStatus string              `json:"identity_status"`
	Error          string              `json:"error,omitempty"`
	DurationMS     int64               `json:"duration_ms"`
	Transcript     ColdStartTranscript `json:"transcript"`
	StoreBefore    ColdStartStore      `json:"store_before"`
	StoreAfter     ColdStartStore      `json:"store_after"`
	Check          *ColdStartCheck     `json:"check,omitempty"`
	Changed        []string            `json:"changed"`
	// UserDataUntouched compares the person's own data root before and after.
	UserDataUntouched bool `json:"user_data_untouched"`
}

func executeColdStart(ctx context.Context, opts ColdStartOptions) error {
	return executeColdStartWith(ctx, opts, coldStartDependencies{prepare: prepareColdStartSession, run: runColdStartAgent})
}

func executeColdStartWith(ctx context.Context, opts ColdStartOptions, deps coldStartDependencies) error {
	switch opts.Phase {
	case coldStartPhasePreflight, coldStartPhaseSmoke, coldStartPhaseSessionOne,
		coldStartPhaseReview, coldStartPhaseSessionTwo, coldStartPhaseReport:
	default:
		return fmt.Errorf("unknown cold-start phase %q", opts.Phase)
	}
	manifest, err := readColdStartManifest(opts.ManifestPath)
	if err != nil {
		return err
	}
	opts.Manifest = manifest
	if opts.Dir == "" {
		return errors.New("cold-start output directory is required")
	}
	if opts.Dir, err = filepath.Abs(opts.Dir); err != nil {
		return err
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(opts.Dir, "runner.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("acquire drill lock (inspect stale locks after an interrupted runner): %w", err)
	}
	defer func() { _ = lock.Close(); _ = os.Remove(lock.Name()) }()
	if _, err := fmt.Fprintf(lock, "pid=%d\n", os.Getpid()); err != nil {
		return err
	}
	opts.KapiBin = findKapi(opts.RepoRoot)
	record, err := makeColdStartStudyRecord(opts)
	if err != nil {
		return err
	}
	opts.SandboxRoot = record.SandboxRoot
	if err := os.MkdirAll(opts.SandboxRoot, 0o700); err != nil {
		return err
	}
	switch opts.Phase {
	case coldStartPhaseReport:
		return reportColdStart(ctx, opts)
	case coldStartPhaseReview:
		if err := ensureColdStartStudy(opts.Dir, record); err != nil {
			return err
		}
		return reviewColdStart(ctx, opts)
	case coldStartPhasePreflight:
		return preflightColdStart(ctx, opts, deps)
	}
	if !opts.Live {
		return preflightColdStart(ctx, opts, deps)
	}
	if opts.MaxAttempts < 1 {
		return errors.New("live execution requires a positive coldstart-max-attempts ceiling")
	}
	if err := ensureColdStartStudy(opts.Dir, record); err != nil {
		return err
	}
	schedule, err := selectColdStartSessions(coldStartSchedule(manifest, opts.Phase), opts.Sessions)
	if err != nil {
		return err
	}
	phaseDir := filepath.Join(opts.Dir, opts.Phase)
	if err := os.MkdirAll(phaseDir, 0o700); err != nil {
		return err
	}
	if err := ensurePairedJSON(filepath.Join(phaseDir, "schedule.json"), schedule); err != nil {
		return err
	}
	return runColdStartSchedule(ctx, opts, record, schedule, deps)
}

func makeColdStartStudyRecord(opts ColdStartOptions) (coldStartStudyRecord, error) {
	record := coldStartStudyRecord{Schema: coldStartSchema, Manifest: opts.Manifest, CreatedAt: time.Now().UTC(),
		UserDataRoot: coldStartUserDataRoot()}
	var err error
	if record.FixtureHash, err = coldStartFixtureHash(); err != nil {
		return record, err
	}
	if record.CodeHash, err = pairedTreeHash(filepath.Join(opts.RepoRoot, "scripts", "skilleval")); err != nil {
		return record, err
	}
	if record.SkillHash, err = pairedTreeHash(filepath.Join(opts.RepoRoot, "cli", "skills", "data", "kapi")); err != nil {
		return record, err
	}
	record.KapiHash, record.KapiVersion = "unavailable", "unavailable"
	if opts.KapiBin != "" {
		if record.KapiHash, err = pairedFileHash(opts.KapiBin); err != nil {
			return record, err
		}
		record.KapiVersion = firstLine(run(opts.KapiBin, "--version"))
	}
	record.KapiCommit = firstLine(run("git", "-C", opts.RepoRoot, "rev-parse", "HEAD"))
	record.Fingerprint, err = pairedHash(struct {
		Manifest                        ColdStartManifest
		Fixture, Code, Skill, Kapi, Git string
	}{Manifest: opts.Manifest, Fixture: record.FixtureHash, Code: record.CodeHash,
		Skill: record.SkillHash, Kapi: record.KapiHash, Git: record.KapiCommit})
	if err != nil {
		return record, err
	}
	// The fixture lives outside this repository, under a directory named for the
	// study, so the cells of one drill survive between phases and two drills
	// never share one.
	record.SandboxRoot = filepath.Join(os.TempDir(), "kapi-coldstart-"+opts.Manifest.Study+"-"+record.Fingerprint[:12])
	return record, nil
}

func ensureColdStartStudy(dir string, record coldStartStudyRecord) error {
	path := filepath.Join(dir, "study.json")
	var existing coldStartStudyRecord
	if err := readPairedJSON(path, &existing); err == nil {
		if existing.Fingerprint != record.Fingerprint {
			return errors.New("drill inputs changed; choose a fresh coldstart-dir to retain the existing evidence")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writePairedJSON(path, record)
}

type coldStartPreflight struct {
	Schema      int                 `json:"schema"`
	Phase       string              `json:"phase"`
	Offline     bool                `json:"offline"`
	Sessions    int                 `json:"sessions"`
	Prepared    []ColdStartPrepared `json:"prepared"`
	Blockers    []string            `json:"blockers"`
	HumanReview string              `json:"human_review"`
}

// preflightColdStart prepares every cell a phase would run, without inference.
// Preparation is the expensive part of the drill and the part most likely to be
// wrong, so it happens before a single subscription-backed session.
func preflightColdStart(ctx context.Context, opts ColdStartOptions, deps coldStartDependencies) error {
	phase := opts.Phase
	if phase == coldStartPhasePreflight {
		phase = coldStartPhaseSmoke
	}
	schedule, err := selectColdStartSessions(coldStartSchedule(opts.Manifest, phase), opts.Sessions)
	if err != nil {
		return err
	}
	report := coldStartPreflight{
		Schema: coldStartSchema, Phase: phase, Offline: true, Sessions: len(schedule),
		Prepared: []ColdStartPrepared{}, Blockers: []string{},
		HumanReview: "unmeasured; only a person's own confirmations count on the second measure",
	}
	for _, session := range schedule {
		if err := ctx.Err(); err != nil {
			return err
		}
		prepared, err := deps.prepare(ctx, opts, session)
		if err != nil {
			report.Blockers = append(report.Blockers, session.ID+": "+err.Error())
		}
		for _, blocker := range prepared.Blockers {
			report.Blockers = append(report.Blockers, session.ID+": "+blocker)
		}
		report.Prepared = append(report.Prepared, prepared)
	}
	path := filepath.Join(opts.Dir, "preflight-"+pairedTimestamp()+".json")
	if err := writePairedJSON(path, report); err != nil {
		return err
	}
	fmt.Printf("cold start: offline preflight for %s; %d prepared, %d live blockers; %s\n",
		phase, len(report.Prepared), len(report.Blockers), path)
	for _, prepared := range report.Prepared {
		if prepared.Server != nil {
			fmt.Printf("  %s: server %s %s from %s (under test: %t)\n", prepared.Session.ID,
				prepared.Server.Name, prepared.Server.Version, prepared.Server.Resolved, prepared.Server.UnderTest)
		}
	}
	for _, blocker := range report.Blockers {
		fmt.Println("  blocker: " + blocker)
	}
	return nil
}

func runColdStartSchedule(ctx context.Context, opts ColdStartOptions, record coldStartStudyRecord, schedule []ColdStartSession, deps coldStartDependencies) error {
	used, err := coldStartAttemptsUsed(opts.Dir)
	if err != nil {
		return err
	}
	if used >= opts.MaxAttempts {
		fmt.Printf("cold start: paused at persistent ceiling (%d/%d attempts); review saved results before authorizing more\n", used, opts.MaxAttempts)
		return nil
	}
	pending := []ColdStartSession{}
	for _, session := range schedule {
		if _, err := os.Stat(filepath.Join(opts.Dir, opts.Phase, session.ID, "started.json")); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		pending = append(pending, session)
	}
	pending = pending[:min(len(pending), opts.MaxAttempts-used)]
	prepared := map[string]ColdStartPrepared{}
	blockers := []string{}
	for _, session := range pending {
		if err := ctx.Err(); err != nil {
			return err
		}
		dir := filepath.Join(opts.Dir, opts.Phase, session.ID)
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		ready, prepareErr := deps.prepare(ctx, opts, session)
		if err := writePairedJSON(filepath.Join(dir, "preparation.json"), ready); err != nil {
			return err
		}
		if prepareErr != nil {
			blockers = append(blockers, session.ID+": "+prepareErr.Error())
		}
		for _, blocker := range ready.Blockers {
			blockers = append(blockers, session.ID+": "+blocker)
		}
		if ready.Version == "" {
			blockers = append(blockers, session.ID+": agent version is unverified")
		}
		if err := writePairedExclusive(filepath.Join(dir, "prompt.txt"), []byte(ready.Prompt+"\n")); err != nil {
			return err
		}
		prepared[session.ID] = ready
	}
	if len(blockers) > 0 {
		if err := writePairedJSON(filepath.Join(opts.Dir, "blocked-"+pairedTimestamp()+".json"), blockers); err != nil {
			return err
		}
		return fmt.Errorf("live drill blocked before inference: %s", strings.Join(blockers, "; "))
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
		ready.Env = append(ready.Env, "KAPI_COLDSTART_TRANSCRIPT="+filepath.Join(dir, "transcript.jsonl"))
		attempt := coldStartAttempt{
			Schema: coldStartSchema, Session: session, Phase: opts.Phase, StartedAt: time.Now().UTC(),
			Fingerprint: record.Fingerprint, HostVersion: ready.Version, Prepared: ready,
		}
		// Reserve before launching. A crash, a timeout or a malformed stream
		// consumes one attempt and leaves this record behind.
		if err := writePairedJSON(filepath.Join(dir, "started.json"), attempt); err != nil {
			return err
		}
		used++
		fmt.Printf("cold start: %s (%d/%d authorized attempts)\n", session.ID, used, opts.MaxAttempts)
		result := executeColdStartAttempt(ctx, opts, ready, dir, deps)
		if err := writePairedJSON(filepath.Join(dir, "result.json"), result); err != nil {
			return err
		}
		fmt.Printf("  status %s; recorded %s; kapi calls %s\n", result.Status,
			result.StoreAfter.kindSummary(), coldStartCallSummary(result.Transcript))
		if result.Transcript.RateLimited {
			fmt.Println("cold start: provider rate limit; paused without automatic retry")
			break
		}
	}
	if used >= opts.MaxAttempts {
		fmt.Printf("cold start: batch ceiling reached (%d); review before increasing coldstart-max-attempts\n", used)
	}
	return reportColdStart(ctx, opts)
}

// executeColdStartAttempt runs one session and reads its effect back through
// the shipped surfaces, leaving the cell committed so the next stage can name
// what changed.
func executeColdStartAttempt(ctx context.Context, opts ColdStartOptions, ready ColdStartPrepared, dir string, deps coldStartDependencies) coldStartAttemptResult {
	result := coldStartAttemptResult{Status: "blocked", Changed: []string{}}
	before, err := coldStartReadStore(ctx, ready.Paths)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.StoreBefore = before
	witnessBefore := ready.Isolation.UserDataWitness
	started := time.Now()
	attemptCtx, cancel := context.WithTimeout(ctx, ready.Timeout)
	defer cancel()
	transcript, runErr := deps.run(attemptCtx, ready)
	result.DurationMS = time.Since(started).Milliseconds()
	result.Transcript, result.Status = transcript, transcript.Status
	result.FinishedAt = time.Now().UTC()
	if runErr != nil {
		result.Error = runErr.Error()
	}
	if attemptCtx.Err() != nil {
		result.Status = "interrupted"
		if errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
			result.Status = "timeout"
		}
	}
	switch {
	case transcript.ActualModel == ready.Session.Host.Model:
		result.IdentityStatus = "verified"
	case transcript.ActualModel != "":
		result.IdentityStatus = "mismatch"
		result.Status = "invalid-identity"
	default:
		result.IdentityStatus = "unavailable"
	}
	if ready.Session.Stage == coldStartStageTwo {
		check := coldStartCheckChanges(ctx, ready.Paths, "HEAD")
		result.Check = &check
	}
	result.Changed = coldStartChangedFiles(ctx, ready.Paths)
	after, err := coldStartReadStore(ctx, ready.Paths)
	if err != nil {
		result.Error = strings.TrimSpace(result.Error + "; store: " + err.Error())
	} else {
		result.StoreAfter = after
	}
	witnessAfter, witnessErr := coldStartWitness(ready.Isolation.UserDataRoot)
	if witnessErr != nil {
		result.Error = strings.TrimSpace(result.Error + "; witness: " + witnessErr.Error())
	}
	result.UserDataUntouched = !coldStartWitnessChanged(witnessBefore, witnessAfter)
	if commitErr := coldStartGitCommit(ctx, ready.Paths, "Session "+ready.Session.Stage+": "+ready.Session.Task); commitErr != nil {
		result.Error = strings.TrimSpace(result.Error + "; commit: " + commitErr.Error())
	}
	return result
}

// coldStartChangedFiles names what a session left in the working tree.
func coldStartChangedFiles(ctx context.Context, paths ColdStartPaths) []string {
	command := exec.CommandContext(ctx, "git", "status", "--porcelain")
	command.Dir = paths.Repo
	command.Env = coldStartEnv(paths)
	out, err := command.Output()
	if err != nil {
		return []string{}
	}
	changed := []string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if entry := strings.TrimSpace(line); entry != "" {
			changed = append(changed, entry)
		}
	}
	return changed
}

func coldStartCallSummary(t ColdStartTranscript) string {
	names := coldStartToolNames(t.kapiCalls())
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// runColdStartAgent launches the host, records its stream, and reads the saved
// transcript back. Recording and scoring are separate so a scoring change can
// be applied to sessions already run.
func runColdStartAgent(ctx context.Context, prepared ColdStartPrepared) (ColdStartTranscript, error) {
	transcript := ColdStartTranscript{Host: prepared.Session.Host.Host, Status: "blocked", Calls: []ColdStartCall{}}
	if prepared.blocked() {
		return transcript, fmt.Errorf("session preparation blocked: %s", strings.Join(prepared.Blockers, "; "))
	}
	if prepared.Timeout <= 0 {
		return transcript, errors.New("positive attempt timeout required")
	}
	path := coldStartTranscriptPath(prepared.Env)
	if path == "" {
		return transcript, errors.New("the attempt has no transcript path")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return transcript, err
	}
	defer file.Close()
	stderr, err := os.OpenFile(path+".stderr", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return transcript, err
	}
	defer stderr.Close()
	command := exec.CommandContext(ctx, prepared.Executable, prepared.Args...)
	command.Dir = prepared.Paths.Repo
	command.Env = prepared.Env
	command.Stdin = strings.NewReader(prepared.Prompt)
	stderrFilter := newPairedRedactor(stderr, prepared.Env)
	defer stderrFilter.Flush()
	command.Stderr = stderrFilter
	command.WaitDelay = 2 * time.Second
	pairedConfigureProcess(command)
	pipe, err := command.StdoutPipe()
	if err != nil {
		return transcript, err
	}
	defer pipe.Close()
	stopPipeClose := context.AfterFunc(ctx, func() { _ = pipe.Close() })
	defer stopPipeClose()
	if err := command.Start(); err != nil {
		transcript.Status = "launch_failed"
		return transcript, err
	}
	defer func() { _ = pairedStopProcess(command) }()
	recorder := newPairedRedactor(file, prepared.Env)
	_, copyErr := io.Copy(recorder, pipe)
	flushErr := recorder.Flush()
	waitErr := command.Wait()
	transcript, scanErr := scanColdStartTranscript(path, prepared.Session.Host.Host)
	transcript.FinalText = pairedRedactText(transcript.FinalText, prepared.Env)
	switch {
	case ctx.Err() != nil:
		return transcript, ctx.Err()
	case scanErr != nil:
		return transcript, scanErr
	case copyErr != nil:
		return transcript, copyErr
	case flushErr != nil:
		return transcript, flushErr
	case waitErr != nil:
		if transcript.Status == "completed" {
			transcript.Status = "process_failed"
		}
		return transcript, waitErr
	}
	return transcript, nil
}

// coldStartTranscriptPath reads the recording path off the attempt environment,
// which is where the schedule put it.
func coldStartTranscriptPath(env []string) string {
	for _, pair := range env {
		if key, value, ok := strings.Cut(pair, "="); ok && key == "KAPI_COLDSTART_TRANSCRIPT" {
			return value
		}
	}
	return ""
}

func coldStartAttemptsUsed(dir string) (int, error) {
	paths, err := coldStartAttemptPaths(dir)
	return len(paths), err
}

func coldStartAttemptPaths(dir string) ([]string, error) {
	attempts := []string{}
	for _, phase := range coldStartLivePhases {
		paths, err := filepath.Glob(filepath.Join(dir, phase, "*", "started.json"))
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, paths...)
	}
	return attempts, nil
}
