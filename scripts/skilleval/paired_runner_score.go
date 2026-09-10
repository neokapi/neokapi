package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type pairedScoreRow struct {
	IdentityStatus    string            `json:"identity_status"`
	EvidencePath      string            `json:"evidence_path"`
	Session           PairedSession     `json:"session"`
	Phase             string            `json:"phase"`
	Status            string            `json:"status"`
	Error             string            `json:"error,omitempty"`
	ObjectivePassed   bool              `json:"objective_passed"`
	Validation        *PairedValidation `json:"validation,omitempty"`
	ArtifactIntegrity string            `json:"artifact_integrity"`
	DurationMS        int64             `json:"duration_ms"`
	InputTokens       *int64            `json:"input_tokens"`
	OutputTokens      *int64            `json:"output_tokens"`
	HumanReview       string            `json:"human_review"`
	ReviewerMinutes   *float64          `json:"reviewer_minutes"`
	DollarCost        *float64          `json:"dollar_cost"`
}

type pairedScore struct {
	Schema         int              `json:"schema"`
	CreatedAt      time.Time        `json:"created_at"`
	Study          string           `json:"study"`
	Fingerprint    string           `json:"fingerprint"`
	Attempts       int              `json:"attempts"`
	Rows           []pairedScoreRow `json:"rows"`
	Interpretation string           `json:"interpretation"`
}

func scorePaired(dir string) error {
	var record pairedStudyRecord
	if err := readPairedJSON(filepath.Join(dir, "study.json"), &record); err != nil {
		return err
	}
	paths, err := pairedAttemptPaths(dir)
	if err != nil {
		return err
	}
	report := pairedScore{
		Schema: pairedSchema, CreatedAt: time.Now().UTC(), Study: record.Manifest.Study,
		Fingerprint: record.Fingerprint, Attempts: len(paths), Rows: []pairedScoreRow{},
		Interpretation: "Automatic artifact criteria only. Content quality, accepted results and reviewer time " +
			"require independent human review. Subscription quota and dollar cost are unmeasured.",
	}
	for _, path := range paths {
		row, err := scorePairedAttempt(path, record.Fingerprint)
		if err != nil {
			return err
		}
		report.Rows = append(report.Rows, row)
	}
	stem := filepath.Join(dir, "score-"+pairedTimestamp())
	if err := writePairedJSON(stem+".json", report); err != nil {
		return err
	}
	var markdown strings.Builder
	fmt.Fprintf(
		&markdown,
		"# %s\n\n%s\n\nAttempts reserved: %d. Failures and interrupted attempts are retained.\n\n",
		report.Study,
		report.Interpretation,
		report.Attempts,
	)
	markdown.WriteString("| Phase | Task | Model | Integration | Status | Artifact criteria | Identity / integrity | Seconds | Evidence | Human review |\n" +
		"|---|---|---|---|---|---|---|---|---|---|\n")
	for _, row := range report.Rows {
		criteria := "not passed"
		if row.ObjectivePassed {
			criteria = "passed"
		}
		fmt.Fprintf(
			&markdown,
			"| %s | %s | %s | %s | %s | %s | %s / %s | %.1f | %s | %s |\n",
			row.Phase,
			row.Session.Task,
			row.Session.Agent.Model,
			row.Session.Condition,
			row.Status,
			criteria,
			row.IdentityStatus,
			row.ArtifactIntegrity,
			float64(row.DurationMS)/1000,
			pairedEvidenceLinks(row),
			row.HumanReview,
		)
	}
	if err := writePairedExclusive(stem+".md", []byte(markdown.String())); err != nil {
		return err
	}
	fmt.Printf("paired: %d retained attempts; %s.md\n", report.Attempts, stem)
	return nil
}

func scorePairedAttempt(path, fingerprint string) (pairedScoreRow, error) {
	var attempt pairedAttempt
	if err := readPairedJSON(path, &attempt); err != nil {
		return pairedScoreRow{}, err
	}
	if attempt.Fingerprint != fingerprint {
		return pairedScoreRow{}, fmt.Errorf("attempt fingerprint differs: %s", path)
	}
	row := pairedScoreRow{
		Session: attempt.Session, Phase: attempt.Phase, Status: "interrupted",
		ArtifactIntegrity: "unrecorded", HumanReview: "pending",
	}
	var result pairedAttemptResult
	err := readPairedJSON(filepath.Join(filepath.Dir(path), "result.json"), &result)
	if errors.Is(err, os.ErrNotExist) {
		row.Error = "attempt was reserved but no result was committed"
		return row, nil
	}
	if err != nil {
		return row, err
	}
	row.IdentityStatus = result.IdentityStatus
	row.EvidencePath = filepath.ToSlash(filepath.Join(attempt.Phase, attempt.Session.ID))
	row.Status, row.Error = result.Agent.Status, result.Error
	row.DurationMS = result.Agent.DurationMS
	if result.Agent.UsageObserved {
		row.InputTokens = &result.Agent.InputTokens
		row.OutputTokens = &result.Agent.OutputTokens
	}
	row.Validation = result.Validation
	hash, err := pairedTreeHash(filepath.Join(filepath.Dir(path), "workspace"))
	if err != nil {
		row.ArtifactIntegrity = "unreadable"
		row.Error = err.Error()
		return row, nil
	}
	row.ArtifactIntegrity = "verified"
	if hash != result.ArtifactHash {
		row.ArtifactIntegrity = "changed"
		return row, nil
	}
	// Preserve original criteria and identify edited artifacts. Re-scoring with a
	// different evaluator never silently replaces the frozen attempt's validation.
	if result.Validation != nil {
		row.ObjectivePassed = result.Validation.ObjectivePassed
	}
	return row, nil
}

func pairedEvidenceLinks(row pairedScoreRow) string {
	path := filepath.ToSlash(filepath.Join(row.Phase, row.Session.ID))
	return "[result](" + path + "/result.json) · [files](" + path + "/workspace/) · [transcript](" + path + "/transcript.jsonl)"
}
