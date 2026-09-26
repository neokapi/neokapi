package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The report, rendered from saved attempts with no model call and no kapi.
//
// Every score is recomputed here from what each attempt saved, so a change to
// the scoring rules reaches runs already made. Measure 3 needs no saved
// attempt: it is measured afresh each time a report is built.

// EvalApplyRow is one Measure 1 run.
type EvalApplyRow struct {
	Session    string         `json:"session"`
	Host       string         `json:"host"`
	Task       string         `json:"task"`
	Status     string         `json:"status"`
	Error      string         `json:"error,omitempty"`
	KapiTools  []string       `json:"kapi_tools"`
	Isolated   bool           `json:"isolated"`
	HeldBefore int            `json:"held_before"`
	HeldAfter  int            `json:"held_after"`
	Score      EvalApplyScore `json:"score"`
}

// EvalGrowRow is one Measure 2 run, or a smoke run.
type EvalGrowRow struct {
	Session   string        `json:"session"`
	Phase     string        `json:"phase"`
	Host      string        `json:"host"`
	Task      string        `json:"task"`
	Status    string        `json:"status"`
	Error     string        `json:"error,omitempty"`
	KapiTools []string      `json:"kapi_tools"`
	Isolated  bool          `json:"isolated"`
	Score     EvalGrowScore `json:"score"`
}

// EvalApplyHost is one host's Measure 1 result against the bar.
type EvalApplyHost struct {
	Host              string  `json:"host"`
	Runs              int     `json:"runs"`
	Completed         int     `json:"completed"`
	Applicable        int     `json:"applicable"`
	Followed          int     `json:"followed"`
	Share             float64 `json:"share"`
	FailingFinal      int     `json:"failing_final"`
	CheckedBeforeDone int     `json:"checked_before_done"`
	Verdict           string  `json:"verdict"`
}

// EvalGrowHost is one host's Measure 2 result against the bar.
type EvalGrowHost struct {
	Host          string  `json:"host"`
	Runs          int     `json:"runs"`
	Completed     int     `json:"completed"`
	MeanRecall    float64 `json:"mean_recall"`
	Records       int     `json:"records"`
	Precise       int     `json:"precise"`
	Precision     float64 `json:"precision"`
	DecoyProposed bool    `json:"decoy_proposed"`
	Verdict       string  `json:"verdict"`
}

// EvalReport summarizes the evaluation's saved evidence.
type EvalReport struct {
	Schema      int               `json:"schema"`
	CreatedAt   time.Time         `json:"created_at"`
	Study       string            `json:"study"`
	Fingerprint string            `json:"fingerprint"`
	KapiVersion string            `json:"kapi_version"`
	KapiCommit  string            `json:"kapi_commit"`
	Hosts       []string          `json:"hosts"`
	HostVersion map[string]string `json:"host_versions"`
	Tasks       []string          `json:"tasks"`
	Attempts    int               `json:"attempts"`
	Key         EvalKey           `json:"key"`
	Apply       []EvalApplyRow    `json:"apply"`
	Grow        []EvalGrowRow     `json:"grow"`
	ApplyHosts  []EvalApplyHost   `json:"apply_hosts"`
	GrowHosts   []EvalGrowHost    `json:"grow_hosts"`
	// Settle is Measure 3, which needs no live run and is measured whenever a
	// report is built.
	Settle *EvalSettle `json:"settle,omitempty"`
	// Review is a person's answers to the review sheet, when given.
	Review      *EvalReviewAnswers `json:"review,omitempty"`
	ReviewSheet string             `json:"review_sheet,omitempty"`
	// Gaps are places the shipped first run left the harness to complete, and
	// notes preparation made about the wiring, one line each.
	Gaps []string `json:"gaps"`
}

// growMeasured lists the Measure 2 runs, leaving out smoke runs.
func (r EvalReport) growMeasured() []EvalGrowRow {
	out := []EvalGrowRow{}
	for _, row := range r.Grow {
		if row.Phase == evalPhaseGrow {
			out = append(out, row)
		}
	}
	return out
}

func reportEval(opts EvalOptions) error {
	var record evalStudyRecord
	if err := readPairedJSON(filepath.Join(opts.Dir, "study.json"), &record); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("no evaluation has been recorded here; run the preflight and a live phase first")
		}
		return err
	}
	answersPath := opts.Answers
	if answersPath == "" {
		answersPath = filepath.Join(opts.Dir, "review-answers.yaml")
	}
	answers, err := readEvalReviewAnswers(answersPath)
	if err != nil {
		return err
	}
	report, err := buildEvalReport(opts.Dir, record, opts.Fixture, answers)
	if err != nil {
		return err
	}
	stem := filepath.Join(opts.Dir, "report-"+pairedTimestamp())
	sheet := stem + "-review.md"
	if len(report.Grow) != 0 {
		report.ReviewSheet = sheet
		if err := writePairedExclusive(sheet, []byte(renderEvalReviewSheet(report))); err != nil {
			return err
		}
	}
	if err := writePairedJSON(stem+".json", report); err != nil {
		return err
	}
	rendered := renderEvalReport(report)
	if err := writePairedExclusive(stem+".md", []byte(rendered)); err != nil {
		return err
	}
	fmt.Print(rendered)
	fmt.Printf("eval: %d retained attempts; %s.md\n", report.Attempts, stem)
	return nil
}

// buildEvalReport scores every saved attempt in an evidence directory.
func buildEvalReport(dir string, record evalStudyRecord, fixture EvalFixture, answers *EvalReviewAnswers) (EvalReport, error) {
	report := EvalReport{
		Schema: evalSchema, CreatedAt: time.Now().UTC(), Study: record.Manifest.Study,
		Fingerprint: record.Fingerprint, KapiVersion: record.KapiVersion, KapiCommit: record.KapiCommit,
		Hosts: []string{}, HostVersion: map[string]string{}, Tasks: record.Manifest.Tasks, Key: fixture.Key,
		Apply: []EvalApplyRow{}, Grow: []EvalGrowRow{}, ApplyHosts: []EvalApplyHost{}, GrowHosts: []EvalGrowHost{},
		Review: answers, Gaps: []string{},
	}
	for _, host := range record.Manifest.Hosts {
		report.Hosts = append(report.Hosts, host.Host)
	}
	settle, err := measureEvalSettle(context.Background())
	if err != nil {
		return report, fmt.Errorf("measure settling: %w", err)
	}
	report.Settle = &settle
	paths, err := evalAttemptPaths(dir)
	if err != nil {
		return report, err
	}
	sort.Strings(paths)
	report.Attempts = len(paths)
	for _, path := range paths {
		var attempt evalAttempt
		if err := readPairedJSON(path, &attempt); err != nil {
			return report, err
		}
		if attempt.Fingerprint != record.Fingerprint {
			return report, fmt.Errorf("attempt fingerprint differs: %s", path)
		}
		report.HostVersion[attempt.Session.Host.Host] = attempt.HostVersion
		for _, gap := range attempt.Prepared.Wiring.Harness {
			report.Gaps = pairedUnique(report.Gaps, gap)
		}
		for _, note := range attempt.Prepared.Notes {
			if strings.HasPrefix(note, "the wired server") || strings.HasPrefix(note, "the transcript reader") {
				report.Gaps = pairedUnique(report.Gaps, note)
			}
		}
		result, transcript, found, err := evalReadResult(filepath.Dir(path), attempt)
		if err != nil {
			return report, err
		}
		status, errText := result.Status, result.Error
		if !found {
			status, errText = "reserved", "attempt was reserved but no result was committed"
		}
		tools := evalToolNames(transcript.kapiCalls())
		switch attempt.Session.Measure {
		case evalMeasureApply:
			row := EvalApplyRow{Session: attempt.Session.ID, Host: attempt.Session.Host.Host, Task: attempt.Session.Task,
				Status: status, Error: errText, KapiTools: tools, Isolated: result.UserDataUntouched,
				HeldBefore: result.HeldBefore, HeldAfter: result.HeldAfter}
			first, final := EvalCheck{Findings: []EvalCheckFinding{}}, EvalCheck{Findings: []EvalCheckFinding{}}
			if result.CheckFirst != nil {
				first = *result.CheckFirst
			}
			if result.CheckFinal != nil {
				final = *result.CheckFinal
			}
			row.Score = scoreEvalApply(fixture.Key, result.FirstAdded, result.FinalAdded, first, final, transcript)
			report.Apply = append(report.Apply, row)
		case evalMeasureGrow:
			row := EvalGrowRow{Session: attempt.Session.ID, Phase: attempt.Phase, Host: attempt.Session.Host.Host,
				Task: attempt.Session.Task, Status: status, Error: errText, KapiTools: tools,
				Isolated: result.UserDataUntouched, Score: scoreEvalGrow(fixture, result.Recorded)}
			report.Grow = append(report.Grow, row)
		}
	}
	for _, host := range report.Hosts {
		report.ApplyHosts = append(report.ApplyHosts, evalApplyVerdict(host, len(record.Manifest.Tasks), report.Apply))
		report.GrowHosts = append(report.GrowHosts, evalGrowVerdict(host, len(record.Manifest.Tasks), report.growMeasured()))
	}
	return report, nil
}

// evalReadResult reads an attempt's result, rescoring its saved stream so a
// change to the transcript reader reaches sessions already run.
func evalReadResult(dir string, attempt evalAttempt) (evalAttemptResult, EvalTranscript, bool, error) {
	var result evalAttemptResult
	err := readPairedJSON(filepath.Join(dir, "result.json"), &result)
	if errors.Is(err, os.ErrNotExist) {
		return result, EvalTranscript{Calls: []EvalCall{}}, false, nil
	}
	if err != nil {
		return result, result.Transcript, false, err
	}
	transcript := result.Transcript
	if rescored, err := scanEvalTranscript(filepath.Join(dir, "transcript.jsonl"), attempt.Session.Host.Host); err == nil {
		evalRecoverIdentity(&rescored, attempt.Prepared)
		transcript = rescored
	}
	return result, transcript, true, nil
}

// evalCompleted reports a run whose session finished on its own.
func evalCompleted(status string) bool { return status == "completed" }

// evalApplyVerdict folds one host's Measure 1 runs against the bar: at least
// the share of applicable rules followed in the first saved version, and no
// failing finding in any final version.
func evalApplyVerdict(host string, want int, rows []EvalApplyRow) EvalApplyHost {
	result := EvalApplyHost{Host: host}
	for _, row := range rows {
		if row.Host != host {
			continue
		}
		result.Runs++
		if !evalCompleted(row.Status) {
			continue
		}
		result.Completed++
		result.Applicable += len(row.Score.ApplicableFirst)
		result.Followed += len(row.Score.FollowedFirst)
		result.FailingFinal += row.Score.FailingFinal
		if row.Score.CheckedBeforeDone {
			result.CheckedBeforeDone++
		}
	}
	if result.Applicable != 0 {
		result.Share = float64(result.Followed) / float64(result.Applicable)
	}
	switch {
	case result.Completed < want:
		result.Verdict = fmt.Sprintf("unmeasured: %d of %d runs completed", result.Completed, want)
	case result.Applicable == 0:
		result.Verdict = "unmeasured: no run gave occasion to follow a rule"
	case result.Share >= evalApplyFirstBar && result.FailingFinal == 0:
		result.Verdict = "pass"
	default:
		result.Verdict = "fail"
	}
	return result
}

// evalGrowVerdict folds one host's Measure 2 runs against the bar: on average
// at least half the names and spellings recorded, at least the precision bar
// over every record, and the decoy never proposed.
func evalGrowVerdict(host string, want int, rows []EvalGrowRow) EvalGrowHost {
	result := EvalGrowHost{Host: host}
	recall := 0.0
	for _, row := range rows {
		if row.Host != host {
			continue
		}
		result.Runs++
		// The decoy counts whether or not the session finished: a rule it
		// proposed is in the store either way.
		if row.Score.DecoyProposed {
			result.DecoyProposed = true
		}
		if !evalCompleted(row.Status) {
			continue
		}
		result.Completed++
		recall += row.Score.recall()
		result.Records += len(row.Score.Records)
		result.Precise += row.Score.Precise
	}
	if result.Completed != 0 {
		result.MeanRecall = recall / float64(result.Completed)
	}
	if result.Records != 0 {
		result.Precision = float64(result.Precise) / float64(result.Records)
	}
	switch {
	case result.Completed < want:
		result.Verdict = fmt.Sprintf("unmeasured: %d of %d runs completed", result.Completed, want)
	case result.MeanRecall >= evalGrowRecallBar && result.Records != 0 &&
		result.Precision >= evalGrowPrecisionBar && !result.DecoyProposed:
		result.Verdict = "pass"
	default:
		result.Verdict = "fail"
	}
	return result
}

func renderEvalReport(report EvalReport) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# Agent evaluation: %s\n\n", report.Study)
	commit := strings.TrimSpace(report.KapiCommit)
	if len(commit) > 12 {
		commit = commit[:12]
	}
	fmt.Fprintf(&out, "%s, from commit %s. Attempts reserved: %d. Scored from saved attempts; no model call.\n",
		evalOr(strings.TrimSpace(report.KapiVersion), "kapi version unrecorded"), evalOr(commit, "unrecorded"), report.Attempts)
	hosts := []string{}
	for _, host := range report.Hosts {
		hosts = append(hosts, host+" "+evalOr(report.HostVersion[host], "version unrecorded"))
	}
	fmt.Fprintf(&out, "Hosts: %s.\n\n", strings.Join(hosts, "; "))

	out.WriteString("## Measure 1: agents apply context\n\n")
	fmt.Fprintf(&out, "The eleven planted conventions are held as rules a person confirmed. Bar: %.0f%% of applicable\n"+
		"rules followed in the first saved version, and no failing finding in the final one.\n\n", evalApplyFirstBar*100)
	if len(report.Apply) == 0 {
		out.WriteString("No run yet.\n\n")
	} else {
		out.WriteString("| Run | Status | Followed first time | Failing, first | Failing, final | Checked before done | kapi tools |\n|---|---|---|---|---|---|---|\n")
		for _, row := range report.Apply {
			fmt.Fprintf(&out, "| %s | %s | %d of %d | %d | %d | %s | %s |\n", row.Session, evalStatus(row.Status, row.Error),
				len(row.Score.FollowedFirst), len(row.Score.ApplicableFirst), row.Score.FailingFirst, row.Score.FailingFinal,
				evalYes(row.Score.CheckedBeforeDone), evalToolList(row.KapiTools))
		}
		out.WriteString("\n| Host | Completed | Followed first time | Failing, final | Checked before done | Verdict |\n|---|---|---|---|---|---|\n")
		for _, host := range report.ApplyHosts {
			fmt.Fprintf(&out, "| %s | %d of %d | %d of %d (%.0f%%) | %d | %d | %s |\n", host.Host, host.Completed, host.Runs,
				host.Followed, host.Applicable, host.Share*100, host.FailingFinal, host.CheckedBeforeDone, host.Verdict)
		}
		out.WriteString("\n")
		for _, row := range report.Apply {
			if len(row.Score.BrokenFirst)+len(row.Score.BrokenFinal) == 0 && row.HeldAfter == row.HeldBefore {
				continue
			}
			fmt.Fprintf(&out, "- %s: broken first time %s; broken at the end %s", row.Session,
				evalToolList(row.Score.BrokenFirst), evalToolList(row.Score.BrokenFinal))
			if row.HeldAfter != row.HeldBefore {
				fmt.Fprintf(&out, "; rules held went from %d to %d", row.HeldBefore, row.HeldAfter)
			}
			out.WriteString("\n")
		}
		out.WriteString("\n")
	}

	out.WriteString("## Measure 2: agents grow context\n\n")
	fmt.Fprintf(&out, "Nothing is held. Bar: on average %.0f%% of the names and spellings recorded, %.0f%% of records\n"+
		"matching the key or harmless, and the decoy never proposed as a rule.\n\n", evalGrowRecallBar*100, evalGrowPrecisionBar*100)
	if len(report.Grow) == 0 {
		out.WriteString("No run yet.\n\n")
	} else {
		out.WriteString("| Run | Status | Names and spellings | Records | Key, harmless, noise | Decoy proposed | kapi tools |\n|---|---|---|---|---|---|---|\n")
		for _, row := range report.Grow {
			key, harmless, noise := evalVerdictCounts(row.Score)
			label := row.Session
			if row.Phase == evalPhaseSmoke {
				label += " (smoke)"
			}
			fmt.Fprintf(&out, "| %s | %s | %d of %d | %d | %d, %d, %d | %s | %s |\n", label, evalStatus(row.Status, row.Error),
				len(row.Score.Recalled), row.Score.RecallOf, len(row.Score.Records), key, harmless, noise,
				evalYes(row.Score.DecoyProposed), evalToolList(row.KapiTools))
		}
		out.WriteString("\nSmoke runs prove the pipeline and are left out of the verdicts.\n\n")
		out.WriteString("| Host | Completed | Mean recall | Precision | Decoy proposed | Verdict |\n|---|---|---|---|---|---|\n")
		for _, host := range report.GrowHosts {
			fmt.Fprintf(&out, "| %s | %d of %d | %.0f%% | %d of %d (%.0f%%) | %s | %s |\n", host.Host, host.Completed, host.Runs,
				host.MeanRecall*100, host.Precise, host.Records, host.Precision*100, evalYes(host.DecoyProposed), host.Verdict)
		}
		out.WriteString("\n")
	}

	renderEvalSettle(&out, report.Settle)

	out.WriteString("## Measure 4: review is worth it\n\n")
	switch {
	case report.ReviewSheet == "":
		out.WriteString("No grow run yet, so there is no review sheet.\n\n")
	case report.Review == nil:
		fmt.Fprintf(&out, "The review sheet is %s. No answers are recorded yet.\n\n", report.ReviewSheet)
	default:
		kept, total := 0, 0
		for _, row := range report.Grow {
			total += len(row.Score.Records)
			kept += len(report.Review.Keep[row.Session])
		}
		fmt.Fprintf(&out, "Review sheet: %s.\n\n", report.ReviewSheet)
		fmt.Fprintf(&out, "- Minutes spent: %g\n", report.Review.Minutes)
		fmt.Fprintf(&out, "- Records worth keeping: %d of %d\n", kept, total)
		fmt.Fprintf(&out, "- Learned about the fixture that was not planted: %s\n\n",
			evalOr(strings.Join(strings.Fields(report.Review.Learned), " "), "nothing"))
	}

	if len(report.Gaps) != 0 {
		out.WriteString("## What the harness wired by hand\n\n")
		for _, gap := range report.Gaps {
			out.WriteString("- " + gap + "\n")
		}
	}
	return out.String()
}

func evalVerdictCounts(score EvalGrowScore) (key, harmless, noise int) {
	for _, record := range score.Records {
		switch record.Verdict {
		case evalVerdictKey:
			key++
		case evalVerdictHarmless:
			harmless++
		default:
			noise++
		}
	}
	return key, harmless, noise
}

func evalStatus(status, errText string) string {
	if errText != "" {
		return status + " (" + evalCell(errText) + ")"
	}
	return status
}

func evalToolList(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

func evalYes(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
