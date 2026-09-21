package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// The person's half of the drill.
//
// The second measure is how many candidates a person confirms. No judge model
// stands in for that, and the harness never confirms anything itself: it prints
// the sheet, prints the exact commands, and afterwards reads back what the
// store holds. A person who prefers the desktop app points it at the cell's
// data root and decides there; the readback is the same either way.

// ColdStartReviewCell is one cell's sheet: what it holds and what a person did.
type ColdStartReviewCell struct {
	Cell            string               `json:"cell"`
	Host            string               `json:"host"`
	Task            string               `json:"task"`
	Repo            string               `json:"repo"`
	DataDir         string               `json:"data_dir"`
	Candidates      []ColdStartOperation `json:"candidates"`
	Confirmed       int                  `json:"confirmed"`
	Discarded       int                  `json:"discarded"`
	Recorded        int                  `json:"recorded"`
	WithEvidence    int                  `json:"with_evidence"`
	WithoutEvidence int                  `json:"without_evidence"`
	// AgentRecorded and PersonRecorded are the actor kinds the store holds over
	// everything this cell recorded.
	AgentRecorded  int    `json:"agent_recorded"`
	PersonRecorded int    `json:"person_recorded"`
	Error          string `json:"error,omitempty"`
}

// ColdStartReview is every cell's sheet at one moment.
type ColdStartReview struct {
	Schema    int                   `json:"schema"`
	CreatedAt time.Time             `json:"created_at"`
	Study     string                `json:"study"`
	Cells     []ColdStartReviewCell `json:"cells"`
	// Pending is the number of candidates still waiting for a decision.
	Pending int `json:"pending"`
	// DesktopApp is the application the sheet's desktop command opens: the name
	// the shipped bundle registers, or a path to a locally built one.
	DesktopApp string `json:"desktop_app"`
}

// reviewColdStart writes a review sheet for every cell that has run a first
// session, and reads back the decisions already taken. Running it again after
// a person decides is how the readback is refreshed.
func reviewColdStart(ctx context.Context, opts ColdStartOptions) error {
	review := ColdStartReview{Schema: coldStartSchema, CreatedAt: time.Now().UTC(), Study: opts.Manifest.Study,
		Cells: []ColdStartReviewCell{}, DesktopApp: opts.DesktopApp}
	for _, cell := range coldStartReviewedCells(opts) {
		paths := coldStartPaths(filepath.Join(opts.SandboxRoot, cell.ID))
		entry := ColdStartReviewCell{
			Cell: cell.ID, Host: cell.Host.Host, Task: cell.Task,
			Repo: paths.Repo, DataDir: paths.Data, Candidates: []ColdStartOperation{},
		}
		store, err := coldStartReadStore(ctx, paths)
		if err != nil {
			entry.Error = err.Error()
			review.Cells = append(review.Cells, entry)
			continue
		}
		entry.Candidates = store.Candidates
		entry.Confirmed, entry.Discarded = store.Confirmed, store.Discarded
		entry.Recorded = store.WithEvidence + store.WithoutEvidence
		entry.WithEvidence, entry.WithoutEvidence = store.WithEvidence, store.WithoutEvidence
		entry.AgentRecorded, entry.PersonRecorded = store.AgentRecorded, store.PersonRecorded
		review.Pending += len(store.Candidates)
		review.Cells = append(review.Cells, entry)
	}
	stem := filepath.Join(opts.Dir, "review-"+pairedTimestamp())
	if err := writePairedJSON(stem+".json", review); err != nil {
		return err
	}
	sheet := renderColdStartReview(review)
	if err := writePairedExclusive(stem+".md", []byte(sheet)); err != nil {
		return err
	}
	fmt.Print(sheet)
	fmt.Printf("cold start: %d candidates awaiting a decision; %s.md\n", review.Pending, stem)
	return nil
}

// coldStartReviewedCells lists every cell whose first session has been started,
// in a fixed order. A cell that recorded nothing is reviewed too, because an
// empty sheet is the finding.
func coldStartReviewedCells(opts ColdStartOptions) []ColdStartCell {
	started := map[string]bool{}
	for _, phase := range []string{coldStartPhaseSmoke, coldStartPhaseSessionOne} {
		paths, err := filepath.Glob(filepath.Join(opts.Dir, phase, "*", "started.json"))
		if err != nil {
			continue
		}
		for _, path := range paths {
			var attempt coldStartAttempt
			if readPairedJSON(path, &attempt) == nil {
				started[attempt.Session.Cell] = true
			}
		}
	}
	cells := []ColdStartCell{}
	for _, cell := range coldStartCells(opts.Manifest, coldStartPhaseSessionOne) {
		if started[cell.ID] {
			cells = append(cells, cell)
		}
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].ID < cells[j].ID })
	return cells
}

func renderColdStartReview(review ColdStartReview) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# Cold-start review: %s\n\n", review.Study)
	out.WriteString("Each candidate below was recorded by an agent working an ordinary writing task.\n" +
		"Confirm the ones you would keep and discard the rest. The commands are exact:\n" +
		"run them as they stand, from the cell's repository.\n\n")
	for _, cell := range review.Cells {
		fmt.Fprintf(&out, "## %s (%s, task %s)\n\n", cell.Cell, cell.Host, cell.Task)
		if cell.Error != "" {
			fmt.Fprintf(&out, "The store could not be read: %s\n\n", cell.Error)
			continue
		}
		fmt.Fprintf(&out, "Recorded %d, of which %d carry evidence. Recorded by an agent: %d; by a person: %d.\n",
			cell.Recorded, cell.WithEvidence, cell.AgentRecorded, cell.PersonRecorded)
		fmt.Fprintf(&out, "Decided so far: %d confirmed, %d discarded.\n\n", cell.Confirmed, cell.Discarded)
		if len(cell.Candidates) == 0 {
			out.WriteString("Nothing is waiting for a decision.\n\n")
			continue
		}
		out.WriteString("| id | kind | subject | evidence | recorded by |\n|---|---|---|---|---|\n")
		for _, candidate := range cell.Candidates {
			evidence := strings.Join(candidate.Evidence, "; ")
			if evidence == "" {
				evidence = "none"
			}
			fmt.Fprintf(&out, "| %s | %s | %s | %s | %s |\n",
				candidate.ID, candidate.Kind, coldStartCell(candidate.Subject), coldStartCell(evidence),
				coldStartCell(coldStartActorLabel(candidate)))
		}
		out.WriteString("\n```sh\ncd " + pairedShellQuote(cell.Repo) + "\n")
		for _, candidate := range cell.Candidates {
			fmt.Fprintf(&out, "%s context confirm %s   # or: discard %s\n",
				coldStartReviewEnv(cell), candidate.ID, candidate.ID)
		}
		out.WriteString("```\n\nOr open Kapi Desktop on this cell's workspace:\n\n```sh\n" +
			coldStartDesktopCommand(cell, review.DesktopApp) + "\n```\n\n")
	}
	out.WriteString("Run the review phase again afterwards to read back what you decided.\n")
	return out.String()
}

// coldStartReviewIsolation renders the isolation a person carries in front of a
// decision, so the command acts on the cell rather than on their own workspace.
func coldStartReviewIsolation(cell ColdStartReviewCell) []string {
	root := filepath.Dir(cell.DataDir)
	return []string{
		"KAPI_DATA_DIR=" + pairedShellQuote(cell.DataDir),
		"KAPI_CONFIG_DIR=" + pairedShellQuote(filepath.Join(root, "kapi-config")),
		"XDG_DATA_HOME=" + pairedShellQuote(filepath.Join(root, "xdg-data")),
		"XDG_CACHE_HOME=" + pairedShellQuote(filepath.Join(root, "kapi-cache")),
		"KAPI_PLUGINS_DIR_ONLY=1",
	}
}

// coldStartReviewEnv puts that isolation in front of the cell's own kapi.
func coldStartReviewEnv(cell ColdStartReviewCell) string {
	kapi := filepath.Join(filepath.Dir(cell.DataDir), "bin", "kapi")
	return strings.Join(append(coldStartReviewIsolation(cell), pairedShellQuote(kapi)), " ")
}

// coldStartDesktopApp is the application the shipped bundle registers, from
// `apps/kapi-desktop/build/config.yml`. A locally built bundle is named by its
// path instead.
const coldStartDesktopApp = "Kapi"

// coldStartDesktopCommand opens Kapi Desktop on one cell's workspace.
//
// On macOS `open` hands the application the login session's environment rather
// than the calling shell's, so the cell's roots travel as `--env` arguments and
// `-n` starts an instance of its own to receive them. On every other platform
// the bundle's own executable takes the environment in front of it.
func coldStartDesktopCommand(cell ColdStartReviewCell, app string) string {
	if strings.TrimSpace(app) == "" {
		app = coldStartDesktopApp
	}
	isolation := coldStartReviewIsolation(cell)
	if runtime.GOOS != "darwin" {
		return strings.Join(append(isolation, pairedShellQuote(app)), " ")
	}
	command := []string{"open", "-n"}
	for _, pair := range isolation {
		command = append(command, "--env", pair)
	}
	return strings.Join(append(command, "-a", pairedShellQuote(app)), " ")
}

// coldStartCell keeps a table cell on one row.
func coldStartCell(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "|", "/")
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 90 {
		return value[:87] + "..."
	}
	return value
}
