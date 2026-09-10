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

const pocAttemptLimit = 6

type pocOptions struct {
	StagePath string
	Dir       string
	RepoRoot  string
	Live      bool
}

// A stage is one authoring call. InputFiles explicitly names its supplied files;
// dependencies or other runtime files are outside this evidence inventory.
type pocStage struct {
	ID              string   `json:"id"`
	Workspace       string   `json:"workspace"`
	PromptFile      string   `json:"prompt_file"`
	Condition       string   `json:"condition"`
	KapiBin         string   `json:"kapi_bin,omitempty"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	MaxTurns        int      `json:"max_turns"`
	InputFiles      []string `json:"input_files"`
	ExpectedOutputs []string `json:"expected_outputs"`
}

type pocFrozen struct {
	Schema      int               `json:"schema"`
	Stage       pocStage          `json:"stage"`
	PromptHash  string            `json:"prompt_sha256"`
	InputHashes map[string]string `json:"input_sha256"`
	CodeHash    string            `json:"runner_sha256"`
	SkillHash   string            `json:"skill_sha256,omitempty"`
	BinaryHash  string            `json:"kapi_sha256,omitempty"`
	Fingerprint string            `json:"fingerprint"`
}

type pocStarted struct {
	Fingerprint string         `json:"fingerprint"`
	StartedAt   time.Time      `json:"started_at"`
	Prepared    PairedPrepared `json:"prepared"`
}

type pocArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
	Bytes  int    `json:"bytes,omitempty"`
	Error  string `json:"error,omitempty"`
}

type pocResult struct {
	Fingerprint string            `json:"fingerprint"`
	Agent       PairedAgentResult `json:"agent"`
	Outputs     []pocArtifact     `json:"outputs"`
	FinishedAt  time.Time         `json:"finished_at"`
	Error       string            `json:"error,omitempty"`
}

func executePOC(ctx context.Context, opts pocOptions) error {
	return executePOCWith(ctx, opts, pairedDependencies{prepare: preparePairedAgent, run: runPairedAgent})
}

func executePOCWith(ctx context.Context, opts pocOptions, deps pairedDependencies) error {
	if opts.Dir == "" {
		return errors.New("POC ledger directory is required")
	}
	stage, prompt, err := loadPOCStage(opts.StagePath)
	if err != nil {
		return err
	}
	opts.Dir, err = filepath.Abs(opts.Dir)
	if err != nil {
		return err
	}
	if err := pocSeparateLedger(stage.Workspace, opts.Dir); err != nil {
		return err
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(opts.Dir, "runner.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("acquire POC ledger lock; inspect interrupted runs before removing a stale lock: %w", err)
	}
	defer func() { _ = lock.Close(); _ = os.Remove(lock.Name()) }()
	if err := ensurePairedJSON(filepath.Join(opts.Dir, "ledger.json"), struct {
		Schema      int             `json:"schema"`
		Billing     string          `json:"billing"`
		MaxAttempts int             `json:"max_attempts"`
		Agent       PairedAgentSpec `json:"agent"`
	}{Schema: 1, Billing: "subscription-only", MaxAttempts: pocAttemptLimit, Agent: pocAgent()}); err != nil {
		return err
	}
	dir := filepath.Join(opts.Dir, "stages", stage.ID)
	if err := pocSeparateLedger(stage.Workspace, dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if resumed, err := pocAlreadyStarted(dir, stage, prompt); resumed || err != nil {
		return err
	}
	frozen, err := freezePOCInputs(opts, stage, prompt, dir)
	if err != nil {
		return err
	}
	if !opts.Live {
		return preflightPOC(ctx, opts, frozen, prompt, dir, deps)
	}
	count, rateLimited, err := pocStartedCount(filepath.Join(opts.Dir, "stages"))
	if err != nil {
		return err
	}
	if rateLimited {
		return errors.New("POC ledger contains a rate-limited attempt; no automatic continuation")
	}
	if count >= pocAttemptLimit {
		return fmt.Errorf("POC persistent attempt limit reached (%d); review before further inference", pocAttemptLimit)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	prepared, err := deps.prepare(ctx, pocLaunch(opts, frozen.Stage, prompt, dir))
	if err != nil {
		return err
	}
	if len(prepared.Blockers) > 0 {
		return fmt.Errorf("POC preflight blocked: %s", strings.Join(prepared.Blockers, "; "))
	}
	// Preparation may install the shipped skill. No declared source input may
	// change between freezing and inference, including an explicitly listed skill.
	if err := verifyPOCInputs(frozen); err != nil {
		return err
	}
	started := pocStarted{Fingerprint: frozen.Fingerprint, StartedAt: time.Now().UTC(), Prepared: prepared}
	if err := writePairedJSON(filepath.Join(dir, "started.json"), started); err != nil {
		return err
	}
	// The exclusive, synced reservation precedes the only inference call.
	agent, runErr := deps.run(ctx, prepared)
	outputs := capturePOCOutputs(stage, dir)
	record := pocResult{
		Fingerprint: frozen.Fingerprint, Agent: agent, Outputs: outputs, FinishedAt: time.Now().UTC(),
	}
	if runErr != nil {
		record.Error = runErr.Error()
	}
	if err := writePairedJSON(filepath.Join(dir, "result.json"), record); err != nil {
		return err
	}
	issues := 0
	for _, output := range outputs {
		if output.Error != "" {
			issues++
		}
	}
	fmt.Printf("poc: %s: host=%s; output issues=%d; %d/%d attempts reserved; semantic quality unmeasured\n",
		stage.ID, agent.Status, issues, count+1, pocAttemptLimit)
	if agent.RateLimited {
		return errors.New("POC stopped after rate limiting; no automatic retry")
	}
	return runErr
}

func pocAgent() PairedAgentSpec {
	return PairedAgentSpec{Host: "claude", Model: "claude-sonnet-5", Effort: "high"}
}

func pocLaunch(opts pocOptions, stage pocStage, prompt, dir string) PairedLaunch {
	condition := "baseline"
	if stage.Condition == "skill" {
		condition = "skill-cli"
	}
	return PairedLaunch{
		Agent: pocAgent(), Condition: condition, Workspace: stage.Workspace,
		StateDir: filepath.Join(dir, "state"), RepoRoot: opts.RepoRoot, KapiBin: stage.KapiBin,
		Prompt: prompt, TranscriptPath: filepath.Join(dir, "transcript.jsonl"),
		Timeout: time.Duration(stage.TimeoutSeconds) * time.Second, MaxTurns: stage.MaxTurns,
	}
}

func preflightPOC(ctx context.Context, opts pocOptions, frozen pocFrozen, prompt, dir string, deps pairedDependencies) error {
	state, err := os.MkdirTemp(opts.Dir, "preflight-state-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(state)
	prepared, prepareErr := deps.prepare(ctx, pocLaunch(opts, frozen.Stage, prompt, state))
	report := struct {
		Offline     bool           `json:"offline"`
		Fingerprint string         `json:"fingerprint"`
		Prepared    PairedPrepared `json:"prepared"`
		Error       string         `json:"error,omitempty"`
	}{Offline: true, Fingerprint: frozen.Fingerprint, Prepared: prepared}
	if prepareErr != nil {
		report.Error = prepareErr.Error()
	}
	path := filepath.Join(dir, "preflight-"+pairedTimestamp()+".json")
	if err := writePairedJSON(path, report); err != nil {
		return err
	}
	fmt.Printf("poc: %s offline preflight; %d blockers; %s\n", frozen.Stage.ID, len(prepared.Blockers), path)
	return prepareErr
}

func pocAlreadyStarted(dir string, stage pocStage, prompt string) (bool, error) {
	if _, err := os.Stat(filepath.Join(dir, "started.json")); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var frozen pocFrozen
	if err := readPairedJSON(filepath.Join(dir, "frozen.json"), &frozen); err != nil {
		return true, err
	}
	want, err := pairedHash(stage)
	if err != nil {
		return true, err
	}
	actual, err := pairedHash(frozen.Stage)
	if err != nil {
		return true, err
	}
	if want != actual || meaningTextHash(prompt) != frozen.PromptHash {
		return true, errors.New("started POC stage manifest or prompt differs; retained attempts cannot be rewritten")
	}
	fmt.Printf("poc: %s already reserved; retained completed, failed or interrupted attempt will not be retried\n", stage.ID)
	return true, nil
}

func pocStartedCount(stagesDir string) (int, bool, error) {
	entries, err := os.ReadDir(stagesDir)
	if err != nil {
		return 0, false, err
	}
	count := 0
	rateLimited := false
	for _, entry := range entries {
		if !entry.IsDir() {
			return count, rateLimited, errors.New("unexpected file in POC stage ledger")
		}
		dir := filepath.Join(stagesDir, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, "started.json")); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return count, rateLimited, err
		}
		count++
		var result pocResult
		if err := readPairedJSON(filepath.Join(dir, "result.json"), &result); err == nil {
			rateLimited = rateLimited || result.Agent.RateLimited
		} else if !errors.Is(err, os.ErrNotExist) {
			return count, rateLimited, err
		}
	}
	return count, rateLimited, nil
}
