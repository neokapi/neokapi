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
	"path/filepath"
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
}

type pairedDependencies struct {
	prepare func(context.Context, PairedLaunch) (PairedPrepared, error)
	run     func(context.Context, PairedPrepared) (PairedAgentResult, error)
}

type pairedStudyRecord struct {
	Schema      int            `json:"schema"`
	Fingerprint string         `json:"fingerprint"`
	Manifest    PairedManifest `json:"manifest"`
	CorpusHash  string         `json:"corpus_hash"`
	CodeHash    string         `json:"code_hash"`
	SkillHash   string         `json:"skill_hash"`
	KapiHash    string         `json:"kapi_hash"`
	CreatedAt   time.Time      `json:"created_at"`
}

type pairedPreflight struct {
	Schema        int              `json:"schema"`
	Phase         string           `json:"phase"`
	Offline       bool             `json:"offline"`
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
	Error          string            `json:"error,omitempty"`
}

func executePaired(ctx context.Context, opts PairedOptions) error {
	return executePairedWith(ctx, opts, pairedDependencies{prepare: preparePairedAgent, run: runPairedAgent})
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
	record, err := makePairedStudyRecord(opts, manifest)
	if err != nil {
		return err
	}
	if opts.Phase == "preflight" || !opts.Live {
		return preflightPaired(ctx, opts, manifest, deps)
	}
	if opts.MaxAttempts < 1 {
		return errors.New("live execution requires a positive paired-max-attempts ceiling")
	}
	if err := ensurePairedStudy(opts.Dir, record); err != nil {
		return err
	}
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
		report.Prepared = append(report.Prepared, prepared)
	}
	path := filepath.Join(opts.Dir, "preflight-"+pairedTimestamp()+".json")
	if err := writePairedJSON(path, report); err != nil {
		return err
	}
	fmt.Printf(
		"paired: offline preflight; %d prepared, %d smoke / %d pilot sessions, %d live blockers; %s\n",
		len(report.Prepared),
		report.SmokeSessions,
		report.PilotSessions,
		len(report.Blockers),
		path,
	)
	return nil
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
		prompt += "\n\n" + pairedDiagnosticInstruction(s.Condition)
	}
	if err := writePairedExclusive(filepath.Join(dir, "prompt.txt"), []byte(prompt+"\n")); err != nil {
		return PairedLaunch{}, err
	}
	return PairedLaunch{
		Agent: s.Agent, Condition: s.Condition, Workspace: workspace,
		StateDir: filepath.Join(dir, "state"), RepoRoot: opts.RepoRoot, KapiBin: findKapi(opts.RepoRoot),
		Prompt: prompt, TranscriptPath: filepath.Join(dir, "transcript.jsonl"),
		Timeout: m.attemptTimeout(), MaxTurns: m.MaxTurns,
	}, nil
}

func findPairedTask(id string) (PairedTask, error) {
	for _, task := range pairedTasks() {
		if task.ID == id {
			return task, nil
		}
	}
	return PairedTask{}, fmt.Errorf("unknown task %q", id)
}

func makePairedStudyRecord(opts PairedOptions, m PairedManifest) (pairedStudyRecord, error) {
	record := pairedStudyRecord{Schema: pairedSchema, Manifest: m, CreatedAt: time.Now().UTC()}
	var err error
	record.CorpusHash, err = pairedCorpusHash()
	if err != nil {
		return record, err
	}
	record.CodeHash, err = pairedTreeHash(filepath.Join(opts.RepoRoot, "scripts", "skilleval"))
	if err != nil {
		return record, err
	}
	record.SkillHash, err = pairedTreeHash(filepath.Join(opts.RepoRoot, "cli", "skills", "data", "kapi"))
	if err != nil {
		return record, err
	}
	record.KapiHash = "unavailable"
	if binary := findKapi(opts.RepoRoot); binary != "" {
		resolved, resolveErr := filepath.EvalSymlinks(binary)
		if resolveErr != nil {
			return record, resolveErr
		}
		record.KapiHash, err = pairedFileHash(resolved)
		if err != nil {
			return record, err
		}
	}
	record.Fingerprint, err = pairedHash(struct {
		Manifest                  PairedManifest
		Corpus, Code, Skill, Kapi string
	}{Manifest: m, Corpus: record.CorpusHash, Code: record.CodeHash, Skill: record.SkillHash, Kapi: record.KapiHash})
	return record, err
}

func ensurePairedStudy(dir string, record pairedStudyRecord) error {
	path := filepath.Join(dir, "study.json")
	var existing pairedStudyRecord
	if err := readPairedJSON(path, &existing); err == nil {
		if existing.Fingerprint != record.Fingerprint {
			return errors.New("study inputs changed; choose a fresh paired-dir to retain the existing evidence")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writePairedJSON(path, record)
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
	entries := []string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
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
