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

// Phases of the evaluation. Preflight and report make no model call; smoke,
// apply and grow run live sessions against a signed-in subscription under one
// persistent attempt ceiling.
const (
	evalPhasePreflight = "preflight"
	evalPhaseSmoke     = "smoke"
	evalPhaseApply     = "apply"
	evalPhaseGrow      = "grow"
	evalPhaseReport    = "report"
)

// evalLivePhases are the phases that reserve attempts, in the order a
// preflight prepares their cells.
var evalLivePhases = []string{evalPhaseSmoke, evalPhaseApply, evalPhaseGrow}

// EvalOptions controls one invocation.
type EvalOptions struct {
	ManifestPath string
	Manifest     EvalManifest
	Fixture      EvalFixture
	Phase        string
	Dir          string
	// CellsDir is where this evaluation's cells are generated. Empty puts them
	// under the system temporary directory.
	CellsDir    string
	SandboxRoot string
	RepoRoot    string
	KapiBin     string
	Live        bool
	MaxAttempts int
	Sessions    string
	// Answers is the YAML file holding a person's answers to the review
	// sheet's questions. Empty reads review-answers.yaml in Dir.
	Answers string
}

type evalDependencies struct {
	prepare func(context.Context, EvalOptions, EvalSession) (EvalPrepared, error)
	run     func(context.Context, EvalPrepared) (EvalTranscript, error)
}

type evalStudyRecord struct {
	Schema       int          `json:"schema"`
	Fingerprint  string       `json:"fingerprint"`
	Manifest     EvalManifest `json:"manifest"`
	FixtureHash  string       `json:"fixture_hash"`
	CodeHash     string       `json:"code_hash"`
	SkillHash    string       `json:"skill_hash"`
	KapiHash     string       `json:"kapi_hash"`
	KapiVersion  string       `json:"kapi_version"`
	KapiCommit   string       `json:"kapi_commit"`
	SandboxRoot  string       `json:"sandbox_root"`
	UserDataRoot string       `json:"user_data_root"`
	CreatedAt    time.Time    `json:"created_at"`
}

type evalAttempt struct {
	Schema      int          `json:"schema"`
	Session     EvalSession  `json:"session"`
	Phase       string       `json:"phase"`
	StartedAt   time.Time    `json:"started_at"`
	Fingerprint string       `json:"fingerprint"`
	HostVersion string       `json:"host_version"`
	Prepared    EvalPrepared `json:"prepared"`
}

// evalAttemptResult is everything one session left behind, read through the
// shipped surfaces. The report scores it; nothing here is a verdict.
type evalAttemptResult struct {
	FinishedAt     time.Time      `json:"finished_at"`
	Status         string         `json:"status"`
	IdentityStatus string         `json:"identity_status"`
	Error          string         `json:"error,omitempty"`
	DurationMS     int64          `json:"duration_ms"`
	Transcript     EvalTranscript `json:"transcript"`
	// Baseline is the commit the session started from.
	Baseline string `json:"baseline"`
	// Changed are the files the session changed, and FirstAdded and FinalAdded
	// the text its first saved versions and its final versions added.
	Changed    []string `json:"changed"`
	FirstAdded string   `json:"first_added"`
	FinalAdded string   `json:"final_added"`
	// CheckFirst and CheckFinal are the check over each version, on a Measure 1
	// run.
	CheckFirst *EvalCheck `json:"check_first,omitempty"`
	CheckFinal *EvalCheck `json:"check_final,omitempty"`
	// Recorded are the context-log entries the session added.
	Recorded []EvalOperation `json:"recorded"`
	// HeldBefore and HeldAfter count the rules a person held, before and after
	// the session, so a run that took a rule away is visible.
	HeldBefore int `json:"held_before"`
	HeldAfter  int `json:"held_after"`
	// UserDataUntouched compares the person's own data root before and after.
	UserDataUntouched bool `json:"user_data_untouched"`
}

func executeEval(ctx context.Context, opts EvalOptions) error {
	return executeEvalWith(ctx, opts, evalDependencies{prepare: prepareEvalSession, run: runEvalAgent})
}

func executeEvalWith(ctx context.Context, opts EvalOptions, deps evalDependencies) error {
	switch opts.Phase {
	case evalPhasePreflight, evalPhaseSmoke, evalPhaseApply, evalPhaseGrow, evalPhaseReport:
	default:
		return fmt.Errorf("unknown evaluation phase %q", opts.Phase)
	}
	manifest, err := readEvalManifest(opts.ManifestPath)
	if err != nil {
		return err
	}
	opts.Manifest = manifest
	if opts.Fixture, err = generateEvalFixture(); err != nil {
		return err
	}
	if opts.Dir == "" {
		return errors.New("evaluation output directory is required")
	}
	if opts.Dir, err = filepath.Abs(opts.Dir); err != nil {
		return err
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(opts.Dir, "runner.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("acquire the evaluation lock (inspect stale locks after an interrupted runner): %w", err)
	}
	defer func() { _ = lock.Close(); _ = os.Remove(lock.Name()) }()
	if _, err := fmt.Fprintf(lock, "pid=%d\n", os.Getpid()); err != nil {
		return err
	}
	if opts.Phase == evalPhaseReport {
		return reportEval(opts)
	}
	opts.KapiBin = findKapi(opts.RepoRoot)
	record, err := makeEvalStudyRecord(ctx, opts)
	if err != nil {
		return err
	}
	// An evidence directory that already holds a study keeps its own cells, so
	// a later phase runs in the cells an earlier one prepared.
	opts.SandboxRoot = record.SandboxRoot
	var existing evalStudyRecord
	if err := readPairedJSON(filepath.Join(opts.Dir, "study.json"), &existing); err == nil && existing.SandboxRoot != "" {
		opts.SandboxRoot = existing.SandboxRoot
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(opts.SandboxRoot, 0o700); err != nil {
		return err
	}
	// The key is the evaluator's. It is written beside the evidence and never
	// into a cell.
	if err := pairedWriteJSON(filepath.Join(opts.Dir, "answer-key.json"), opts.Fixture); err != nil {
		return err
	}
	if opts.Phase == evalPhasePreflight || !opts.Live {
		return preflightEval(ctx, opts, deps)
	}
	if opts.MaxAttempts < 1 {
		return errors.New("live execution requires a positive eval-max-attempts ceiling")
	}
	if err := ensureEvalStudy(opts.Dir, record); err != nil {
		return err
	}
	schedule, err := selectEvalSessions(evalSchedule(manifest, opts.Phase), opts.Sessions)
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
	return runEvalSchedule(ctx, opts, record, schedule, deps)
}

func makeEvalStudyRecord(ctx context.Context, opts EvalOptions) (evalStudyRecord, error) {
	record := evalStudyRecord{Schema: evalSchema, Manifest: opts.Manifest, CreatedAt: time.Now().UTC(),
		UserDataRoot: evalUserDataRoot()}
	var err error
	if record.FixtureHash, err = evalFixtureHash(opts.Fixture); err != nil {
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
		record.KapiVersion = firstLine(evalProbe(ctx, opts.KapiBin, "--version"))
	}
	// The commit is provenance. It stays out of the fingerprint, which binds the
	// inputs an evaluation actually runs on: the manifest, the fixture with its
	// key and prompts, the runner's source, the shipped skill and the binary.
	record.KapiCommit = firstLine(evalProbe(ctx, "git", "-C", opts.RepoRoot, "rev-parse", "HEAD"))
	record.Fingerprint, err = pairedHash(struct {
		Manifest                   EvalManifest
		Fixture, Code, Skill, Kapi string
	}{Manifest: opts.Manifest, Fixture: record.FixtureHash, Code: record.CodeHash,
		Skill: record.SkillHash, Kapi: record.KapiHash})
	if err != nil {
		return record, err
	}
	// The cells live outside this repository, under a directory named for the
	// study, so one evaluation's cells survive between phases and two
	// evaluations never share one.
	cells, err := evalCellsDir(opts)
	if err != nil {
		return record, err
	}
	record.SandboxRoot = filepath.Join(cells, "kapi-eval-"+opts.Manifest.Study+"-"+record.Fingerprint[:12])
	return record, nil
}

// evalCellsDir answers where this evaluation's cells are generated.
//
// The default is the system temporary directory, which macOS sweeps after a few
// days, and a person's review can come later than that. EVAL_CELLS_DIR names a
// directory that survives instead. A path inside this checkout is refused: an
// agent host walks up from its working directory looking for CLAUDE.md and kapi
// walks up looking for kapi.yaml, so a cell there would bind to neokapi's own
// project. What sits above the chosen directory is measured per cell by
// evalAncestorFindings, which blocks the batch on a finding.
func evalCellsDir(opts EvalOptions) (string, error) {
	if strings.TrimSpace(opts.CellsDir) == "" {
		return os.TempDir(), nil
	}
	dir, err := filepath.Abs(opts.CellsDir)
	if err != nil {
		return "", err
	}
	repo, err := filepath.Abs(opts.RepoRoot)
	if err != nil {
		return "", err
	}
	if opts.RepoRoot != "" {
		inside, err := filepath.Rel(repo, dir)
		if err == nil && inside != ".." && !strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("cells directory %s sits inside this checkout; the evaluation generates its repositories outside the tree", dir)
		}
	}
	return dir, nil
}

// evalProbe captures a short command's stdout for a provenance field. A failure
// leaves the field blank rather than stopping the evaluation.
func evalProbe(ctx context.Context, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func ensureEvalStudy(dir string, record evalStudyRecord) error {
	path := filepath.Join(dir, "study.json")
	var existing evalStudyRecord
	if err := readPairedJSON(path, &existing); err == nil {
		if existing.Fingerprint != record.Fingerprint {
			return errors.New("evaluation inputs changed; choose a fresh EVAL_DIR to retain the existing evidence")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writePairedJSON(path, record)
}

type evalPreflight struct {
	Schema   int            `json:"schema"`
	Offline  bool           `json:"offline"`
	Sessions int            `json:"sessions"`
	Prepared []EvalPrepared `json:"prepared"`
	Blockers []string       `json:"blockers"`
	Notes    []string       `json:"notes"`
}

// preflightEval prepares every cell the live phases run, without inference.
// Preparation is the expensive part of the evaluation and the part most likely
// to be wrong, so it happens before a single subscription-backed session. A
// cell prepared here is the cell its live phase runs in.
func preflightEval(ctx context.Context, opts EvalOptions, deps evalDependencies) error {
	phases := evalLivePhases
	if opts.Phase != evalPhasePreflight {
		phases = []string{opts.Phase}
	}
	schedule := []EvalSession{}
	for _, phase := range phases {
		selected, err := selectEvalSessions(evalSchedule(opts.Manifest, phase), "")
		if err != nil {
			return err
		}
		schedule = append(schedule, selected...)
	}
	schedule, err := selectEvalSessions(schedule, opts.Sessions)
	if err != nil {
		return err
	}
	report := evalPreflight{Schema: evalSchema, Offline: true, Sessions: len(schedule),
		Prepared: []EvalPrepared{}, Blockers: []string{}, Notes: []string{}}
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
		for _, note := range prepared.Notes {
			report.Notes = pairedUnique(report.Notes, note)
		}
		report.Prepared = append(report.Prepared, prepared)
	}
	path := filepath.Join(opts.Dir, "preflight-"+pairedTimestamp()+".json")
	if err := writePairedJSON(path, report); err != nil {
		return err
	}
	fmt.Printf("eval: offline preflight; %d cells prepared, %d blockers; %s\n",
		len(report.Prepared), len(report.Blockers), path)
	for _, prepared := range report.Prepared {
		line := "  " + prepared.Session.ID + ":"
		if prepared.Server != nil {
			line += fmt.Sprintf(" server %s %s (under test: %t, %d tools)", prepared.Server.Name,
				prepared.Server.Version, prepared.Server.UnderTest, len(prepared.Server.Tools))
		}
		if prepared.Session.Measure == evalMeasureApply {
			line += fmt.Sprintf(", %d rules held", len(prepared.Wiring.Held))
		}
		if prepared.Codex != nil {
			line += ", codex sees " + evalToolList(prepared.Codex.Servers)
		}
		fmt.Println(line)
	}
	for _, note := range report.Notes {
		fmt.Println("  note: " + note)
	}
	for _, blocker := range report.Blockers {
		fmt.Println("  blocker: " + blocker)
	}
	if len(report.Blockers) != 0 {
		return fmt.Errorf("preflight found %d blockers; no live phase can run until they are cleared", len(report.Blockers))
	}
	return nil
}

func runEvalSchedule(ctx context.Context, opts EvalOptions, record evalStudyRecord, schedule []EvalSession, deps evalDependencies) error {
	used, err := evalAttemptsUsed(opts.Dir)
	if err != nil {
		return err
	}
	if used >= opts.MaxAttempts {
		fmt.Printf("eval: paused at persistent ceiling (%d/%d attempts); review saved results before authorizing more\n", used, opts.MaxAttempts)
		return nil
	}
	pending := []EvalSession{}
	for _, session := range schedule {
		if _, err := os.Stat(filepath.Join(opts.Dir, opts.Phase, session.ID, "started.json")); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		pending = append(pending, session)
	}
	pending = pending[:min(len(pending), opts.MaxAttempts-used)]
	prepared := map[string]EvalPrepared{}
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
		return fmt.Errorf("live evaluation blocked before inference: %s", strings.Join(blockers, "; "))
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
		ready.Env = append(ready.Env, "KAPI_EVAL_TRANSCRIPT="+filepath.Join(dir, "transcript.jsonl"))
		attempt := evalAttempt{
			Schema: evalSchema, Session: session, Phase: opts.Phase, StartedAt: time.Now().UTC(),
			Fingerprint: record.Fingerprint, HostVersion: ready.Version, Prepared: ready,
		}
		// Reserve before launching. A crash, a timeout or a malformed stream
		// consumes one attempt and leaves this record behind.
		if err := writePairedJSON(filepath.Join(dir, "started.json"), attempt); err != nil {
			return err
		}
		used++
		fmt.Printf("eval: %s (%d/%d authorized attempts)\n", session.ID, used, opts.MaxAttempts)
		result := executeEvalAttempt(ctx, ready, dir, deps)
		if err := writePairedJSON(filepath.Join(dir, "result.json"), result); err != nil {
			return err
		}
		fmt.Printf("  status %s; changed %d files; recorded %d; kapi calls %s\n", result.Status,
			len(result.Changed), len(result.Recorded), evalCallSummary(result.Transcript))
		if result.Transcript.RateLimited {
			fmt.Println("eval: provider rate limit; paused without automatic retry")
			break
		}
	}
	if used >= opts.MaxAttempts {
		fmt.Printf("eval: batch ceiling reached (%d); review before increasing EVAL_MAX_ATTEMPTS\n", used)
	}
	return reportEval(opts)
}

// executeEvalAttempt runs one session and reads its effect back through the
// shipped surfaces: the versions of what it wrote, the check over each on a
// Measure 1 cell, and what it recorded.
func executeEvalAttempt(ctx context.Context, ready EvalPrepared, dir string, deps evalDependencies) evalAttemptResult {
	result := evalAttemptResult{Status: "blocked", Baseline: ready.Wiring.Baseline, Changed: []string{}, Recorded: []EvalOperation{}}
	note := func(label string, err error) {
		if err != nil {
			result.Error = strings.TrimSpace(result.Error + "; " + label + ": " + err.Error())
		}
	}
	before, err := evalReadStore(ctx, ready.Paths)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.HeldBefore = len(evalHeldTerms(before))
	watcher, err := newEvalWatcher(ready.Paths.Repo)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	witnessBefore := ready.Isolation.UserDataWitness
	started := time.Now()
	attemptCtx, cancel := context.WithTimeout(ctx, ready.Timeout)
	defer cancel()
	watchCtx, stopWatching := context.WithCancel(ctx)
	watched := make(chan struct{})
	go func() { watcher.run(watchCtx); close(watched) }()
	transcript, runErr := deps.run(attemptCtx, ready)
	stopWatching()
	<-watched
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
	names, baseline, first, final, err := watcher.versions()
	note("versions", err)
	result.Changed = names
	note("save versions", evalSaveVersions(dir, first, final))
	result.FirstAdded, result.FinalAdded = evalAddedAcross(names, baseline, first), evalAddedAcross(names, baseline, final)
	note("commit", evalGitCommit(ctx, ready.Paths, "Session: "+ready.Session.Task))
	if ready.Session.Measure == evalMeasureApply && result.Baseline != "" {
		finalCheck := evalCheckAgainst(ctx, ready.Paths, result.Baseline)
		result.CheckFinal = &finalCheck
		firstCheck, err := evalCheckFirstVersion(ctx, ready.Paths, result.Baseline, first)
		note("first version", err)
		result.CheckFirst = &firstCheck
	}
	after, err := evalReadStore(ctx, ready.Paths)
	note("store", err)
	if err == nil {
		result.Recorded = evalRecordedSince(before, after)
		result.HeldAfter = len(evalHeldTerms(after))
		note("save context log", writePairedJSON(filepath.Join(dir, "context-log.json"), after))
	}
	witnessAfter, witnessErr := evalWitness(ready.Isolation.UserDataRoot)
	note("witness", witnessErr)
	result.UserDataUntouched = !evalWitnessChanged(witnessBefore, witnessAfter)
	return result
}

// evalAddedAcross joins what one version of every changed file added.
func evalAddedAcross(names []string, baseline, version map[string][]byte) string {
	parts := []string{}
	for _, name := range names {
		if added := evalAddedText(string(baseline[name]), string(version[name])); added != "" {
			parts = append(parts, added)
		}
	}
	return strings.Join(parts, "\n")
}

// evalSaveVersions keeps both versions of every changed file beside the
// attempt, so a person can read what was scored.
func evalSaveVersions(dir string, first, final map[string][]byte) error {
	for label, version := range map[string]map[string][]byte{"first": first, "final": final} {
		for name, data := range version {
			if data == nil {
				continue
			}
			path := filepath.Join(dir, label, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

// evalCheckFirstVersion checks the first saved version of the session's work.
// The session's final state is committed; the first versions are laid over the
// baseline in the same working tree, checked against the baseline, and the
// tree is returned to the committed final state.
func evalCheckFirstVersion(ctx context.Context, paths EvalPaths, baseline string, first map[string][]byte) (EvalCheck, error) {
	head, err := evalGitHead(ctx, paths)
	if err != nil {
		return EvalCheck{Findings: []EvalCheckFinding{}, Error: err.Error()}, err
	}
	restore := func() error {
		if err := evalRunGit(ctx, paths, "reset", "--quiet", "--hard", head); err != nil {
			return err
		}
		return evalRunGit(ctx, paths, "clean", "--quiet", "-fd")
	}
	if err := evalRunGit(ctx, paths, "checkout", "--quiet", baseline, "--", "."); err != nil {
		return EvalCheck{Findings: []EvalCheckFinding{}, Error: err.Error()}, errors.Join(err, restore())
	}
	for name, data := range first {
		path := filepath.Join(paths.Repo, filepath.FromSlash(name))
		if data == nil {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return EvalCheck{Findings: []EvalCheckFinding{}, Error: err.Error()}, errors.Join(err, restore())
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return EvalCheck{Findings: []EvalCheckFinding{}, Error: err.Error()}, errors.Join(err, restore())
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return EvalCheck{Findings: []EvalCheckFinding{}, Error: err.Error()}, errors.Join(err, restore())
		}
	}
	check := evalCheckAgainst(ctx, paths, baseline)
	return check, restore()
}

func evalCallSummary(t EvalTranscript) string {
	names := evalToolNames(t.kapiCalls())
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// runEvalAgent launches the host, records its stream, and reads the saved
// transcript back. Recording and scoring are separate so a scoring change can
// be applied to sessions already run.
func runEvalAgent(ctx context.Context, prepared EvalPrepared) (EvalTranscript, error) {
	transcript := EvalTranscript{Host: prepared.Session.Host.Host, Status: "blocked", Calls: []EvalCall{}}
	if prepared.blocked() {
		return transcript, fmt.Errorf("session preparation blocked: %s", strings.Join(prepared.Blockers, "; "))
	}
	if prepared.Timeout <= 0 {
		return transcript, errors.New("positive attempt timeout required")
	}
	path := evalTranscriptPath(prepared.Env)
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
	transcript, scanErr := scanEvalTranscript(path, prepared.Session.Host.Host)
	transcript.FinalText = pairedRedactText(transcript.FinalText, prepared.Env)
	evalRecoverIdentity(&transcript, prepared)
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

// evalRecoverIdentity fills in the model a host's stream left out. Codex
// records the turn's model in the rollout file it keeps beside the session, so
// identity is established from the host's own record rather than left unknown.
func evalRecoverIdentity(transcript *EvalTranscript, prepared EvalPrepared) {
	if transcript.ActualModel != "" || transcript.Host != "codex" || transcript.SessionID == "" {
		return
	}
	if model, err := pairedCodexRolloutModel(prepared.Paths.State, transcript.SessionID); err == nil {
		transcript.ActualModel = model
	}
}

// evalTranscriptPath reads the recording path off the attempt environment,
// which is where the schedule put it.
func evalTranscriptPath(env []string) string {
	for _, pair := range env {
		if key, value, ok := strings.Cut(pair, "="); ok && key == "KAPI_EVAL_TRANSCRIPT" {
			return value
		}
	}
	return ""
}

func evalAttemptsUsed(dir string) (int, error) {
	paths, err := evalAttemptPaths(dir)
	return len(paths), err
}

func evalAttemptPaths(dir string) ([]string, error) {
	attempts := []string{}
	for _, phase := range evalLivePhases {
		paths, err := filepath.Glob(filepath.Join(dir, phase, "*", "started.json"))
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, paths...)
	}
	return attempts, nil
}
