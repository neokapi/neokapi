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

// The three measures, rendered from saved attempts with no model call.
//
//  1. What the store holds after the first session, by operation kind, and
//     whether each entry carries evidence.
//  2. How many of those candidates a person confirmed. Only a person's own
//     decisions count, read back through `kapi context log`.
//  3. What the second session did: whether it asked what applies before it
//     wrote, and what `kapi check` caught on its output.
//
// A session that recorded nothing is a row with zeros. That is the result the
// drill exists to be able to show.

// ColdStartRow is one cell's line in the report.
type ColdStartRow struct {
	Cell string `json:"cell"`
	Host string `json:"host"`
	Task string `json:"task"`
	// One is the first session.
	One ColdStartStageRow `json:"one"`
	// Two is the second session, empty until it has run.
	Two ColdStartStageRow `json:"two"`
	// Confirmed and Discarded are a person's decisions on this cell.
	Confirmed int  `json:"confirmed"`
	Discarded int  `json:"discarded"`
	Pending   int  `json:"pending"`
	Reviewed  bool `json:"reviewed"`
}

// ColdStartStageRow is what one session did.
type ColdStartStageRow struct {
	Ran       bool           `json:"ran"`
	Status    string         `json:"status"`
	Identity  string         `json:"identity"`
	KapiTools []string       `json:"kapi_tools"`
	Recorded  int            `json:"recorded"`
	ByKind    map[string]int `json:"by_kind"`
	// ByAgent and ByPerson split what this session recorded by the actor kind
	// the store holds. A session is an agent working, so an entry on ByPerson
	// is one whose attribution the store lost.
	ByAgent  int `json:"by_agent"`
	ByPerson int `json:"by_person"`
	// Actors are the agent names and sessions the store holds for this
	// session's entries, one line each.
	Actors          []string        `json:"actors"`
	WithEvidence    int             `json:"with_evidence"`
	WithoutEvidence int             `json:"without_evidence"`
	AskedFirst      bool            `json:"asked_first"`
	SkillLoaded     bool            `json:"skill_loaded"`
	Changed         []string        `json:"changed"`
	Check           *ColdStartCheck `json:"check,omitempty"`
	Isolated        bool            `json:"isolated"`
	Error           string          `json:"error,omitempty"`
}

// ColdStartReport is the whole drill as saved evidence describes it.
type ColdStartReport struct {
	Schema      int               `json:"schema"`
	CreatedAt   time.Time         `json:"created_at"`
	Study       string            `json:"study"`
	Fingerprint string            `json:"fingerprint"`
	KapiVersion string            `json:"kapi_version"`
	KapiCommit  string            `json:"kapi_commit"`
	Hosts       []string          `json:"hosts"`
	HostVersion map[string]string `json:"host_versions"`
	Attempts    int               `json:"attempts"`
	Rows        []ColdStartRow    `json:"rows"`
	// Gaps are places the shipped first run left the harness to complete, one
	// line each, so the report says what it had to wire by hand.
	Gaps           []string `json:"gaps"`
	Interpretation string   `json:"interpretation"`
}

func reportColdStart(ctx context.Context, opts ColdStartOptions) error {
	var record coldStartStudyRecord
	if err := readPairedJSON(filepath.Join(opts.Dir, "study.json"), &record); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("no drill has been recorded here; run the preflight and a session first")
		}
		return err
	}
	report := ColdStartReport{
		Schema: coldStartSchema, CreatedAt: time.Now().UTC(), Study: record.Manifest.Study,
		Fingerprint: record.Fingerprint, KapiVersion: record.KapiVersion, KapiCommit: record.KapiCommit,
		Hosts: []string{}, HostVersion: map[string]string{}, Rows: []ColdStartRow{}, Gaps: []string{},
		Interpretation: "Three measures per cell, from saved attempts. Confirmations are a person's own; " +
			"no judge model stands in for them. A session that recorded nothing is a row with zeros, " +
			"which is the result that would end the premise.",
	}
	for _, host := range record.Manifest.Hosts {
		report.Hosts = append(report.Hosts, host.Host)
	}
	rows := map[string]*ColdStartRow{}
	for _, cell := range coldStartCells(record.Manifest, coldStartPhaseSessionOne) {
		rows[cell.ID] = &ColdStartRow{Cell: cell.ID, Host: cell.Host.Host, Task: cell.Task}
	}
	paths, err := coldStartAttemptPaths(opts.Dir)
	if err != nil {
		return err
	}
	report.Attempts = len(paths)
	for _, path := range paths {
		var attempt coldStartAttempt
		if err := readPairedJSON(path, &attempt); err != nil {
			return err
		}
		if attempt.Fingerprint != record.Fingerprint {
			return fmt.Errorf("attempt fingerprint differs: %s", path)
		}
		row, ok := rows[attempt.Session.Cell]
		if !ok {
			row = &ColdStartRow{Cell: attempt.Session.Cell, Host: attempt.Session.Host.Host, Task: attempt.Session.Task}
			rows[attempt.Session.Cell] = row
		}
		report.HostVersion[attempt.Session.Host.Host] = attempt.HostVersion
		for _, gap := range attempt.Prepared.Wiring.Harness {
			report.Gaps = pairedUnique(report.Gaps, gap)
		}
		stage, err := coldStartStageRow(filepath.Dir(path), attempt)
		if err != nil {
			return err
		}
		if attempt.Session.Stage == coldStartStageTwo {
			row.Two = stage
			continue
		}
		row.One = stage
	}
	if err := coldStartApplyReview(opts.Dir, rows); err != nil {
		return err
	}
	ids := make([]string, 0, len(rows))
	for id := range rows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		report.Rows = append(report.Rows, *rows[id])
	}
	stem := filepath.Join(opts.Dir, "report-"+pairedTimestamp())
	if err := writePairedJSON(stem+".json", report); err != nil {
		return err
	}
	rendered := renderColdStartReport(report)
	if err := writePairedExclusive(stem+".md", []byte(rendered)); err != nil {
		return err
	}
	fmt.Print(rendered)
	fmt.Printf("cold start: %d retained attempts; %s.md\n", report.Attempts, stem)
	return nil
}

// coldStartStageRow folds one saved attempt into a row. An attempt that was
// reserved and never committed a result is kept as a row of its own.
func coldStartStageRow(dir string, attempt coldStartAttempt) (ColdStartStageRow, error) {
	stage := ColdStartStageRow{Ran: true, Status: "reserved", KapiTools: []string{},
		ByKind: map[string]int{}, Actors: []string{}, Changed: []string{}}
	var result coldStartAttemptResult
	err := readPairedJSON(filepath.Join(dir, "result.json"), &result)
	if errors.Is(err, os.ErrNotExist) {
		stage.Error = "attempt was reserved but no result was committed"
		return stage, nil
	}
	if err != nil {
		return stage, err
	}
	stage.Status, stage.Identity, stage.Error = result.Status, result.IdentityStatus, result.Error
	// Score the saved stream rather than the reading taken when it ran, so a
	// change to the scoring rules reaches sessions already run. The store counts
	// stay as they were read live: a later session moved the store on.
	transcript := result.Transcript
	if rescored, err := scanColdStartTranscript(filepath.Join(dir, "transcript.jsonl"), attempt.Session.Host.Host); err == nil {
		coldStartRecoverIdentity(&rescored, attempt.Prepared)
		transcript = rescored
	}
	stage.KapiTools = coldStartToolNames(transcript.kapiCalls())
	stage.AskedFirst, stage.SkillLoaded = transcript.AskedBeforeWriting, transcript.SkillLoaded
	if transcript.ActualModel != "" && attempt.Session.Host.Model != "" {
		stage.Identity = "verified"
		if transcript.ActualModel != attempt.Session.Host.Model {
			stage.Identity = "mismatch"
		}
	}
	stage.Changed, stage.Check, stage.Isolated = result.Changed, result.Check, result.UserDataUntouched
	stage.ByKind = coldStartGrowth(result.StoreBefore, result.StoreAfter)
	for _, count := range stage.ByKind {
		stage.Recorded += count
	}
	stage.attribute(coldStartRecordedSince(result.StoreBefore, result.StoreAfter))
	stage.WithEvidence = max(result.StoreAfter.WithEvidence-result.StoreBefore.WithEvidence, 0)
	stage.WithoutEvidence = max(result.StoreAfter.WithoutEvidence-result.StoreBefore.WithoutEvidence, 0)
	return stage, nil
}

// attribute folds who the store says recorded this session's entries. A
// session is one agent working a task, so every entry it added is an agent's
// work whatever the store holds, and the two counts say whether the store
// agrees.
func (s *ColdStartStageRow) attribute(added []ColdStartOperation) {
	for _, op := range added {
		switch op.Actor {
		case coldStartActorAgent:
			s.ByAgent++
		default:
			s.ByPerson++
		}
		s.Actors = pairedUnique(s.Actors, coldStartActorLabel(op))
	}
}

// coldStartGrowth is what one session added, rather than what the store holds,
// so a second session's row is about the second session.
func coldStartGrowth(before, after ColdStartStore) map[string]int {
	growth := map[string]int{}
	for kind, count := range after.ByKind {
		if added := count - before.ByKind[kind]; added > 0 {
			growth[kind] = added
		}
	}
	return growth
}

// coldStartApplyReview folds the most recent review readback into the rows.
func coldStartApplyReview(dir string, rows map[string]*ColdStartRow) error {
	matches, err := filepath.Glob(filepath.Join(dir, "review-*.json"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	var review ColdStartReview
	if err := readPairedJSON(matches[len(matches)-1], &review); err != nil {
		return err
	}
	for _, cell := range review.Cells {
		row, ok := rows[cell.Cell]
		if !ok {
			continue
		}
		row.Reviewed, row.Confirmed, row.Discarded, row.Pending = true, cell.Confirmed, cell.Discarded, len(cell.Candidates)
	}
	return nil
}

func renderColdStartReport(report ColdStartReport) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# Cold-start drill: %s\n\n%s\n\n", report.Study, report.Interpretation)
	fmt.Fprintf(&out, "kapi %s (%s). Attempts reserved: %d.\n", report.KapiVersion, report.KapiCommit, report.Attempts)
	hosts := []string{}
	for _, host := range report.Hosts {
		hosts = append(hosts, host+" "+coldStartOr(report.HostVersion[host], "version unrecorded"))
	}
	fmt.Fprintf(&out, "Hosts: %s.\n\n", strings.Join(hosts, "; "))

	out.WriteString("## What the first session recorded\n\n")
	out.WriteString("| Cell | Status | kapi tools it called | Recorded | By kind | With evidence | Isolated |\n|---|---|---|---|---|---|---|\n")
	for _, row := range report.Rows {
		fmt.Fprintf(&out, "| %s | %s | %s | %d | %s | %d of %d | %s |\n",
			row.Cell, coldStartStatus(row.One), coldStartToolList(row.One.KapiTools), row.One.Recorded,
			coldStartKinds(row.One.ByKind), row.One.WithEvidence, row.One.Recorded, coldStartYes(row.One.Isolated, row.One.Ran))
	}

	out.WriteString("\n## Who the store says recorded it\n\n")
	out.WriteString("An entry a session added was recorded by an agent, so a count under \"a person\" is an\n" +
		"entry whose attribution the store lost.\n\n")
	out.WriteString("| Cell | First session | Second session | Agent name and session |\n|---|---|---|---|\n")
	flagged := []string{}
	for _, row := range report.Rows {
		actors := row.One.Actors
		for _, actor := range row.Two.Actors {
			actors = pairedUnique(actors, actor)
		}
		fmt.Fprintf(&out, "| %s | %s | %s | %s |\n", row.Cell,
			coldStartAttribution(row.One), coldStartAttribution(row.Two), coldStartCell(strings.Join(actors, "; ")))
		if lost := row.One.ByPerson + row.Two.ByPerson; lost != 0 {
			flagged = append(flagged, fmt.Sprintf("%s: %d of %d entries an agent recorded are attributed to a person",
				row.Cell, lost, row.One.Recorded+row.Two.Recorded))
		}
	}
	if len(flagged) != 0 {
		out.WriteString("\nAttribution to settle before reading the rest:\n\n")
		for _, line := range flagged {
			out.WriteString("- " + line + "\n")
		}
	}

	out.WriteString("\n## What a person confirmed\n\n")
	out.WriteString("| Cell | Reviewed | Confirmed | Discarded | Still waiting |\n|---|---|---|---|---|\n")
	for _, row := range report.Rows {
		reviewed := "not yet"
		if row.Reviewed {
			reviewed = "yes"
		}
		fmt.Fprintf(&out, "| %s | %s | %d | %d | %d |\n", row.Cell, reviewed, row.Confirmed, row.Discarded, row.Pending)
	}

	out.WriteString("\n## What the second session did\n\n")
	out.WriteString("| Cell | Status | Asked before writing | Check | Findings |\n|---|---|---|---|---|\n")
	for _, row := range report.Rows {
		fmt.Fprintf(&out, "| %s | %s | %s | %s | %s |\n",
			row.Cell, coldStartStatus(row.Two), coldStartYes(row.Two.AskedFirst, row.Two.Ran),
			coldStartCheckVerdict(row.Two.Check), coldStartCheckDetail(row.Two.Check))
	}

	if len(report.Gaps) != 0 {
		out.WriteString("\n## What the harness wired by hand\n\n")
		for _, gap := range report.Gaps {
			out.WriteString("- " + gap + "\n")
		}
	}
	return out.String()
}

func coldStartStatus(stage ColdStartStageRow) string {
	if !stage.Ran {
		return "not run"
	}
	if stage.Error != "" {
		return stage.Status + " (" + coldStartCell(stage.Error) + ")"
	}
	return stage.Status
}

// coldStartAttribution renders one session's entries by the actor kind the
// store holds.
func coldStartAttribution(stage ColdStartStageRow) string {
	switch {
	case !stage.Ran:
		return ""
	case stage.ByAgent+stage.ByPerson == 0:
		return "nothing"
	case stage.ByPerson == 0:
		return fmt.Sprintf("agent %d", stage.ByAgent)
	case stage.ByAgent == 0:
		return fmt.Sprintf("person %d", stage.ByPerson)
	default:
		return fmt.Sprintf("agent %d, person %d", stage.ByAgent, stage.ByPerson)
	}
}

func coldStartToolList(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

func coldStartKinds(byKind map[string]int) string {
	if len(byKind) == 0 {
		return "nothing"
	}
	parts := []string{}
	for _, kind := range []string{"observe", "propose", "correct", "confirm", "discard", "revert", "widen"} {
		if count := byKind[kind]; count != 0 {
			parts = append(parts, fmt.Sprintf("%s %d", kind, count))
		}
	}
	return strings.Join(parts, ", ")
}

func coldStartYes(value, ran bool) string {
	switch {
	case !ran:
		return ""
	case value:
		return "yes"
	default:
		return "no"
	}
}

func coldStartCheckVerdict(check *ColdStartCheck) string {
	switch {
	case check == nil:
		return ""
	case check.Error != "":
		return "did not run"
	case check.Pass:
		return fmt.Sprintf("passed, score %d", check.Score)
	default:
		return fmt.Sprintf("failed, score %d", check.Score)
	}
}

func coldStartCheckDetail(check *ColdStartCheck) string {
	if check == nil {
		return ""
	}
	if check.Error != "" {
		return coldStartCell(check.Error)
	}
	if check.Findings == 0 {
		return "none"
	}
	return coldStartCell(strings.Join(check.Messages, "; "))
}

func coldStartOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
