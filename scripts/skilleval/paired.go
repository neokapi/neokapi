package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// PairedOptions controls local orchestration. Live execution requires an explicit
// subscription-only batch; restarting the process never resets its attempt count.
type PairedOptions struct {
	ManifestPath string
	Phase        string
	Dir          string
	RepoRoot     string
	Live         bool
	MaxAttempts  int
	Sessions     string
	// Concurrency is how many sessions run at once, spread evenly over the
	// hosts. It is not part of the study's identity.
	Concurrency int
	// Retry runs again the attempts whose outcome pairedRetryStatuses lists.
	Retry bool

	// kapiBin and skillSource are the kapi binary and skill the cells get:
	// the study's own copies in a live phase, the checkout's in a preflight.
	kapiBin     string
	skillSource string
}

type pairedDependencies struct {
	prepare func(context.Context, PairedLaunch) (PairedPrepared, error)
	run     func(context.Context, PairedPrepared) (PairedAgentResult, error)
	// probe reads a prepared cell's surface without a model call. Nil skips it.
	probe func(context.Context, PairedPrepared) PairedSurface
	// build checks that bin/kapi is this tree's build. Nil skips it.
	build func(context.Context, string) (pairedBuild, error)
}

// pairedStudyRecord is study.json, written when a study starts. Its
// fingerprint binds every later run of the study to the same manifest,
// corpus, runner code, and the study's own copies of kapi and its skill.
type pairedStudyRecord struct {
	Schema      int            `json:"schema"`
	Fingerprint string         `json:"fingerprint"`
	Manifest    PairedManifest `json:"manifest"`
	CorpusHash  string         `json:"corpus_hash"`
	CodeHash    string         `json:"code_hash"`
	// SkillHash and KapiHash are the hashes of the copies in inputs/, taken
	// from the checkout when the study started.
	SkillHash string `json:"skill_hash"`
	KapiHash  string `json:"kapi_hash"`
	// Build is the kapi build the copy was taken from, and the checkout's
	// commit at that moment: the commit to resume from.
	Build     pairedBuild `json:"build"`
	CreatedAt time.Time   `json:"created_at"`
}

// pairedInputs are the study's own copies of what the cells run, under
// <study>/inputs: the kapi binary and the shipped kapi skill. Each session
// takes them from there, so a rebuild or an edit in the checkout during the
// study changes nothing the study measures.
type pairedInputs struct {
	Kapi, Skill         string
	KapiHash, SkillHash string
	Build               pairedBuild
}

type pairedPreflight struct {
	Schema        int              `json:"schema"`
	Phase         string           `json:"phase"`
	Offline       bool             `json:"offline"`
	Build         pairedBuild      `json:"build"`
	PilotSessions int              `json:"pilot_sessions"`
	SmokeSessions int              `json:"smoke_sessions"`
	Prepared      []PairedPrepared `json:"prepared"`
	Blockers      []string         `json:"blockers"`
	HumanReview   string           `json:"human_review"`
}

type pairedAttempt struct {
	Schema      int            `json:"schema"`
	Session     PairedSession  `json:"session"`
	Phase       string         `json:"phase"`
	StartedAt   time.Time      `json:"started_at"`
	Fingerprint string         `json:"fingerprint"`
	Prepared    PairedPrepared `json:"prepared"`
}

type pairedAttemptResult struct {
	IdentityStatus string            `json:"identity_status"`
	FinishedAt     time.Time         `json:"finished_at"`
	Agent          PairedAgentResult `json:"agent"`
	Validation     *PairedValidation `json:"validation,omitempty"`
	ArtifactHash   string            `json:"artifact_hash"`
	// ArtifactScope says what ArtifactHash covers: "task" for the workspace
	// without its git metadata and kapi's runtime folders, which a reviewer's
	// git status or kapi command rewrites; empty for the whole workspace.
	ArtifactScope string `json:"artifact_scope,omitempty"`
	// TmpDir is where the cell's temporary directory was kept after the
	// session.
	TmpDir string `json:"tmp_dir,omitempty"`
	Error  string `json:"error,omitempty"`
}

func executePaired(ctx context.Context, opts PairedOptions) error {
	return executePairedWith(ctx, opts, pairedDependencies{
		prepare: preparePairedAgent, run: runPairedAgent, probe: probePairedSurface, build: checkPairedBuild,
	})
}

func executePairedWith(ctx context.Context, opts PairedOptions, deps pairedDependencies) error {
	switch opts.Phase {
	case "preflight", "diagnostic", "smoke", "pilot", "score":
	default:
		return fmt.Errorf("unknown paired phase %q", opts.Phase)
	}
	manifest, err := readPairedManifest(opts.ManifestPath)
	if err != nil {
		return err
	}
	if opts.Dir == "" {
		return errors.New("paired output directory is required")
	}
	opts.Dir, err = filepath.Abs(opts.Dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(opts.Dir, "runner.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("acquire study lock (inspect stale locks after an interrupted runner): %w", err)
	}
	defer func() { _ = lock.Close(); _ = os.Remove(lock.Name()) }()
	if _, err := fmt.Fprintf(lock, "pid=%d\n", os.Getpid()); err != nil {
		return err
	}
	if opts.Phase == "score" {
		return scorePaired(opts.Dir)
	}
	if opts.Phase == "preflight" || !opts.Live {
		opts.kapiBin = findKapi(opts.RepoRoot)
		opts.skillSource = filepath.Join(opts.RepoRoot, "cli", "skills", "data", "kapi")
		return preflightPaired(ctx, opts, manifest, deps)
	}
	if err := checkPairedLocation(opts.Dir); err != nil {
		return err
	}
	if opts.MaxAttempts < 1 {
		return errors.New("live execution requires a positive paired-max-attempts ceiling")
	}
	inputs, err := preparePairedInputs(ctx, opts, deps)
	if err != nil {
		return err
	}
	record, err := makePairedStudyRecord(opts, manifest, inputs)
	if err != nil {
		return err
	}
	if err := ensurePairedStudy(opts.Dir, record); err != nil {
		return err
	}
	opts.kapiBin, opts.skillSource = inputs.Kapi, inputs.Skill
	schedule := pairedSchedule(manifest, opts.Phase)
	phaseDir := filepath.Join(opts.Dir, opts.Phase)
	if err := os.MkdirAll(phaseDir, 0o700); err != nil {
		return err
	}
	if err := ensurePairedJSON(filepath.Join(phaseDir, "schedule.json"), schedule); err != nil {
		return err
	}
	selected, err := selectPairedSessions(schedule, opts.Sessions)
	if err != nil {
		return err
	}
	return runPairedSchedule(ctx, opts, record, selected, deps)
}

func preflightPaired(ctx context.Context, opts PairedOptions, m PairedManifest, deps pairedDependencies) error {
	report := pairedPreflight{Schema: pairedSchema, Phase: opts.Phase, Offline: true,
		PilotSessions: len(pairedSchedule(m, "pilot")), SmokeSessions: len(pairedSchedule(m, "smoke")),
		Prepared: []PairedPrepared{}, Blockers: []string{}, HumanReview: "unmeasured; independent review is required"}
	dir, err := os.MkdirTemp("", "kapi-paired-preflight-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	phase := "smoke"
	if opts.Phase == "diagnostic" {
		phase = "diagnostic"
	}
	schedule, err := selectPairedSessions(pairedSchedule(m, phase), opts.Sessions)
	if err != nil {
		return err
	}
	if deps.build != nil {
		build, err := deps.build(ctx, opts.RepoRoot)
		report.Build = build
		if err != nil {
			report.Blockers = append(report.Blockers, err.Error())
		}
	}
	// Preflight cells live in the system temporary directory; the live phases
	// put theirs in the study directory, so it is checked here too.
	if err := checkPairedLocation(opts.Dir); err != nil {
		report.Blockers = append(report.Blockers, err.Error())
	}
	for _, session := range schedule {
		if err := ctx.Err(); err != nil {
			return err
		}
		attemptDir := filepath.Join(dir, session.ID)
		launch, err := materializePairedLaunch(opts, m, session, attemptDir)
		if err != nil {
			return err
		}
		prepared, err := deps.prepare(ctx, launch)
		if err != nil {
			report.Blockers = append(report.Blockers, session.ID+": "+err.Error())
		}
		for _, blocker := range prepared.Blockers {
			report.Blockers = append(report.Blockers, session.ID+": "+blocker)
		}
		if err == nil && deps.probe != nil {
			surface := deps.probe(ctx, prepared)
			prepared.Surface = &surface
			for _, problem := range surface.Problems {
				report.Blockers = append(report.Blockers, session.ID+": surface: "+problem)
			}
		}
		discardPairedCellTmp(prepared)
		report.Prepared = append(report.Prepared, prepared)
	}
	path := filepath.Join(opts.Dir, "preflight-"+pairedTimestamp()+".json")
	if err := writePairedJSON(path, report); err != nil {
		return err
	}
	printPairedSurfaces(report.Prepared)
	fmt.Printf(
		"paired: offline preflight; %d prepared, %d smoke / %d pilot sessions, %d live blockers; %s\n",
		len(report.Prepared),
		report.SmokeSessions,
		report.PilotSessions,
		len(report.Blockers),
		path,
	)
	if len(report.Blockers) > 0 {
		for _, blocker := range report.Blockers {
			fmt.Println("  blocker: " + blocker)
		}
		return fmt.Errorf("preflight found %d blockers; no live phase can start", len(report.Blockers))
	}
	return nil
}

// printPairedSurfaces prints what each prepared cell exposes, one line per
// host and condition: the differential the isolation proof rests on.
func printPairedSurfaces(prepared []PairedPrepared) {
	fmt.Println("paired: surface per cell (kapi skills, MCP servers, kapi names on PATH, problems)")
	for _, p := range prepared {
		if p.Surface == nil {
			continue
		}
		s := p.Surface
		skills := []string{}
		for _, skill := range s.Skills {
			if skill == "kapi" || skill == pairedFilesAlias {
				skills = append(skills, skill)
			}
		}
		fmt.Printf("  %-6s %-12s skills=%v mcp=%v path=%v problems=%d (%d skills visible in all)\n",
			s.Host, s.Condition, skills, s.MCPServers, s.Executables, len(s.Problems), len(s.Skills))
		if len(s.WritableRoots) > 0 {
			fmt.Printf("  %-6s %-12s writable=%v\n", "", "", s.WritableRoots)
		}
	}
}

func materializePairedLaunch(opts PairedOptions, m PairedManifest, s PairedSession, dir string) (PairedLaunch, error) {
	task, err := findPairedTask(s.Task)
	if err != nil {
		return PairedLaunch{}, err
	}
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		return PairedLaunch{}, err
	}
	if err := materializePairedTask(workspace, task); err != nil {
		return PairedLaunch{}, err
	}
	prompt := task.Prompt
	if opts.Phase == "diagnostic" {
		prompt += "\n\n" + pairedDiagnosticInstruction(s.Condition, pairedDiagnosticFiles(task))
	}
	if err := writePairedExclusive(filepath.Join(dir, "prompt.txt"), []byte(prompt+"\n")); err != nil {
		return PairedLaunch{}, err
	}
	return PairedLaunch{
		Agent: s.Agent, Condition: s.Condition, Task: task.ID, Workspace: workspace,
		StateDir: filepath.Join(dir, "state"), RepoRoot: opts.RepoRoot, KapiBin: opts.kapiBin,
		SkillSource: opts.skillSource, StudyDir: opts.Dir,
		Prompt: prompt, TranscriptPath: filepath.Join(dir, "transcript.jsonl"),
		Timeout: m.attemptTimeout(), MaxTurns: m.MaxTurns, Interference: task.spec.Interference,
	}, nil
}

// readPairedContext puts the fixture's voice profile and vocabulary into the
// store the workspace's kapi answers from.
//
// The fixture ships them as files, and a file in a checkout governs nothing
// until somebody reads it in, so every arm of the study starts from the same
// context rather than from an empty one. It runs as the person setting the
// study up, because a context import is a person's decision.
//
// A study run without a built kapi has no store to fill and nothing to do.
func readPairedContext(ctx context.Context, workspace, kapiBin string) error {
	if kapiBin == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	//nolint:gosec // G702: kapiBin is this checkout's own build (findKapi) and the workspace a cell this run created.
	cmd := exec.CommandContext(ctx, kapiBin, "context", "import",
		"-p", filepath.Join(workspace, "kapi.yaml"))
	cmd.Dir = workspace
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + workspace,
		"KAPI_ACTOR=person",
	}, isolationEnv(workspace)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("read the fixture's context: %w\n%s", err, out)
	}
	return nil
}

func findPairedTask(id string) (PairedTask, error) {
	for _, task := range pairedTasks() {
		if task.ID == id {
			return task, nil
		}
	}
	return PairedTask{}, fmt.Errorf("unknown task %q", id)
}

func makePairedStudyRecord(opts PairedOptions, m PairedManifest, inputs pairedInputs) (pairedStudyRecord, error) {
	record := pairedStudyRecord{
		Schema: pairedSchema, Manifest: m, CreatedAt: time.Now().UTC(),
		SkillHash: inputs.SkillHash, KapiHash: inputs.KapiHash, Build: inputs.Build,
	}
	var err error
	record.CorpusHash, err = pairedCorpusHash()
	if err != nil {
		return record, err
	}
	record.CodeHash, err = pairedTreeHash(filepath.Join(opts.RepoRoot, "scripts", "skilleval"))
	if err != nil {
		return record, err
	}
	record.Fingerprint, err = pairedHash(struct {
		Manifest                          PairedManifest
		Corpus, Code, Skill, Kapi, Commit string
	}{Manifest: m, Corpus: record.CorpusHash, Code: record.CodeHash, Skill: record.SkillHash,
		Kapi: record.KapiHash, Commit: record.Build.Commit})
	return record, err
}

// ensurePairedStudy writes study.json for a new study and, for one already
// started, checks that this run measures the same thing.
func ensurePairedStudy(dir string, record pairedStudyRecord) error {
	path := filepath.Join(dir, "study.json")
	var existing pairedStudyRecord
	if err := readPairedJSON(path, &existing); err == nil {
		if existing.Fingerprint == record.Fingerprint {
			return nil
		}
		resume := "a checkout of the commit it started from"
		if existing.Build.Head != "" {
			resume = fmt.Sprintf("a checkout of %s (git worktree add <dir> %s)", existing.Build.Head, existing.Build.Head)
		}
		return fmt.Errorf("this study started with another manifest, corpus or runner (scripts/skilleval) than the "+
			"checkout holds now, so this run would measure something else. Resume it from %s, running make "+
			"paired-eval-pilot there with the same PAIRED_EVAL_DIR; no rebuild is needed, since the study runs its own "+
			"copy of kapi. Its started attempts stay where they are, and a new directory would run every session again",
			resume)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writePairedJSON(path, record)
}

// preparePairedInputs returns the study's own copies of kapi and its skill.
// A new study takes them from the checkout, whose bin/kapi must be this
// tree's build; a study already started keeps the copies it has, and every
// run checks them against study.json.
func preparePairedInputs(ctx context.Context, opts PairedOptions, deps pairedDependencies) (pairedInputs, error) {
	dir := filepath.Join(opts.Dir, "inputs")
	inputs := pairedInputs{Kapi: filepath.Join(dir, "kapi"), Skill: filepath.Join(dir, "skills", "kapi")}
	var existing pairedStudyRecord
	err := readPairedJSON(filepath.Join(opts.Dir, "study.json"), &existing)
	if err == nil {
		inputs.Build = existing.Build
		inputs.KapiHash, inputs.SkillHash = existing.KapiHash, existing.SkillHash
		return inputs, verifyPairedInputs(inputs)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return inputs, err
	}
	// No attempt starts before study.json exists, so copies left by a run
	// that stopped earlier are disposable.
	if err := os.RemoveAll(dir); err != nil {
		return inputs, err
	}
	if deps.build != nil {
		if inputs.Build, err = deps.build(ctx, opts.RepoRoot); err != nil {
			return inputs, err
		}
	}
	binary := findKapi(opts.RepoRoot)
	if binary == "" {
		return inputs, errors.New("bin/kapi is missing: the study runs the kapi built from this tree; run make build")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return inputs, err
	}
	if err := pairedCopyFile(binary, inputs.Kapi, 0o500); err != nil {
		return inputs, fmt.Errorf("copy kapi into the study: %w", err)
	}
	if err := copyTree(filepath.Join(opts.RepoRoot, "cli", "skills", "data", "kapi"), inputs.Skill); err != nil {
		return inputs, fmt.Errorf("copy the kapi skill into the study: %w", err)
	}
	if inputs.KapiHash, err = pairedFileHash(inputs.Kapi); err != nil {
		return inputs, err
	}
	inputs.SkillHash, err = pairedTreeHash(inputs.Skill)
	return inputs, err
}

// verifyPairedInputs checks the study's copies against the hashes study.json
// recorded when the study started.
func verifyPairedInputs(inputs pairedInputs) error {
	kapi, err := pairedFileHash(inputs.Kapi)
	if err != nil {
		return fmt.Errorf("the study's copy of kapi: %w", err)
	}
	skill, err := pairedTreeHash(inputs.Skill)
	if err != nil {
		return fmt.Errorf("the study's copy of the kapi skill: %w", err)
	}
	if kapi != inputs.KapiHash || skill != inputs.SkillHash {
		return fmt.Errorf("the study's copies of kapi and its skill under %s differ from the ones it started with; "+
			"it cannot resume, and its started attempts stay as evidence", filepath.Dir(inputs.Kapi))
	}
	return nil
}

// pairedCopyFile copies a regular file to a new path with the given mode.
func pairedCopyFile(source, destination string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(destination, mode)
}

const pairedMaxHashFileBytes int64 = 256 << 20

func pairedFileHash(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("artifact is not a regular file: %s", path)
	}
	if info.Size() > pairedMaxHashFileBytes {
		return "", fmt.Errorf("artifact exceeds hash size limit: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, pairedMaxHashFileBytes+1))
	if err != nil {
		return "", err
	}
	if size > pairedMaxHashFileBytes {
		return "", fmt.Errorf("artifact exceeds hash size limit: %s", path)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func pairedTreeHash(dir string) (string, error) {
	return pairedTreeHashExcept(dir, nil)
}

// pairedTreeHashExcept hashes every regular file under dir but those under
// the slash paths, relative to dir, that skip names.
func pairedTreeHashExcept(dir string, skip []string) (string, error) {
	entries := []string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if relative, relErr := filepath.Rel(dir, path); relErr == nil && slices.Contains(skip, filepath.ToSlash(relative)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("non-regular file in immutable input %s", path)
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		hash, err := pairedFileHash(path)
		if err != nil {
			return err
		}
		entries = append(entries, filepath.ToSlash(relative)+":"+hash)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(entries)
	return pairedHash(entries)
}

func ensurePairedJSON(path string, value any) error {
	expected, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err == nil {
		if strings.TrimSpace(string(existing)) != string(expected) {
			return fmt.Errorf("immutable record differs: %s", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writePairedJSON(path, value)
}

func writePairedJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writePairedExclusive(path, append(data, '\n'))
}

func writePairedExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}

func readPairedJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

func pairedTimestamp() string { return time.Now().UTC().Format("20060102T150405.000000000Z") }
