package backend

// The Overview: the page a project tab opens on. It shows the project's content
// files, what applies to them, and four counts (files, blocks, rules and
// findings), with the suggestions that wait for a person beside them.
//
// OverviewSummary answers the whole page in one call, from what the app already
// holds: the resolved recipe, the block cache the store sync keeps current,
// the operation log and the stores bound to the project, and the last checks
// run over the whole project. It reads no file the store sync has not read,
// runs no check and writes nothing.

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/screens"
)

// overviewSummaryTimeout bounds the reads behind one OverviewSummary call.
const overviewSummaryTimeout = 30 * time.Second

// mapCheckTally is what the Overview keeps of a checks run over the whole project.
type mapCheckTally struct {
	at       time.Time
	findings int
	// byFile counts findings by the file's absolute path.
	byFile map[string]int
	// result is the run itself, which the checks panel opens on.
	result *CheckRunResult
}

// recordStoreRead notes that the store sync found the block cache current with
// the files on disk now.
func (op *openProject) recordStoreRead(at time.Time) {
	op.mapMu.Lock()
	defer op.mapMu.Unlock()
	op.storeReadAt = at
}

// recordCheck keeps the counts of a checks run over the whole project.
func (op *openProject) recordCheck(res *CheckRunResult, at time.Time) {
	t := &mapCheckTally{at: at, byFile: map[string]int{}, result: res}
	for _, f := range res.Files {
		t.byFile[f.Path] += len(f.Findings)
		t.findings += len(f.Findings)
	}
	op.mapMu.Lock()
	defer op.mapMu.Unlock()
	op.lastCheck = t
}

// replaceCheckedFile puts one file's findings, from a checks run over that
// file alone in the same target languages, in place of that file's entry in
// the last whole-project run, so the Overview and the Checks page count what the
// file holds after a fix. path is the file's absolute path. The verdict and
// the score are decided again over every file's findings; a run that did not
// run stays one that did not run. With no whole-project run recorded there
// is nothing to replace.
func (op *openProject) replaceCheckedFile(path string, one *CheckRunResult) {
	var found []DesktopFinding
	for _, f := range one.Files {
		if f.Path == path {
			found = append(found, f.Findings...)
		}
	}
	op.mapMu.Lock()
	defer op.mapMu.Unlock()
	last := op.lastCheck
	if last == nil || last.result == nil {
		return
	}
	res := *last.result
	res.Files = make([]CheckFileResult, 0, len(last.result.Files))
	placed := false
	for _, f := range last.result.Files {
		if f.Path == path {
			if !placed {
				res.Files = append(res.Files, CheckFileResult{Path: path, Findings: found})
				placed = true
			}
			continue
		}
		res.Files = append(res.Files, f)
	}
	if !placed {
		res.Files = append(res.Files, CheckFileResult{Path: path, Findings: found})
	}
	var scored []check.Finding
	failing := false
	for _, f := range res.Files {
		for _, d := range f.Findings {
			scored = append(scored, check.Finding{Category: d.Category, Fails: d.Fails, Suggested: d.Suggested})
			failing = failing || d.Fails
		}
	}
	res.Score = check.CalculateScore(scored).Overall
	if res.Verdict != string(check.VerdictDidNotRun) {
		res.Pass = !failing
		res.Verdict = string(check.VerdictFailed)
		if res.Pass {
			res.Verdict = string(check.VerdictPassed)
		}
	}
	t := &mapCheckTally{at: last.at, byFile: map[string]int{}, result: &res}
	for _, f := range res.Files {
		t.byFile[f.Path] += len(f.Findings)
		t.findings += len(f.Findings)
	}
	op.lastCheck = t
}

// coversProject reports whether a checks run with this filter covers the
// whole project: every file, and every target language the recipe declares.
// Only such a run is the one the Overview counts findings from.
func coversProject(filter ProjectFilter, proj *project.KapiProject) bool {
	if filter.FilesNarrowed() {
		return false
	}
	if proj == nil {
		return true
	}
	checked := make(map[string]bool, len(filter.Languages))
	for _, l := range filter.Languages {
		checked[l] = true
	}
	targets, err := canonicalLocales(localeStrings(proj.Defaults.TargetLanguages))
	if err != nil {
		return false
	}
	for _, l := range targets {
		if !checked[l] {
			return false
		}
	}
	return true
}

// LastProjectChecks returns the last checks run over the whole project in
// this session, the run the Overview's findings count is read from, so the checks
// panel opens on the findings that count names. Nil when none has run.
func (a *App) LastProjectChecks(tabID string) (*CheckRunResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	_, check := op.mapReads()
	if check == nil {
		return nil, nil
	}
	return check.result, nil
}

// mapReads returns what this session recorded for the Map.
func (op *openProject) mapReads() (time.Time, *mapCheckTally) {
	op.mapMu.Lock()
	defer op.mapMu.Unlock()
	return op.storeReadAt, op.lastCheck
}

// OverviewSummary answers the Overview of the project a tab holds in one call. A tab
// with no recipe on disk, or holding a project's context alone, answers with
// zeros.
func (a *App) OverviewSummary(tabID string) (*screens.OverviewSummaryDTO, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	out := &screens.OverviewSummaryDTO{
		ProjectKey:      string(op.workspaceKey),
		TargetLanguages: []string{},
		FileRows:        []screens.OverviewFileDTO{},
		Suggestions:     []screens.MapSuggestionDTO{},
	}
	if op.contextOnly || op.Project == nil || op.Path == "" {
		return out, nil
	}
	if err := a.checkProjectOnDisk(op); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), overviewSummaryTimeout)
	defer cancel()

	targets, err := canonicalLocales(localeStrings(op.Project.Defaults.TargetLanguages))
	if err != nil {
		return nil, err
	}
	out.TargetLanguages = append(out.TargetLanguages, targets...)

	pctx := project.NewProjectContext(op.Project, op.Path)
	resolved, err := pctx.ResolveContent(a.formatReg)
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}

	readAt, check := op.mapReads()
	if !readAt.IsZero() {
		out.ReadAt = readAt.UTC().Format(time.RFC3339Nano)
	}
	if check != nil {
		n := check.findings
		out.Findings = &n
		out.CheckedAt = check.at.UTC().Format(time.RFC3339Nano)
	}

	blocksByFile, err := a.mapBlockCounts(ctx, op)
	if err != nil {
		return nil, err
	}
	out.ReadFiles = len(blocksByFile)

	seen := map[string]bool{}
	for _, rf := range resolved {
		rel := filepath.ToSlash(rf.Relative)
		if seen[rel] {
			continue
		}
		seen[rel] = true
		row := screens.OverviewFileDTO{
			Path:       rf.Path,
			Relative:   rel,
			Format:     rf.Format,
			Collection: rf.Collection,
			Blocks:     blocksByFile[rel],
		}
		if check != nil {
			row.Findings = check.byFile[rf.Path]
		}
		out.Blocks += row.Blocks
		out.FileRows = append(out.FileRows, row)
	}
	out.Files = len(out.FileRows)
	screens.SortOverviewFiles(out.FileRows)

	rules, err := a.mapRuleCount(ctx, op)
	if err != nil {
		return nil, err
	}
	out.Rules = rules

	if op.workspaceKey != "" {
		if err := a.mapSuggestions(ctx, op, out); err != nil {
			return nil, err
		}
	}
	out.Steps = a.overviewSteps(ctx, op, out, check)
	return out, nil
}

// overviewSteps reads the first project's steps for the Overview: none for a project
// the workspace does not hold or whose steps the person dismissed. A state it
// cannot read counts as a step not taken, so the step stays on the list.
func (a *App) overviewSteps(ctx context.Context, op *openProject, out *screens.OverviewSummaryDTO, check *mapCheckTally) []screens.OverviewStepDTO {
	if op.workspaceKey == "" || a.overviewStepsDismissed(string(op.workspaceKey)) {
		return nil
	}
	state := screens.OverviewStepState{Awaiting: out.Awaiting}
	if check != nil && check.result != nil {
		state.Checked = true
		for _, f := range check.result.Files {
			for _, d := range f.Findings {
				if d.Fails {
					state.Failing++
				}
			}
		}
	}
	a.mu.RLock()
	recipe := op.Path
	a.mu.RUnlock()
	if root, ok := projectRoot(op); ok {
		if hosts, err := a.assistantHosts(ctx, recipe, root); err == nil {
			for _, h := range hosts {
				state.Connected = state.Connected || h.Connected
			}
		}
	}
	if rung, err := host.RecipeRung(recipe); err == nil {
		state.Shared = rung != host.ShareRungMachine
	}
	return screens.OverviewSteps(state)
}

// localeStrings turns the recipe's target languages into strings.
func localeStrings[L ~string](in []L) []string {
	out := make([]string, 0, len(in))
	for _, l := range in {
		out = append(out, string(l))
	}
	return out
}

// mapBlockCounts counts the block cache's blocks by the file they were read
// from, project-relative and slash-separated. It counts the blocks the status
// counts (translatable ones), so the Overview's total matches the collections'.
// A project with no block cache yet counts none, and reading it creates no
// store.
func (a *App) mapBlockCounts(ctx context.Context, op *openProject) (map[string]int, error) {
	counts := map[string]int{}
	db, ok := a.existingProjectStore(op) //nolint:contextcheck // the store opens under the engine's own context, as GetProjectStatus opens it
	if !ok {
		return counts, nil
	}
	if has, err := db.HasBlocks(ctx); err != nil || !has {
		return counts, nil //nolint:nilerr // an unreadable or empty cache counts no blocks, as GetProjectStatus treats it
	}
	store := db.BlocksAutocommit()
	if store == nil {
		return counts, nil
	}
	sess, err := store.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("open block store session: %w", err)
	}
	defer func() { _ = sess.Close() }()
	translatable := true
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Translatable: &translatable}) {
		if berr != nil {
			return nil, fmt.Errorf("read blocks: %w", berr)
		}
		counts[filepath.ToSlash(b.Properties.File)]++
	}
	return counts, nil
}

// mapRuleCount counts the rules in force across the project's places, the
// count `kapi look` reports, through host.ProjectRuleCount. A folder whose
// recipe is not written yet has no place to count rules at and counts none.
func (a *App) mapRuleCount(ctx context.Context, op *openProject) (int, error) {
	if !op.onDisk() {
		return 0, nil
	}
	a.mu.RLock()
	recipe := op.Path
	a.mu.RUnlock()
	n, err := a.hostEngine().ProjectRuleCount(ctx, recipe)
	if err != nil {
		return 0, fmt.Errorf("count rules: %w", err)
	}
	return n, nil
}

// mapSuggestions fills the items awaiting a decision from the project's
// digest, the same read the Decide view makes, with each item's evidence
// counted from the operation that carries it.
func (a *App) mapSuggestions(ctx context.Context, op *openProject, out *screens.OverviewSummaryDTO) error {
	if !op.onDisk() {
		// Suggestions are recorded against a recipe; a folder without one
		// has none yet.
		return nil
	}
	digest, err := a.hostEngine().ContextDigest(ctx, host.ContextDigestRequest{Key: op.workspaceKey})
	if err != nil {
		return fmt.Errorf("read the project's suggestions: %w", err)
	}
	log, err := a.hostEngine().ContextOperations(ctx, host.ContextLogRequest{
		Project:  op.Path,
		Subjects: true,
	})
	if err != nil {
		return fmt.Errorf("read the suggestions' evidence: %w", err)
	}
	evidence := make(map[string][]contextop.Evidence, len(log.Operations))
	for _, r := range log.Operations {
		evidence[r.ID] = r.Evidence
	}
	var suggested []screens.DigestItem
	for _, theme := range digest.Suggested {
		for _, g := range theme.Groups {
			suggested = append(suggested, g.Items...)
		}
	}
	rows := screens.MapSuggestionRows(digest.Conflicts, suggested, evidence)
	out.Suggestions = append(out.Suggestions, rows.Rows...)
	out.Conflicts, out.Suggested = rows.Conflicts, rows.Suggested
	out.Awaiting = out.Conflicts + out.Suggested
	return nil
}
