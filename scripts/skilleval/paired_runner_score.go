package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

type pairedScoreRow struct {
	IdentityStatus    string                    `json:"identity_status"`
	EvidencePath      string                    `json:"evidence_path"`
	Session           PairedSession             `json:"session"`
	Phase             string                    `json:"phase"`
	Superseded        bool                      `json:"superseded,omitempty"`
	Status            string                    `json:"status"`
	Error             string                    `json:"error,omitempty"`
	ObjectivePassed   bool                      `json:"objective_passed"`
	Validation        *PairedValidation         `json:"validation,omitempty"`
	ArtifactIntegrity string                    `json:"artifact_integrity"`
	DurationMS        int64                     `json:"duration_ms"`
	InputTokens       *int64                    `json:"input_tokens"`
	CacheReadTokens   *int64                    `json:"cache_read_tokens"`
	CacheWriteTokens  *int64                    `json:"cache_write_tokens"`
	OutputTokens      *int64                    `json:"output_tokens"`
	Turns             *int64                    `json:"turns,omitempty"`
	ToolCalls         int                       `json:"tool_calls"`
	Refusals          map[string]int            `json:"refusals,omitempty"`
	OverrideAttempts  []string                  `json:"override_attempts,omitempty"`
	RouteAttempts     []string                  `json:"route_attempts,omitempty"`
	Interference      *PairedInterferenceRecord `json:"interference,omitempty"`
	HumanReview       string                    `json:"human_review"`
	ReviewerMinutes   *float64                  `json:"reviewer_minutes"`
	DollarCost        *float64                  `json:"dollar_cost"`
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
			"require independent human review. Subscription quota and dollar cost are unmeasured. " +
			"Diagnostic rows use explicit integration instructions and must be interpreted separately from natural tasks. " +
			"Superseded rows were run again under -paired-retry and are left out of the summary.",
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
	writePairedSummary(&markdown, report.Rows)
	markdown.WriteString("\n## Attempts\n\n| Phase | Task | Model | Integration | Status | Artifact criteria | Identity / integrity | Seconds | Tools | Refusals | Evidence | Human review |\n" +
		"|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, row := range report.Rows {
		criteria := "not passed"
		if row.ObjectivePassed {
			criteria = "passed"
		}
		status := row.Status
		if row.Superseded {
			status += " (superseded)"
		}
		fmt.Fprintf(
			&markdown,
			"| %s | %s | %s | %s | %s | %s | %s / %s | %.1f | %d | %s | %s | %s |\n",
			row.Phase,
			row.Session.Task,
			row.Session.Agent.Model,
			row.Session.Condition,
			status,
			criteria,
			row.IdentityStatus,
			row.ArtifactIntegrity,
			float64(row.DurationMS)/1000,
			row.ToolCalls,
			pairedRefusalText(row.Refusals),
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

// writePairedSummary groups the current attempts of the natural phases by
// task, host and condition.
func writePairedSummary(markdown *strings.Builder, rows []pairedScoreRow) {
	type cell struct {
		n, passed, completed, overrides int
		seconds, input, output, tools   []float64
		refusals                        map[string]int
	}
	cells := map[[3]string]*cell{}
	for _, row := range rows {
		if row.Superseded || row.Phase == "diagnostic" {
			continue
		}
		key := [3]string{row.Session.Task, row.Session.Agent.Host, row.Session.Condition}
		c := cells[key]
		if c == nil {
			c = &cell{refusals: map[string]int{}}
			cells[key] = c
		}
		c.n++
		if row.ObjectivePassed {
			c.passed++
		}
		if row.Status == "completed" {
			c.completed++
		}
		if len(row.OverrideAttempts) > 0 {
			c.overrides++
		}
		c.seconds = append(c.seconds, float64(row.DurationMS)/1000)
		c.tools = append(c.tools, float64(row.ToolCalls))
		if row.InputTokens != nil && row.OutputTokens != nil {
			c.input = append(c.input, float64(*row.InputTokens))
			c.output = append(c.output, float64(*row.OutputTokens))
		}
		for code, count := range row.Refusals {
			c.refusals[code] += count
		}
	}
	if len(cells) == 0 {
		return
	}
	keys := make([][3]string, 0, len(cells))
	for key := range cells {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		if keys[i][1] != keys[j][1] {
			return keys[i][1] < keys[j][1]
		}
		return slices.Index(pairedConditions, keys[i][2]) < slices.Index(pairedConditions, keys[j][2])
	})
	markdown.WriteString("## Summary\n\nMedians over each cell's current attempts.\n\n" +
		"| Task | Host | Condition | n | Objective passed | Completed | Seconds | Input tokens | Output tokens | Tool calls | Refusals | Override attempts |\n" +
		"|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, key := range keys {
		c := cells[key]
		fmt.Fprintf(markdown, "| %s | %s | %s | %d | %d | %d | %.0f | %.0f | %.0f | %.0f | %s | %d |\n",
			key[0], key[1], key[2], c.n, c.passed, c.completed,
			pairedMedian(c.seconds), pairedMedian(c.input), pairedMedian(c.output), pairedMedian(c.tools),
			pairedRefusalText(c.refusals), c.overrides)
	}
}

func pairedMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func pairedRefusalText(refusals map[string]int) string {
	if len(refusals) == 0 {
		return ""
	}
	codes := make([]string, 0, len(refusals))
	for code := range refusals {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	parts := make([]string, len(codes))
	for i, code := range codes {
		parts[i] = fmt.Sprintf("%s %d", code, refusals[code])
	}
	return strings.Join(parts, ", ")
}

func scorePairedAttempt(path, fingerprint string) (pairedScoreRow, error) {
	var attempt pairedAttempt
	if err := readPairedJSON(path, &attempt); err != nil {
		return pairedScoreRow{}, err
	}
	if attempt.Fingerprint != fingerprint {
		return pairedScoreRow{}, fmt.Errorf("attempt fingerprint differs: %s", path)
	}
	attemptDir := filepath.Dir(path)
	row := pairedScoreRow{
		Session: attempt.Session, Phase: attempt.Phase, Status: "interrupted",
		Superseded:        strings.Contains(filepath.Base(attemptDir), ".retired-"),
		ArtifactIntegrity: "unrecorded", HumanReview: "pending",
		EvidencePath: filepath.ToSlash(filepath.Join(attempt.Phase, filepath.Base(attemptDir))),
	}
	var result pairedAttemptResult
	err := readPairedJSON(filepath.Join(attemptDir, "result.json"), &result)
	if errors.Is(err, os.ErrNotExist) {
		row.Error = "attempt was reserved but no result was committed"
		return row, nil
	}
	if err != nil {
		return row, err
	}
	row.IdentityStatus = result.IdentityStatus
	row.Status, row.Error = result.Agent.Status, result.Error
	row.DurationMS = result.Agent.DurationMS
	if result.Agent.UsageObserved {
		row.InputTokens = &result.Agent.InputTokens
		row.CacheReadTokens = &result.Agent.CacheReadTokens
		row.CacheWriteTokens = &result.Agent.CacheWriteTokens
		row.OutputTokens = &result.Agent.OutputTokens
	}
	row.Turns = result.Agent.Turns
	row.ToolCalls = result.Agent.ToolCalls
	row.Refusals = result.Agent.Refusals
	row.OverrideAttempts = result.Agent.OverrideAttempts
	row.RouteAttempts = result.Agent.RouteAttempts
	row.Interference = result.Agent.Interference
	row.Validation = result.Validation
	hash, err := pairedTreeHash(filepath.Join(attemptDir, "workspace"))
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
	path := row.EvidencePath
	return "[result](" + path + "/result.json) · [files](" + path + "/workspace/) · [transcript](" + path + "/transcript.jsonl)"
}
