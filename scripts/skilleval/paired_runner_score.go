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
	WriteRoute        string                    `json:"write_route,omitempty"`
	MCPExposure       string                    `json:"mcp_exposure,omitempty"`
	OutsideCell       []string                  `json:"outside_cell,omitempty"`
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
			"Superseded rows were run again under -paired-retry and are left out of the summary. Each verdict is the one " +
			"recorded when its attempt finished; integrity says whether the graded files still match it, so review " +
			"copies of the workspaces rather than the workspaces themselves.",
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
	markdown.WriteString("\n## Attempts\n\n| Phase | Task | Model | Integration | Status | Artifact criteria | Identity / integrity | Seconds | Tools | Refusals | Outside cell | Evidence | Human review |\n" +
		"|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
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
			"| %s | %s | %s | %s | %s | %s | %s / %s | %.1f | %d | %s | %s | %s | %s |\n",
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
			strings.Join(row.OutsideCell, ", "),
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

// pairedSummaryCell is one task, host and condition of the summary.
type pairedSummaryCell struct {
	n, passed, completed, overrides, outside, changed int
	// unmeasured counts the attempts whose host reported no token use,
	// which every median leaves out alike.
	unmeasured                    int
	seconds, input, output, tools []float64
	refusals, invalid             map[string]int
	// The stale-recovery columns, for a task with another editor.
	interfered                                                   bool
	exercised, afterWrite, neverLanded, conflict                 int
	passedExercised, passedConflict, passedAfter, passedUnlanded int
}

// pairedConflictSignals are the refusals that tell an agent its view of a
// file is out of date.
var pairedConflictSignals = []string{"stale", "host:stale", "host:patch_failed"}

// writePairedSummary groups the current attempts of the natural phases by
// task, host and condition. An attempt cut short by a rate limit, an
// interruption, a failed launch or an infrastructure failure never had its
// chance, and is left out until PAIRED_EVAL_RETRY=1 runs it again.
func writePairedSummary(markdown *strings.Builder, rows []pairedScoreRow) {
	cells := map[[3]string]*pairedSummaryCell{}
	routes := map[[4]string]*pairedSummaryCell{}
	held, absent := 0, 0
	exposure := map[string]int{}
	for _, row := range rows {
		if row.Superseded || row.Phase == "diagnostic" {
			continue
		}
		if slices.Contains(pairedRetryStatuses, row.Status) {
			held++
			continue
		}
		if row.Status == "mcp_absent" {
			absent++
			continue
		}
		if row.MCPExposure != "" {
			exposure[row.MCPExposure]++
		}
		key := [3]string{row.Session.Task, row.Session.Agent.Host, row.Session.Condition}
		c := cells[key]
		if c == nil {
			c = &pairedSummaryCell{refusals: map[string]int{}, invalid: map[string]int{}}
			cells[key] = c
		}
		c.add(row)
		route := row.WriteRoute
		if route == "" {
			route = "unrecorded"
		}
		rkey := [4]string{key[0], key[1], key[2], route}
		r := routes[rkey]
		if r == nil {
			r = &pairedSummaryCell{refusals: map[string]int{}, invalid: map[string]int{}}
			routes[rkey] = r
		}
		r.add(row)
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
	markdown.WriteString("## Summary\n\nMedians over each cell's current attempts. Outside cell counts the attempts " +
		"whose tool calls named a path outside their cell (another attempt, the checkout, the home directory or a " +
		"shared temporary directory): read their transcripts before counting them. Changed counts the attempts whose " +
		"graded files no longer match what was graded; their recorded verdict stands.\n\n")
	if held > 0 {
		fmt.Fprintf(markdown, "%d attempts are left out: a rate limit, an interruption, a failed launch or an "+
			"infrastructure failure cut them short. PAIRED_EVAL_RETRY=1 runs them again.\n\n", held)
	}
	if absent > 0 {
		fmt.Fprintf(markdown, "%d mcp attempts are left out: the host gave the model no kapi tools, so they did not "+
			"run the arm.\n\n", absent)
	}
	if len(exposure) > 0 {
		fmt.Fprintf(markdown, "What the mcp attempts show of the kapi tools the model was given: declared by the "+
			"host %d, called without a declared list %d, unverified %d.\n\n",
			exposure["declared"], exposure["called"], exposure["unverified"])
	}
	markdown.WriteString("| Task | Host | Condition | n | Objective passed | Completed | Seconds | Input tokens | Output tokens | Tool calls | Refusals | Override attempts | Outside cell | Changed |\n" +
		"|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, key := range keys {
		c := cells[key]
		fmt.Fprintf(markdown, "| %s | %s | %s | %d | %d | %d | %.0f | %.0f | %.0f | %.0f | %s | %d | %d | %d |\n",
			key[0], key[1], key[2], c.n, c.passed, c.completed,
			pairedMedian(c.seconds), pairedMedian(c.input), pairedMedian(c.output), pairedMedian(c.tools),
			pairedRefusalText(c.refusals), c.overrides, c.outside, c.changed)
	}
	var unmeasured []string
	for _, key := range keys {
		if n := cells[key].unmeasured; n > 0 {
			unmeasured = append(unmeasured, fmt.Sprintf("%s %s %s (%d)", key[0], key[1], key[2], n))
		}
	}
	if len(unmeasured) > 0 {
		fmt.Fprintf(markdown, "\nThe medians leave out the attempts whose host reported no token use, in seconds and "+
			"tool calls as in tokens: %s.\n", strings.Join(unmeasured, ", "))
	}
	writePairedRouteSummary(markdown, routes)
	writePairedStaleSummary(markdown, keys, cells)
	writePairedInvalidSummary(markdown, keys, cells)
}

// writePairedRouteSummary splits each cell's attempts by the route they wrote
// the task's files through, with the medians of each: what an attempt costs
// depends on the route it took as much as on its condition.
func writePairedRouteSummary(markdown *strings.Builder, routes map[[4]string]*pairedSummaryCell) {
	if len(routes) == 0 {
		return
	}
	keys := make([][4]string, 0, len(routes))
	for key := range routes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		for k := range 4 {
			if keys[i][k] != keys[j][k] {
				if k == 2 {
					return slices.Index(pairedConditions, keys[i][k]) < slices.Index(pairedConditions, keys[j][k])
				}
				return keys[i][k] < keys[j][k]
			}
		}
		return false
	})
	markdown.WriteString("\n## Write routes\n\nEach cell's attempts by the route their tool calls took to write the " +
		"task's files: contract (kapi apply, ksed -i or apply_edits), merge (kapi merge), native (the host's own " +
		"edit, write or patch tools, or a shell command rewriting a task file), several joined with +, or none. " +
		"Medians as in the summary.\n\n" +
		"| Task | Host | Condition | Route | n | Objective passed | Seconds | Input tokens | Tool calls |\n" +
		"|---|---|---|---|---|---|---|---|---|\n")
	for _, key := range keys {
		c := routes[key]
		fmt.Fprintf(markdown, "| %s | %s | %s | %s | %d | %d | %.0f | %.0f | %.0f |\n",
			key[0], key[1], key[2], key[3], c.n, c.passed,
			pairedMedian(c.seconds), pairedMedian(c.input), pairedMedian(c.tools))
	}
}

func (c *pairedSummaryCell) add(row pairedScoreRow) {
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
	if len(row.OutsideCell) > 0 {
		c.outside++
	}
	if row.ArtifactIntegrity == "changed" {
		c.changed++
	}
	// An attempt whose host reported no token use ended before the host
	// could account for it. It is left out of every median, so the seconds,
	// tokens and tool calls of a cell describe the same attempts.
	if row.InputTokens != nil && row.OutputTokens != nil {
		c.seconds = append(c.seconds, float64(row.DurationMS)/1000)
		c.tools = append(c.tools, float64(row.ToolCalls))
		c.input = append(c.input, float64(*row.InputTokens))
		c.output = append(c.output, float64(*row.OutputTokens))
	} else {
		c.unmeasured++
	}
	for code, count := range row.Refusals {
		if pointer, ok := strings.CutPrefix(code, "invalid:"); ok {
			c.invalid[pointer] += count
		}
		c.refusals[code] += count
	}
	if row.Interference == nil {
		return
	}
	c.interfered = true
	conflict := false
	for _, signal := range pairedConflictSignals {
		conflict = conflict || row.Refusals[signal] > 0
	}
	if conflict {
		c.conflict++
		if row.ObjectivePassed {
			c.passedConflict++
		}
	}
	switch {
	case !row.Interference.Applied:
		c.neverLanded++
		if row.ObjectivePassed {
			c.passedUnlanded++
		}
	case row.Interference.AgentWroteFirst:
		c.afterWrite++
		if row.ObjectivePassed {
			c.passedAfter++
		}
	default:
		c.exercised++
		if row.ObjectivePassed {
			c.passedExercised++
		}
	}
}

// writePairedStaleSummary splits the attempts of a task with another editor
// by when that editor's change landed: before the agent wrote (the attempt
// worked from a stale read), after it, or never. Recovery is read from the
// first group, and from the attempts that met a conflict signal.
func writePairedStaleSummary(markdown *strings.Builder, keys [][3]string, cells map[[3]string]*pairedSummaryCell) {
	header := false
	for _, key := range keys {
		c := cells[key]
		if !c.interfered {
			continue
		}
		if !header {
			markdown.WriteString("\n## Stale recovery\n\nWhen the other editor's change landed, and how many of " +
				"those attempts passed. A conflict signal is kapi's stale refusal, Claude Code's note that the file " +
				"changed since it was read, or a Codex patch that failed to apply.\n\n" +
				"| Task | Host | Condition | n | Landed before the agent wrote | Passed | Landed after | Passed | Never landed | Passed | Met a conflict signal | Passed |\n" +
				"|---|---|---|---|---|---|---|---|---|---|---|---|\n")
			header = true
		}
		fmt.Fprintf(markdown, "| %s | %s | %s | %d | %d | %d | %d | %d | %d | %d | %d | %d |\n",
			key[0], key[1], key[2], c.n, c.exercised, c.passedExercised, c.afterWrite, c.passedAfter,
			c.neverLanded, c.passedUnlanded, c.conflict, c.passedConflict)
	}
}

// writePairedInvalidSummary lists the change-set fields that failed to
// decode, by the JSON pointer kapi named: the names and shapes agents reach
// for that the contract does not take.
func writePairedInvalidSummary(markdown *strings.Builder, keys [][3]string, cells map[[3]string]*pairedSummaryCell) {
	header := false
	for _, key := range keys {
		c := cells[key]
		if len(c.invalid) == 0 {
			continue
		}
		if !header {
			markdown.WriteString("\n## Change sets that did not decode\n\nEach field kapi refused as invalid, by its " +
				"JSON pointer (array positions as *), and how often.\n\n| Task | Host | Condition | Fields |\n|---|---|---|---|\n")
			header = true
		}
		fmt.Fprintf(markdown, "| %s | %s | %s | %s |\n", key[0], key[1], key[2], pairedRefusalText(c.invalid))
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
	row.WriteRoute = pairedWriteRoute(result.Agent.WriteRoutes)
	row.MCPExposure = result.Agent.MCPExposure
	row.OutsideCell = result.Agent.OutsideCell
	row.Interference = result.Agent.Interference
	row.Validation = result.Validation
	// The verdict is the one recorded when the attempt finished: re-scoring
	// with a different evaluator never silently replaces it. Integrity says
	// whether the graded files still match what was graded.
	if result.Validation != nil {
		row.ObjectivePassed = result.Validation.ObjectivePassed
	}
	hash, err := pairedArtifactHash(filepath.Join(attemptDir, "workspace"), result.ArtifactScope)
	if err != nil {
		row.ArtifactIntegrity = "unreadable"
		row.Error = err.Error()
		return row, nil
	}
	row.ArtifactIntegrity = "verified"
	if hash != result.ArtifactHash {
		row.ArtifactIntegrity = "changed"
	}
	return row, nil
}

func pairedEvidenceLinks(row pairedScoreRow) string {
	path := row.EvidencePath
	return "[result](" + path + "/result.json) · [files](" + path + "/workspace/) · [transcript](" + path + "/transcript.jsonl)"
}
