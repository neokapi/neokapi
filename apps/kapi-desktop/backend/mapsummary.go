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
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// mapSummaryTimeout bounds the reads behind one MapSummary call.
const mapSummaryTimeout = 30 * time.Second

// MapSummaryDTO is everything the Map renders.
type MapSummaryDTO struct {
	// ProjectKey is the project's workspace key, empty for a tab the
	// workspace does not hold.
	ProjectKey string `json:"project_key,omitempty"`
	// Files counts the project's content files: the files its collections
	// declare, as the recipe resolves them.
	Files int `json:"files"`
	// Blocks counts the blocks the block cache holds for those files.
	Blocks int `json:"blocks"`
	// Rules counts the rules in force at the project's own point: the
	// concepts in the bound terms store, the word rules and pattern rules of
	// the voice profile, and the notes a person kept.
	Rules int `json:"rules"`
	// Findings counts the findings of the last checks run over the whole
	// project in this session. Nil when none has run.
	Findings *int `json:"findings"`
	// CheckedAt is when that run finished, RFC3339; empty when none has run.
	CheckedAt string `json:"checked_at,omitempty"`
	// ReadFiles counts the files the block cache holds blocks for, and ReadAt
	// is when the store sync last found the cache current with the files on
	// disk, RFC3339. ReadAt is empty until the first sync in this session.
	ReadFiles int    `json:"read_files"`
	ReadAt    string `json:"read_at,omitempty"`
	// Suggested counts the suggestions that advise and wait for a person, and
	// Conflicts the disagreements. Awaiting is the two together: the rows
	// under Suggestions.
	Suggested int `json:"suggested"`
	Conflicts int `json:"conflicts"`
	Awaiting  int `json:"awaiting"`
	// TargetLanguages are the languages the recipe declares, canonical
	// BCP-47; empty for a source-only project.
	TargetLanguages []string `json:"target_languages"`
	// FileRows has one row per content file, the most findings first, then by
	// path.
	FileRows []MapFileDTO `json:"file_rows"`
	// Suggestions are the items awaiting a decision: the disagreements, then
	// the suggestions, in the order the digest lists them.
	Suggestions []MapSuggestionDTO `json:"suggestions"`
}

// MapFileDTO is one content file on the Map.
type MapFileDTO struct {
	// Path is absolute, for the preview; Relative is project-relative and
	// slash-separated, for the row.
	Path       string `json:"path"`
	Relative   string `json:"relative"`
	Format     string `json:"format,omitempty"`
	Collection string `json:"collection,omitempty"`
	// Blocks counts the file's blocks in the block cache.
	Blocks int `json:"blocks"`
	// Findings counts the file's findings in the last checks run over the
	// whole project, zero when none has run.
	Findings int `json:"findings"`
}

// MapSuggestionDTO is one item awaiting a decision.
type MapSuggestionDTO struct {
	// Group is "conflict" for a disagreement, whose Item is its first side,
	// or "suggested". It and the item's id make the key the Decide view
	// selects the row by.
	Group string          `json:"group"`
	Item  host.DigestItem `json:"item"`
	// Conflict is the disagreement, for a conflict row.
	Conflict *host.DigestConflict `json:"conflict,omitempty"`
	// Blocks and Files count the places the evidence names: distinct blocks
	// (a unit, or a quote where no unit is recorded) and distinct files.
	Blocks int `json:"blocks"`
	Files  int `json:"files"`
}

// mapCheckTally is what the Map keeps of a checks run over the whole project.
type mapCheckTally struct {
	at       time.Time
	findings int
	// byFile counts findings by the file's absolute path.
	byFile map[string]int
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
	t := &mapCheckTally{at: at, byFile: map[string]int{}}
	for _, f := range res.Files {
		t.byFile[f.Path] += len(f.Findings)
		t.findings += len(f.Findings)
	}
	op.mapMu.Lock()
	defer op.mapMu.Unlock()
	op.lastCheck = t
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
func (a *App) MapSummary(tabID string) (*MapSummaryDTO, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	out := &MapSummaryDTO{
		ProjectKey:      string(op.workspaceKey),
		TargetLanguages: []string{},
		FileRows:        []MapFileDTO{},
		Suggestions:     []MapSuggestionDTO{},
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
		row := MapFileDTO{
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
	sortMapFiles(out.FileRows)

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

// sortMapFiles orders the file rows the most findings first, then by path.
func sortMapFiles(rows []MapFileDTO) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Findings != rows[j].Findings {
			return rows[i].Findings > rows[j].Findings
		}
		return rows[i].Relative < rows[j].Relative
	})
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

	if op.workspaceKey != "" {
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
func (a *App) mapSuggestions(ctx context.Context, op *openProject, out *MapSummaryDTO) error {
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
	row := func(group string, item host.DigestItem, conflict *host.DigestConflict) MapSuggestionDTO {
		blocks, files := evidenceCounts(evidence[item.ID])
		return MapSuggestionDTO{Group: group, Item: item, Conflict: conflict, Blocks: blocks, Files: files}
	}
	for i := range digest.Conflicts {
		c := digest.Conflicts[i]
		if len(c.Sides) == 0 {
			continue
		}
		out.Suggestions = append(out.Suggestions, row("conflict", c.Sides[0], &c))
		out.Conflicts++
	}
	for _, theme := range digest.Suggested {
		for _, g := range theme.Groups {
			for _, item := range g.Items {
				out.Suggestions = append(out.Suggestions, row("suggested", item, nil))
				out.Suggested++
			}
		}
	}
	out.Awaiting = out.Conflicts + out.Suggested
	return nil
}

// evidenceCounts counts the distinct blocks and files a list of evidence
// names. A block is a file's unit, or its quote where no unit is recorded; a
// web page counts as neither.
func evidenceCounts(ev []contextop.Evidence) (blocks, files int) {
	seenFile := map[string]bool{}
	seenBlock := map[string]bool{}
	for _, e := range ev {
		if e.Path == "" {
			continue
		}
		if !seenFile[e.Path] {
			seenFile[e.Path] = true
			files++
		}
		at := e.Unit
		if at == "" {
			at = "quote:" + e.Quote
		}
		if key := e.Path + "\x00" + at; !seenBlock[key] && (e.Unit != "" || e.Quote != "") {
			seenBlock[key] = true
			blocks++
		}
	}
	return blocks, files
}
