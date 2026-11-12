package backend

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/screens"
	"github.com/neokapi/neokapi/terms"
)

// DesktopFinding is one content-check finding, flattened for the React Checks
// panel. It mirrors core/check.Finding but adds the fields the panel needs to
// locate the offending block and, where the rule names a replacement, the change
// operation that applies it.
type DesktopFinding struct {
	Category string `json:"category"`
	// Fails says whether the finding fails the check, as the rule that raised
	// it decides. A finding that does not fail reports.
	Fails        bool   `json:"fails"`
	Message      string `json:"message"`
	Suggestion   string `json:"suggestion,omitempty"`
	OriginalText string `json:"original_text,omitempty"`
	// BlockID identifies the block the finding applies to (the format's stable
	// block ID), so the document view can open at it.
	BlockID string `json:"block_id,omitempty"`
	// Field is which side of the block the offending text lives on:
	// "source" or "target".
	Field string `json:"field,omitempty"`
	// Locale is the language OriginalText is written in — the source locale for
	// a source-side finding, the target locale for a target-side one — so the
	// panel can render it with the right direction and lang attribute.
	Locale string `json:"locale,omitempty"`
	// Replacement is the wording the rule asks for (a term's replacement).
	// Empty when the rule names none.
	Replacement string `json:"replacement,omitempty"`
	// Fix is the change operation that applies Replacement, as
	// kapi.change/v1 JSON: a replace_text of the words the finding objects to,
	// addressed to the block's own edition under the revision the check read
	// (check.Fix). The panel sends it to Apply as it is. Empty when the finding
	// has no fix: no replacement, a translation's finding, or a comment.
	Fix string `json:"fix,omitempty"`
	// Rule names what fired, so a finding can be traced to the decision that
	// produced it rather than only to the text it objects to.
	Rule string `json:"rule,omitempty"`
	// Point is the coordinate the checked file sits at, written the way the
	// explorer addresses it. Empty is the project's own point.
	Point string `json:"point,omitempty"`
	// Collection is the one the checked file belongs to.
	Collection string `json:"collection,omitempty"`
	// Position is the run range the finding applies to, anchored to the block's
	// source runs (check.Finding). It is what lets a surface underline the
	// offending words instead of describing them, and what a reviewer needs to
	// see which placeholder in a sentence of four the checker means.
	Position model.Anchor `json:"position,omitzero"`
	// Lines are the lines of the file the finding's block spans, when its place
	// in the file is known, as it is for a comment.
	Lines *format.LineRange `json:"lines,omitempty"`
	// SourceRuns are the block's source runs, so a surface can show the finding
	// in the text it was raised on with Position marked over it, rather than
	// quoting OriginalText alone.
	SourceRuns []model.Run `json:"source_runs,omitempty"`
	// TargetRuns are the block's runs in the target locale a target-side finding
	// names, when the block carries them.
	TargetRuns []model.Run `json:"target_runs,omitempty"`
	// Check is the family of checks that raised the finding, as `kapi check`
	// names it: terms, voice, placeholder, dnt, or a comment check's name.
	Check string `json:"check,omitempty"`
	// Term is the word a term rule is about, when a term rule raised it.
	Term string `json:"term,omitempty"`
	// Suggested marks a finding raised by a rule nobody has established. It
	// never fails.
	Suggested bool `json:"suggested,omitempty"`
}

// CheckFileResult groups the findings for a single content file.
type CheckFileResult struct {
	Path     string           `json:"path"`
	Findings []DesktopFinding `json:"findings"`
}

// CheckRunResult is the structured result of a RunChecks pass over a project,
// the unit the Checks panel renders and an assistant fix-loop acts on.
type CheckRunResult struct {
	// Pass is true exactly when Verdict is "passed".
	Pass bool `json:"pass"`
	// Verdict is "passed", "failed" or "did_not_run", decided by
	// check.Report.Decide the way `kapi check` decides it: a run that checked
	// no blocks, or whose checkers could not show they catch a known-bad input,
	// did not run and is never shown as passing.
	Verdict string `json:"verdict"`
	// DidNotRun lists why the verdict is did_not_run, and DidNotRunCause names
	// the kind: "checker_invalid", "nothing_to_check" or "content_not_checked".
	DidNotRun      []string          `json:"did_not_run,omitempty"`
	DidNotRunCause string            `json:"did_not_run_cause,omitempty"`
	Score          int               `json:"score"`
	Files          []CheckFileResult `json:"files"`
	// Warnings name configuration to fix, such as a key a voice profile carries
	// that the profile model does not define. They are the warnings a
	// kapi.check/v2 report carries, and never change the verdict or the score.
	Warnings []check.Warning `json:"warnings,omitempty"`
}

// RunChecks runs `kapi check` over the content files the Active Filter selects
// (its collections and glob; all when empty), and holds each file's
// translations in the filter's target languages to the checks a translation is
// held to. With no languages selected, only the sources are checked.
//
// The findings come from the host's project check (host.App.ComputeProjectCheck),
// run inside the project document cache so unchanged files replay instead of
// re-parsing. The panel and `kapi check` therefore report the same findings,
// verdict and score for the same content.
func (a *App) RunChecks(tabID string, filter ProjectFilter) (*CheckRunResult, error) {
	return a.runChecks(tabID, filter, checkScope{})
}

// checkScope narrows a checks run to one file, for the Reader, and keeps the
// source blocks the run read from it.
type checkScope struct {
	// file is the project-relative, slash-separated file the run checks; empty
	// checks every file the filter selects.
	file string
	// blocks, when set, receives the source blocks the run read from file:
	// the blocks its findings name and its fixes are guarded by.
	blocks *[]*model.Block
}

// runChecks is RunChecks within scope. A run narrowed to one file is never the
// run the Overview counts findings from.
func (a *App) runChecks(tabID string, filter ProjectFilter, scope checkScope) (*CheckRunResult, error) {
	langs, lerr := canonicalLocales(filter.Languages)
	if lerr != nil {
		return nil, lerr
	}
	filter.Languages = langs
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("tab %q not found", tabID)
	}

	pctx := project.NewProjectContext(op.Project, op.Path)
	resolved, err := pctx.ResolveContent(a.formatReg)
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}
	sourceLang := string(pctx.SourceLocale)
	root := ""
	if op.Path != "" {
		root = filepath.Dir(op.Path)
	}

	// The files the run hands the check, each with the translations the
	// filter's languages select, at the paths the project resolves for them.
	req := host.ProjectCheckRequest{Root: root, SourceLang: sourceLang}
	if op.onDisk() {
		req.Recipe = op.Path
	}
	byPath := map[string]project.ResolvedFile{}
	for _, rf := range resolved {
		if filter.FilesNarrowed() && !filter.MatchesFile(rf.Collection, rf.Relative) {
			continue
		}
		if scope.file != "" && filepath.ToSlash(rf.Relative) != scope.file {
			continue
		}
		ref := host.ProjectCheckFileRef{Path: rf.Path}
		// A file declared for its comments alone holds no value to translate.
		if !rf.CommentsOnly() {
			for _, lang := range filter.Languages {
				if tgt := a.resolveTargetPath(rf, op, lang); tgt != "" {
					ref.Targets = append(ref.Targets, host.ProjectCheckTargetRef{Locale: lang, Path: tgt})
				}
			}
		}
		byPath[rf.Path] = rf
		req.Files = append(req.Files, ref)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// The host.App behind a checks run holds single-occupancy state (the open
	// document cache), so a checks run is serialized.
	a.checksMu.Lock()
	defer a.checksMu.Unlock()
	capp := a.checksCLI()

	var run host.ProjectCheck
	runErr := capp.WithDocumentCache(root, func() error {
		var cerr error
		run, cerr = capp.ComputeProjectCheck(ctx, req)
		return cerr
	})
	if runErr != nil {
		return nil, runErr
	}

	files := make([]CheckFileResult, 0, len(run.Files))
	for _, f := range run.Files {
		rf := byPath[f.Path]
		if scope.blocks != nil && *scope.blocks == nil {
			*scope.blocks = readerBlocks(f.Blocks)
		}
		doc := filepath.ToSlash(rf.Relative)
		findings := make([]DesktopFinding, 0, len(f.Findings))
		for _, d := range f.Findings {
			findings = append(findings, desktopFinding(d, f.Blocks, "source", sourceLang, rf.Collection, doc))
		}
		for _, t := range f.Targets {
			for _, d := range t.Findings {
				findings = append(findings, desktopFinding(d, t.Blocks, "target", t.Locale, rf.Collection, doc))
			}
		}
		sortDesktopFindings(findings)
		files = append(files, CheckFileResult{Path: f.Path, Findings: findings})
	}

	report := run.Report
	result := &CheckRunResult{
		Pass:           report.Pass,
		Verdict:        string(report.Verdict),
		DidNotRun:      report.DidNotRun,
		DidNotRunCause: report.DidNotRunCause,
		Score:          report.Summary.Score,
		Files:          files,
		Warnings:       report.Warnings,
	}
	// A run over every file in every target language is the one the Overview
	// counts findings from.
	if scope.file == "" && coversProject(filter, op.Project) {
		op.recordCheck(result, time.Now())
	}
	return result, nil
}

// readerBlocks are the blocks of a checked file the Reader draws: the
// reader's own, without the comment layer the check adds when the recipe
// declares the file's comments.
func readerBlocks(blocks []*model.Block) []*model.Block {
	out := make([]*model.Block, 0, len(blocks))
	for _, b := range blocks {
		if !comment.IsBlock(b) {
			out = append(out, b)
		}
	}
	return out
}

// desktopFinding flattens one finding of the host's project check for the
// panel. blocks are the blocks the finding was raised over, which locate its
// block by key; field is the side of the block the finding is on ("source" or
// "target") and locale that side's language. doc is the checked file
// project-relative, which a fix addresses.
//
// A finding raised over the whole document (a voice's required pattern) names
// no block. A finding on a comment carries the comment's lines.
func desktopFinding(d check.Diagnostic, blocks []*model.Block, field, locale, collection, doc string) DesktopFinding {
	f := DesktopFinding{
		Category:     strings.TrimPrefix(d.Rule, d.Check+"."),
		Check:        d.Check,
		Suggested:    d.Suggested,
		Fails:        d.Fails,
		Message:      d.Message,
		Suggestion:   d.Suggestion,
		OriginalText: d.Location.Snippet,
		Field:        field,
		Locale:       locale,
		Rule:         d.Rule,
		Collection:   collection,
		Lines:        d.Location.Lines,
		Replacement:  d.Metadata["replacement"],
		Term:         d.Metadata["term"],
	}
	// A finding on a block names the rule as every surface that places findings
	// in blocks names it (screens.FindingRule), so the Reader keys it the same
	// way on the desktop and on the platform. A finding on a comment keeps the
	// rule `kapi check` reports.
	if d.Location.CommentSHA256 == "" {
		f.Rule = findingRule(check.Finding{Category: f.Category, Metadata: d.Metadata, OriginalText: d.Location.Snippet})
	}
	if d.Point != nil {
		f.Point = project.ChannelRef{Profile: d.Point.Profile, Channel: d.Point.Channel}.String()
	}
	if d.Location.Anchor != nil {
		f.Position = *d.Location.Anchor
	}
	if b := blockByKey(blocks, d.Location.Block); b != nil && !comment.IsBlock(b) {
		f.BlockID = b.ID
		f.SourceRuns = b.SourceRuns()
		if field == "target" {
			f.TargetRuns = b.TargetRuns(model.LocaleID(locale))
		}
	} else {
		f.BlockID = d.Location.Block
	}
	if d.Fix != nil && field == "source" && doc != "" {
		op := *d.Fix
		op.At.Doc = doc
		if raw, err := encodeChange(&op); err == nil {
			f.Fix = raw
		}
	}
	return f
}

// blockByKey finds the block a finding names by its key (convergence.BlockKey),
// nil when the finding names none.
func blockByKey(blocks []*model.Block, key string) *model.Block {
	if key == "" {
		return nil
	}
	for _, b := range blocks {
		if b != nil && convergence.BlockKey(b) == key {
			return b
		}
	}
	return nil
}

// checksCLI lazily builds the host.App behind RunChecks, sharing the desktop's
// plugin-wired format and tool registries so plugin-provided formats read
// exactly as they do everywhere else in the app. It owns the document cache a
// checks run opens, which is why it is a second App rather than the engine —
// but it borrows the engine's project stores, so a check that resolves
// terminology or brand governance reads through the same handle everything else
// does. Callers must hold checksMu.
func (a *App) checksCLI() *host.App {
	if a.checks == nil {
		a.checks = a.borrowEngine(&host.App{FormatReg: a.formatReg, ToolReg: a.toolReg})
	}
	return a.checks
}

// readBlocksForChecks reads a file's translatable blocks through the shared
// CLI check pipeline (host.App.ReadBlocksForCheck) — the adapter the review and
// inspect paths use for one-shot reads outside a checks run. fmtName may be
// empty to auto-detect by extension; fmtCfg carries the recipe's reader config
// for the item, nil for reader defaults.
func (a *App) readBlocksForChecks(ctx context.Context, path, fmtName string, fmtCfg map[string]any, sourceLang string) ([]*model.Block, error) {
	a.checksMu.Lock()
	defer a.checksMu.Unlock()
	return a.checksCLI().ReadBlocksForCheck(ctx, path, fmtName, fmtCfg, sourceLang)
}

// pointResolver caches the voice profile and vocabulary for each resolved
// channel reference. Files at the same context point share governance.
//
// Resolution is per point because profiles can bind different voice and terms
// stores. Applying one project-wide result would check files against the wrong
// rules and attach incorrect governance coordinates to the findings.
type pointResolver struct {
	err  error // resolution error; strict check callers must inspect it
	app  *App
	op   *openProject
	proj *project.KapiProject
	root string
	// instant is the governance clock for the whole operation, so two files
	// cannot resolve against different moments.
	instant time.Time
	// held is true when the caller already owns checksMu, as a checks run does
	// for its whole duration.
	held   bool
	voices map[string]*coreprofile.VoiceProfile
	terms  map[string]terms.Terminology
}

// newPointResolver builds a resolver for one operation over one project. held
// says whether the caller already owns checksMu.
func (a *App) newPointResolver(op *openProject, held bool) *pointResolver {
	if op == nil || op.Project == nil || op.Path == "" {
		return nil
	}
	return &pointResolver{
		app:     a,
		op:      op,
		proj:    op.Project,
		root:    filepath.Dir(op.Path),
		instant: a.hostEngine().GovernanceInstant(),
		held:    held,
		voices:  map[string]*coreprofile.VoiceProfile{},
		terms:   map[string]terms.Terminology{},
	}
}

// point builds the governance point for a file.
func (v *pointResolver) point(collection, relPath string) project.GovernancePoint {
	return project.GovernancePoint{Collection: collection, Path: relPath, At: v.instant}
}

// key names the point a file resolves to. Resolution errors are retained for
// strict check callers; best-effort callers can continue using the empty key.
func (v *pointResolver) key(pt project.GovernancePoint) string {
	rc, err := v.proj.ResolveGovernanceFor(pt)
	if err != nil {
		v.err = err
		return ""
	}
	return rc.Ref().String()
}

// termsAt returns the vocabulary decided for this file's point —
// what `kapi check` runs its gate against.
//
// A surface passing no terminology reports only what a voice profile forbids
// and stays silent about every term the project itself retired, which is a
// different answer about the same file. Best-effort: nil when nothing binds.
func (v *pointResolver) termsAt(ctx context.Context, collection, relPath string) terms.Terminology {
	if v == nil {
		return nil
	}
	k := v.key(v.point(collection, relPath))
	if tb, ok := v.terms[k]; ok {
		return tb
	}
	tb := v.loadTerms(ctx, relPath)
	v.terms[k] = tb
	return tb
}

// loadTerms performs one vocabulary resolution through the same host resolver
// the CLI's gate uses, taking checksMu unless the caller holds it.
func (v *pointResolver) loadTerms(ctx context.Context, relPath string) terms.Terminology {
	// A folder opened without a kapi.yaml has no recipe on disk for the host
	// resolver to load, and the recipe drafted for it binds no terms store, so
	// there is no vocabulary to read. Asking would fail the whole run with a
	// missing-file error the person cannot act on.
	if v.op.unsaved.Load() {
		return nil
	}
	if !v.held {
		v.app.checksMu.Lock()
		defer v.app.checksMu.Unlock()
	}
	cmd, err := v.app.contextCommand(ctx, v.op)
	if err != nil {
		v.err = err
		return nil
	}
	tb, terr := v.app.checksCLI().ProjectTermsForFile(
		ctx, cmd, filepath.Join(v.root, filepath.FromSlash(relPath)),
	)
	if terr != nil {
		v.err = terr
		return nil
	}
	return tb
}

// at returns the voice profile governing the point (collection, relPath), which
// is the point a run resolves for that file.
//
// Best-effort: nil when nothing is bound or resolution fails, and the voice
// vocabulary check is then skipped — the CLI's flag-free behaviour.
func (v *pointResolver) at(ctx context.Context, collection, relPath string) *coreprofile.VoiceProfile {
	if v == nil {
		return nil
	}
	pt := v.point(collection, relPath)
	k := v.key(pt)
	if p, ok := v.voices[k]; ok {
		return p
	}
	profile := v.load(ctx, pt)
	v.voices[k] = profile
	return profile
}

// load performs one resolution, taking checksMu unless the caller holds it.
func (v *pointResolver) load(ctx context.Context, pt project.GovernancePoint) *coreprofile.VoiceProfile {
	if !v.held {
		v.app.checksMu.Lock()
		defer v.app.checksMu.Unlock()
	}
	engine := v.app.checksCLI()
	store, release, serr := engine.ProjectVoiceStore(ctx, v.root)
	if serr != nil {
		v.err = serr
		return nil
	}
	defer release()
	p, _, ok, err := engine.ResolveVoiceProfile(
		ctx, v.proj, v.root, host.VoiceResolveOptions{Point: pt, Store: store},
	)
	if err != nil {
		v.err = err
		return nil
	}
	if !ok {
		return nil
	}
	return p
}

// --- helpers ---------------------------------------------------------------

// resolveTargetPath derives the on-disk path of the translated file for a
// source file and target language from the content item's target template,
// with the locale spelled the way the project's locale_format writes it. It
// goes through the shared resolver (project.ProjectContext.TargetFile), the one
// the runner and merge write outputs through. Returns "" when the item declares
// no target template.
func (a *App) resolveTargetPath(rf project.ResolvedFile, op *openProject, targetLang string) string {
	if rf.Item == nil || rf.Item.Target == "" || op == nil || op.Project == nil {
		return ""
	}
	pctx := project.NewProjectContext(op.Project, op.Path)
	return pctx.TargetFile(*rf.Item, filepath.ToSlash(rf.Relative), model.LocaleID(targetLang))
}

// toDesktopFinding flattens a check.Finding for the panel, wiring the block ID,
// the field the offending text lives on, and a structured replacement (from
// Metadata["replacement"], set by the vocabulary checker) when one exists.
//
// Rule and Point travel with it so a finding can be acted on: the C-series
// promise is that a finding names the rule, the point and the fix, and a reader
// who cannot see which rule fired cannot go and change it.
func toDesktopFinding(f check.Finding, b *model.Block, field string, locale string, point ContextPointDTO) DesktopFinding {
	replacement := ""
	if f.Metadata != nil {
		replacement = f.Metadata["replacement"]
	}
	df := DesktopFinding{
		Category:     f.Category,
		Fails:        f.Fails && !f.Suggested,
		Message:      f.Message,
		Suggestion:   f.Suggestion,
		OriginalText: f.OriginalText,
		BlockID:      b.ID,
		Field:        field,
		Locale:       locale,
		Replacement:  replacement,
		Rule:         findingRule(f),
		Point:        pointRef(point),
		Collection:   point.Collection,
		Position:     f.Position,
		SourceRuns:   b.SourceRuns(),
		Term:         f.Metadata["term"],
		Suggested:    f.Suggested,
	}
	if field == "target" {
		df.TargetRuns = b.TargetRuns(model.LocaleID(locale))
	}
	return df
}

// withFix gives a source-side finding the change operation that applies its
// replacement (check.Fix) to b, a block of the document doc, a
// project-relative path, written in source. The operation names the revision
// the check read, so a change to the block since then is refused as stale with
// the text it now holds.
func withFix(df DesktopFinding, f check.Finding, b *model.Block, doc string, source model.LocaleID) DesktopFinding {
	if df.Field != "source" || doc == "" || comment.IsBlock(b) {
		return df
	}
	op := check.Fix(f, b.SourceRuns(), change.Ref{Doc: doc, Block: change.BlockKey(b)}, check.SourceRevision(b, source))
	if op == nil {
		return df
	}
	raw, err := encodeChange(op)
	if err != nil {
		return df
	}
	df.Fix = raw
	return df
}

// findingRule names the rule that fired (screens.FindingRule). A finding
// that named nothing would leave a reader with a complaint and nowhere to go.
func findingRule(f check.Finding) string { return screens.FindingRule(f) }

// pointRef renders the point a finding is scoped to, as the explorer addresses
// it. The project's own point renders empty rather than as a guessed name.
func pointRef(p ContextPointDTO) string {
	return project.ChannelRef{Profile: p.Profile, Channel: p.Channel}.String()
}

// sortDesktopFindings orders failing findings first, for stable panel output.
func sortDesktopFindings(fs []DesktopFinding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].Fails != fs[j].Fails {
			return fs[i].Fails
		}
		return fs[i].Category < fs[j].Category
	})
}
