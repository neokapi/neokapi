package backend

// The Map: the page a project tab opens on. It shows the project's content
// files, what applies to them, and four counts (files, blocks, rules and
// findings), with the suggestions that wait for a person beside them.
//
// MapSummary answers the whole page in one call, from what the app already
// holds: the resolved recipe, the block cache the store sync keeps current,
// the operation log and the stores bound to the project, and the last checks
// run over the whole project. It reads no file the store sync has not read,
// runs no check and writes nothing.

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/screens"
)

// mapSummaryTimeout bounds the reads behind one MapSummary call.
const mapSummaryTimeout = 30 * time.Second

// mapCheckTally is what the Map keeps of a checks run over the whole project.
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
// the last whole-project run, so the Map and the Checks page count what the
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
// Only such a run is the one the Map counts findings from.
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
// this session, the run the Map's findings count is read from, so the checks
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

// MapSummary answers the Map of the project a tab holds in one call. A tab
// with no recipe on disk, or holding a project's context alone, answers with
// zeros.
func (a *App) MapSummary(tabID string) (*screens.MapSummaryDTO, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	out := &screens.MapSummaryDTO{
		ProjectKey:      string(op.workspaceKey),
		TargetLanguages: []string{},
		FileRows:        []screens.MapFileDTO{},
		Suggestions:     []screens.MapSuggestionDTO{},
	}
	if op.contextOnly || op.Project == nil || op.Path == "" {
		return out, nil
	}
	if err := a.checkProjectOnDisk(op); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), mapSummaryTimeout)
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
		row := screens.MapFileDTO{
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
	screens.SortMapFiles(out.FileRows)

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
	return out, nil
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
// counts (translatable ones), so the Map's total matches the collections'.
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

// mapRuleCount counts the rules in force at the project's own point. Those
// are the concepts in the terms store the project binds, where a kept term
// rule is recorded, and the word rules and patterns of the voice profile. The
// notes a person kept count too; the operation log alone holds them.
func (a *App) mapRuleCount(ctx context.Context, op *openProject) (int, error) {
	total := 0
	if _, ok := a.existingProjectStore(op); ok { //nolint:contextcheck // the store opens under the engine's own context
		n, err := a.mapStoredRuleCount(ctx, op)
		if err != nil {
			return 0, err
		}
		total += n
	}

	// The context log reads a project by its recipe, so a folder whose recipe
	// is not written yet has no kept notes to count.
	if op.workspaceKey != "" && op.onDisk() {
		log, lerr := a.hostEngine().ContextOperations(ctx, host.ContextLogRequest{
			Project:  op.Path,
			Status:   contextop.StatusEstablished,
			Subjects: true,
		})
		if lerr != nil {
			return 0, fmt.Errorf("read kept notes: %w", lerr)
		}
		for _, r := range log.Operations {
			if r.Subject.Kind == contextop.SubjectNote && r.Project == op.workspaceKey {
				total++
			}
		}
	}
	return total, nil
}

// mapStoredRuleCount counts the rules the project's stores hold: the concepts
// in its terms store and the rules of the voice profile at its own point. The
// caller asks only when the project has a store, so the Map never brings one
// into being.
func (a *App) mapStoredRuleCount(ctx context.Context, op *openProject) (int, error) {
	total := 0
	src, cleanup, err := a.contextExplorerSources(ctx, op)
	if err != nil {
		return 0, err
	}
	defer cleanup()
	if src.TermsErr == nil && src.Terms != nil {
		_, n, terr := src.Terms.Search(ctx, "", "", "", 0, 1)
		if terr != nil {
			return 0, fmt.Errorf("count terms: %w", terr)
		}
		total += n
	}

	root := filepath.Dir(op.Path)
	voiceStore, release, verr := a.hostEngine().ProjectVoiceStore(ctx, root)
	if verr == nil {
		defer release()
		profile, _, _, ok, perr := a.hostEngine().LoadCollectionVoice(ctx, op.Project, root,
			host.VoiceResolveOptions{
				Store: voiceStore,
				Point: project.GovernancePoint{At: a.hostEngine().GovernanceInstant()},
			})
		if perr == nil && ok {
			total += voiceRuleCount(profile)
		}
	}
	return total, nil
}

// voiceRuleCount counts the rules a voice profile applies to text: the word
// rules its file carries and its prohibited and required patterns.
func voiceRuleCount(p *coreprofile.VoiceProfile) int {
	if p == nil {
		return 0
	}
	n := 0
	for _, set := range coreprofile.CarriedRuleSets(p) {
		for _, r := range set.Rules {
			if strings.TrimSpace(r.Term) != "" {
				n++
			}
		}
	}
	for _, pat := range append(append([]coreprofile.Pattern{}, p.Style.ProhibitedPatterns...), p.Style.RequiredPatterns...) {
		if strings.TrimSpace(pat.Regex) != "" {
			n++
		}
	}
	return n
}

// mapSuggestions fills the items awaiting a decision from the project's
// digest, the same read the Decide view makes, with each item's evidence
// counted from the operation that carries it.
func (a *App) mapSuggestions(ctx context.Context, op *openProject, out *screens.MapSummaryDTO) error {
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
