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

func meaningLaunch(opts MeaningOptions, study meaningStudy, session meaningSession, dir, prompt string) (PairedLaunch, error) {
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		return PairedLaunch{}, err
	}
	if err := writePairedExclusive(filepath.Join(dir, "prompt.txt"), []byte(prompt)); err != nil {
		return PairedLaunch{}, err
	}
	var agent PairedAgentSpec
	for _, item := range study.Manifest.Agents {
		if item.Host == session.Host {
			agent = item
		}
	}
	return PairedLaunch{
		Agent: agent, Condition: "baseline", Workspace: workspace, StateDir: filepath.Join(dir, "state"),
		RepoRoot: opts.RepoRoot, Prompt: prompt, TranscriptPath: filepath.Join(dir, "transcript.jsonl"),
		Timeout: time.Duration(study.Manifest.AttemptTimeoutSeconds) * time.Second, MaxTurns: study.Manifest.MaxTurns, NoTools: true,
	}, nil
}

func preflightMeaning(ctx context.Context, opts MeaningOptions, study meaningStudy, prompts map[string]string, deps pairedDependencies) error {
	dir, err := os.MkdirTemp("", "kapi-meaning-preflight-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	report := struct {
		Offline  bool             `json:"offline"`
		Prepared []PairedPrepared `json:"prepared"`
		Blockers []string         `json:"blockers"`
	}{Offline: true, Prepared: []PairedPrepared{}, Blockers: []string{}}
	for _, session := range study.Manifest.Sessions {
		if err := ctx.Err(); err != nil {
			return err
		}
		launch, err := meaningLaunch(opts, study, session, filepath.Join(dir, session.ID), prompts[session.ID])
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
	fmt.Printf("meaning: offline preflight; %d sessions, %d blockers; %s\n", len(report.Prepared), len(report.Blockers), path)
	return nil
}

func runMeaning(ctx context.Context, opts MeaningOptions, study meaningStudy, inputs map[string]meaningInput, prompts map[string]string, deps pairedDependencies) error {
	stopPath := filepath.Join(opts.Dir, "rate-limit-stop.json")
	if _, err := os.Stat(stopPath); err == nil {
		return errors.New("meaning study stopped after rate limiting; review quota before starting any new study")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	attemptsDir := filepath.Join(opts.Dir, "attempts")
	if err := os.MkdirAll(attemptsDir, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(attemptsDir)
	if err != nil {
		return err
	}
	// Every reserved attempt directory counts, even a crash before launch. No retry
	// can erase an uncertain start or silently obtain another subscription call.
	started := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			return errors.New("unexpected file in meaning attempt ledger")
		}
		started[entry.Name()] = true
		var result meaningResult
		resultErr := readPairedJSON(filepath.Join(attemptsDir, entry.Name(), "result.json"), &result)
		if resultErr == nil && result.Agent.RateLimited {
			return errors.New("meaning study contains a rate-limited attempt; no further launches")
		}
		if resultErr != nil && !errors.Is(resultErr, os.ErrNotExist) {
			return resultErr
		}
	}
	for _, session := range study.Manifest.Sessions {
		if started[session.ID] {
			continue
		}
		if len(started) >= opts.MaxAttempts {
			fmt.Printf("meaning: persistent attempt ceiling reached (%d); review before another batch\n", opts.MaxAttempts)
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		dir := filepath.Join(attemptsDir, session.ID)
		if err := os.Mkdir(dir, 0o700); err != nil {
			return err
		}
		started[session.ID] = true
		launch, err := meaningLaunch(opts, study, session, dir, prompts[session.ID])
		if err != nil {
			return err
		}
		prepared, err := deps.prepare(ctx, launch)
		if err != nil {
			return err
		}
		if len(prepared.Blockers) > 0 {
			return fmt.Errorf("meaning preflight blocked: %s", strings.Join(prepared.Blockers, "; "))
		}
		attempt := meaningAttempt{Schema: 1, Fingerprint: study.Fingerprint, Session: session, StartedAt: time.Now().UTC(), Prepared: prepared}
		if err := writePairedJSON(filepath.Join(dir, "started.json"), attempt); err != nil {
			return err
		}
		result, runErr := deps.run(ctx, prepared)
		integrity := validateMeaningReview(result.FinalText, inputs[session.CaseID])
		if len(result.Tools) > 0 {
			result.Status = "tool_use_violation"
			integrity.Errors = append(integrity.Errors, "tool use violates fixed-input review protocol")
			integrity.Valid = false
		}
		if result.Status != "completed" {
			integrity.Valid = false
			integrity.Errors = append(integrity.Errors, "review protocol did not complete with verified model identity")
		}
		record := meaningResult{Agent: result, Integrity: integrity, FinishedAt: time.Now().UTC()}
		if runErr != nil {
			record.Error = runErr.Error()
		}
		if err := writePairedJSON(filepath.Join(dir, "result.json"), record); err != nil {
			return err
		}
		fmt.Printf("meaning: %s: %s; output integrity=%t (semantic correctness unmeasured)\n", session.ID, result.Status, integrity.Valid)
		if result.RateLimited {
			if err := writePairedJSON(stopPath, map[string]string{"session": session.ID, "reason": "rate limited; no automatic retry"}); err != nil {
				return err
			}
			return errors.New("meaning study paused after rate limit")
		}
	}
	return nil
}

func validateMeaningReview(text string, input meaningInput) meaningIntegrity {
	result := meaningIntegrity{Errors: []string{}, Scope: "JSON shape, source references and literal quotes only; semantic correctness unmeasured"}
	var review meaningReview
	if err := decodeMeaningJSON([]byte(text), &review); err != nil {
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	result.Review = &review
	if review.Findings == nil || review.Abstentions == nil {
		result.Errors = append(result.Errors, "findings and abstentions must be arrays")
	}
	sources := map[string]bool{}
	for _, source := range input.Sources {
		sources[source.ID] = true
	}
	checkEvidence := func(quote string, ids []string, label string) {
		if quote != "" && !strings.Contains(input.Candidate, quote) {
			result.Errors = append(result.Errors, label+": candidate quote is not an exact substring")
		}
		for _, id := range ids {
			if !sources[id] {
				result.Errors = append(result.Errors, label+": unknown source ID "+id)
			}
		}
	}
	for i, finding := range review.Findings {
		label := fmt.Sprintf("finding %d", i+1)
		checkEvidence(finding.CandidateQuote, finding.SourceIDs, label)
		if finding.Kind != "conflict" && finding.Kind != "omission" {
			result.Errors = append(result.Errors, label+": kind must be conflict or omission")
		}
		if finding.Kind == "conflict" && strings.TrimSpace(finding.CandidateQuote) == "" {
			result.Errors = append(result.Errors, label+": conflict requires a candidate quote")
		}
		if len(finding.SourceIDs) == 0 || strings.TrimSpace(finding.Rationale) == "" {
			result.Errors = append(result.Errors, label+": source IDs and rationale are required")
		}
	}
	for i, abstention := range review.Abstentions {
		label := fmt.Sprintf("abstention %d", i+1)
		checkEvidence(abstention.CandidateQuote, abstention.SourceIDs, label)
		if abstention.SourceIDs == nil || strings.TrimSpace(abstention.MissingEvidence) == "" {
			result.Errors = append(result.Errors, label+": source_ids array and missing_evidence are required")
		}
	}
	result.Valid = len(result.Errors) == 0
	return result
}
