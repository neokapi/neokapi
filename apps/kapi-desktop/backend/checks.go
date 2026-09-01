package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/terms"
)

// DesktopFinding is one content-check finding, flattened for the React Checks
// panel. It mirrors core/check.Finding but adds the fields the panel needs to
// locate the offending block and (when safe) offer a one-click fix.
type DesktopFinding struct {
	Category     string `json:"category"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	Suggestion   string `json:"suggestion,omitempty"`
	OriginalText string `json:"original_text,omitempty"`
	// BlockID identifies the block the finding applies to (the format's stable
	// block ID), so ApplyCheckFix can re-find it.
	BlockID string `json:"block_id,omitempty"`
	// Field is which side of the block the offending text lives on:
	// "source" or "target".
	Field string `json:"field,omitempty"`
	// Locale is the language OriginalText is written in — the source locale for
	// a source-side finding, the target locale for a target-side one — so the
	// panel can render it with the right direction and lang attribute.
	Locale string `json:"locale,omitempty"`
	// Replacement is the structured fix text (e.g. a voice profile's preferred
	// term). Empty when there is no safe automatic replacement.
	Replacement string `json:"replacement,omitempty"`
	// Fixable reports whether the panel may show an "Apply fix" button: a
	// replacement and a block to target both exist.
	Fixable bool `json:"fixable"`
	// Rule names what fired, so a finding can be traced to the decision that
	// produced it rather than only to the text it objects to.
	Rule string `json:"rule,omitempty"`
	// Point is the coordinate the checked file sits at, written the way the
	// explorer addresses it. Empty is the project's own point.
	Point string `json:"point,omitempty"`
	// Collection is the one the checked file belongs to.
	Collection string `json:"collection,omitempty"`
}

// CheckFileResult groups the findings for a single content file.
type CheckFileResult struct {
	Path     string           `json:"path"`
	Findings []DesktopFinding `json:"findings"`
}

// CheckRunResult is the structured result of a RunChecks pass over a project,
// the unit the Checks panel renders and an assistant fix-loop acts on.
type CheckRunResult struct {
	Pass  bool              `json:"pass"`
	Score int               `json:"score"`
	Files []CheckFileResult `json:"files"`
}

// RunChecks runs the project's content checks (placeholder + do-not-translate
// when a target exists, voice vocabulary on the source when a voice profile is
// bound) over the content files the Active Filter selects (its collections +
// glob; all when empty), for the filter's target languages — source-side checks
// run once per file, target-side checks run once per filtered language, and the
// panel no longer carries its own language picker. With no languages selected,
// only source-side checks run.
//
// The pipeline itself is the CLI's exported check service
// (host.App.ReadBlocksForCheck / host.OverlayTargets / host.RunCheckTool /
// host.FindingsFromBlock), run inside the project document cache
// (WithDocumentCache) so unchanged files replay instead of re-parsing —
// exactly the `kapi check` semantics: the gate fails on any critical finding.
func (a *App) RunChecks(tabID string, filter ProjectFilter) (*CheckRunResult, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// One instant for the whole run, so every file's point resolves against the
	// same clock and two findings cannot disagree about which profile governs.
	at := a.hostEngine().GovernanceInstant()

	// The shared check pipeline holds single-occupancy state on the host.App
	// (the open document cache), so a checks run is serialized.
	a.checksMu.Lock()
	defer a.checksMu.Unlock()
	capp := a.checksCLI()

	// The voice is resolved per file's point rather than once for the project:
	// a recipe that binds a different profile per product governs each file by
	// the one at its own point, which is the point the finding reports.
	//
	// Do-not-translate terms come from the bound terms store, project-wide.
	// Both are optional — when absent the corresponding check is skipped,
	// matching the CLI's flag-free behaviour.
	points := a.newPointResolver(op, true)
	dntTerms := a.resolveProjectDNTTerms(ctx, op, sourceLang)

	var allFindings []check.Finding
	files := make([]CheckFileResult, 0, len(resolved))

	runErr := capp.WithDocumentCache(root, func() error {
		for _, rf := range resolved {
			if filter.FilesNarrowed() && !filter.MatchesFile(rf.Collection, rf.Relative) {
				continue
			}

			// Both halves of the recipe's binding: the format the item declares
			// and the reader config the project declares for it. Under reader
			// defaults the panel judges content the project excludes — a demo
			// script's `command:` lines where the recipe's keyPathPatterns select
			// only its prose.
			fmtCfg := pctx.FormatConfigFor(rf.Format, rf.Item)

			// The point this file sits at, resolved the way a run resolves it,
			// so every finding on it can name where it is scoped.
			filePoint, _, ptErr := contextPoint(op.Project, rf.Collection, rf.Relative, at)
			if ptErr != nil {
				filePoint = ContextPointDTO{Collection: rf.Collection, Default: true}
			}

			sourceBlocks, rerr := capp.ReadBlocksForCheck(ctx, rf.Path, rf.Format, fmtCfg, sourceLang)
			if rerr != nil {
				// Surface the read failure as a finding rather than aborting the
				// whole run: one unreadable file should not hide the rest.
				files = append(files, CheckFileResult{
					Path: rf.Path,
					Findings: []DesktopFinding{{
						Category: "io",
						Severity: string(check.SeverityMajor),
						Message:  rerr.Error(),
					}},
				})
				continue
			}

			var fileFindings []DesktopFinding
			// checkErr records a checker that could not RUN, as distinct from one
			// that ran and found nothing. It is reported as a finding on the file,
			// never dropped: a panel showing a clean file for checks that never
			// executed is the "operation failed, success reported" defect this
			// loop's own read-failure branch above already guards against.
			var checkErr error

			// Voice vocabulary — source-side, against the profile AND the
			// vocabulary governing THIS file's point. Passing no terminology
			// reports what the profile forbids and stays silent about every
			// term the project itself retired. Runs once per file (independent
			// of how many target languages are checked).
			profile := points.at(ctx, rf.Collection, rf.Relative)
			if profile != nil {
				vocab := coretools.NewVoiceVocabCheckTool(
					profile, points.termsAt(ctx, rf.Collection, rf.Relative),
				).InSourceLocale(pctx.SourceLocale)
				for _, b := range sourceBlocks {
					if cerr := host.RunCheckTool(ctx, vocab, b); cerr != nil {
						checkErr = fmt.Errorf("voice vocabulary: %w", cerr)
						break
					}
					if ann, ok := model.AnnoAs[*coreprofile.VoiceAnnotation](b, "voice"); ok {
						for _, f := range ann.Findings {
							fileFindings = append(fileFindings, toDesktopFinding(f, b, "source", sourceLang, filePoint))
							allFindings = append(allFindings, f)
						}
					}
				}
			}

			// Target-side checks, once per filtered language.
			for _, lang := range filter.Languages {
				if checkErr != nil {
					break
				}
				if lang == "" {
					continue
				}
				targetLoc := model.LocaleID(lang)
				tgtPath := a.resolveTargetPath(rf, op, lang)
				if tgtPath == "" {
					continue
				}
				if _, serr := os.Stat(tgtPath); serr != nil {
					continue
				}
				// Read source blocks fresh per language — OverlayTargets mutates
				// them (a cached file replays cheaply).
				passBlocks, perr := capp.ReadBlocksForCheck(ctx, rf.Path, rf.Format, fmtCfg, sourceLang)
				if perr != nil {
					continue
				}
				// The target is the translated rendering of this source file, so
				// it carries the source's reader binding.
				targetBlocks, terr := capp.ReadBlocksForCheck(ctx, tgtPath, rf.Format, fmtCfg, sourceLang)
				if terr != nil {
					continue
				}
				host.OverlayTargets(passBlocks, targetBlocks, targetLoc)

				// Placeholder integrity.
				placeholder := coretools.NewPlaceholderCheckTool(coretools.NewPlaceholderCheckConfig(targetLoc))
				for _, b := range passBlocks {
					if cerr := host.RunCheckTool(ctx, placeholder, b); cerr != nil {
						checkErr = fmt.Errorf("placeholder (%s): %w", lang, cerr)
						break
					}
					for _, f := range host.FindingsFromBlock(b, true) {
						fileFindings = append(fileFindings, toDesktopFinding(f, b, "target", lang, filePoint))
						allFindings = append(allFindings, f)
					}
				}

				// Do-not-translate: only when terms are configured.
				if checkErr == nil && len(dntTerms) > 0 {
					dntCfg := coretools.NewDNTCheckConfig(targetLoc)
					dntCfg.Terms = dntTerms
					dnt := coretools.NewDNTCheckTool(dntCfg)
					for _, b := range passBlocks {
						if cerr := host.RunCheckTool(ctx, dnt, b); cerr != nil {
							checkErr = fmt.Errorf("do-not-translate (%s): %w", lang, cerr)
							break
						}
						for _, f := range host.FindingsFromBlock(b, true) {
							fileFindings = append(fileFindings, toDesktopFinding(f, b, "target", lang, filePoint))
							allFindings = append(allFindings, f)
						}
					}
				}
			}

			if checkErr != nil {
				fileFindings = append(fileFindings, DesktopFinding{
					Category: "check",
					Severity: string(check.SeverityMajor),
					Message:  "checks did not complete: " + checkErr.Error(),
				})
			}

			sortDesktopFindings(fileFindings)
			files = append(files, CheckFileResult{Path: rf.Path, Findings: fileFindings})
		}
		return nil
	})
	if runErr != nil {
		return nil, runErr
	}

	score := check.CalculateScore(allFindings).Overall
	critical := 0
	for _, f := range allFindings {
		if f.Severity == check.SeverityCritical {
			critical++
		}
	}

	return &CheckRunResult{
		Pass:  critical == 0,
		Score: score,
		Files: files,
	}, nil
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

// pointResolver answers what governs a file — the voice profile and the
// vocabulary — at that file's own point, once per point.
//
// A project binds both per profile, so the pair governing a file is the pair at
// its point. Resolving once for the project and applying it everywhere checks
// half a two-profile repository against the wrong rules, and reports each
// finding against a point it did not use.
//
// Both resolutions read from disk, so each point is resolved once and reused.
// The key is the resolved channel reference rather than the file: every file at
// a point shares its governance, which is what makes the point the unit.
type pointResolver struct {
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

// key names the point a file resolves to. A point that fails to resolve keys as
// the project's own, which is where its content is governed.
func (v *pointResolver) key(pt project.GovernancePoint) string {
	if rc, err := v.proj.ResolveGovernanceFor(pt); err == nil {
		return rc.Ref().String()
	}
	return ""
}

// termsAt returns the vocabulary the project decided for this file's point —
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
	if !v.held {
		v.app.checksMu.Lock()
		defer v.app.checksMu.Unlock()
	}
	cmd, err := v.app.contextCommand(ctx, v.op)
	if err != nil {
		return nil
	}
	tb, terr := v.app.checksCLI().ProjectTermsForFile(
		ctx, cmd, filepath.Join(v.root, filepath.FromSlash(relPath)),
	)
	if terr != nil {
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
	p, _, ok, err := v.app.checksCLI().ResolveVoiceProfile(
		ctx, v.proj, v.root, host.VoiceResolveOptions{Point: pt},
	)
	if err != nil || !ok {
		return nil
	}
	return p
}

// ApplyCheckFix applies a single finding's structured replacement to a block in
// a content file — the Checks panel's one-click fix. It reads the file through
// its format reader, finds the block by ID, replaces the first occurrence of
// original with replacement in the requested field (source or target), and
// writes the file back through the format writer.
//
// Safety: the edit is only applied when the field's content is a single plain
// text run (no inline markup / multiple runs). A plain substring replace over
// runs that carry placeholders or paired codes could silently corrupt the
// markup, so in that case the fix is refused with a clear error and the file is
// left untouched.
func (a *App) ApplyCheckFix(tabID, filePath, blockID, field, original, replacement string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}
	if blockID == "" {
		return errors.New("a block id is required to apply a fix")
	}
	if original == "" || replacement == "" {
		return errors.New("both the original text and its replacement are required")
	}
	if field != "source" && field != "target" {
		return fmt.Errorf("field must be %q or %q, got %q", "source", "target", field)
	}

	pctx := project.NewProjectContext(op.Project, op.Path)
	sourceLang := string(pctx.SourceLocale)

	// The format the recipe declares for this item, not the one its extension
	// suggests. This rewrites the user's file: reading a `.md` item the recipe
	// binds to mdx with the markdown reader turns an `import { … }` line into a
	// paragraph, and writing that back is how a fix corrupts a page. Detection is
	// the fallback for a file the project does not declare as content.
	fmtName := ""
	item := (*project.ContentItem)(nil)
	if rf := a.resolvedFileFor(pctx, filePath); rf != nil {
		fmtName, item = rf.Format, rf.Item
	}
	if fmtName == "" {
		fmtName = pctx.DetectFormat(a.formatReg, filePath)
	}
	if fmtName == "" {
		return fmt.Errorf("could not detect a format for %q", filepath.Base(filePath))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The fix is in-place: read parts, mutate the one matching block, write the
	// same parts back. We mutate the live *model.Block carried by the part so
	// the writer reproduces the file faithfully (skeleton, ordering, untouched
	// blocks all preserved).
	//
	// A content file is read monolingually here, so its text — whether it is the
	// source file (field "source") or a translated target file (field "target")
	// — lands in the block's source runs. The fix therefore always replaces in
	// the block's own text runs; `field` records which side the finding was on
	// (it determines which file path the panel passes), not a different setter.
	applied := false
	var applyErr error
	transform := func(b *model.Block) {
		if b.ID != blockID {
			return
		}
		runs := b.SourceRuns()
		if !isSinglePlainTextRun(runs) {
			applyErr = fmt.Errorf("manual fix needed (formatted content): block %q has inline markup or multiple runs, so an automatic replace could corrupt it", blockID)
			return
		}
		text := model.RunsText(runs)
		if !strings.Contains(text, original) {
			applyErr = fmt.Errorf("the original text %q is no longer present in block %q (it may already be fixed)", original, blockID)
			return
		}
		b.SetSourceText(strings.Replace(text, original, replacement, 1))
		applied = true
	}

	if err := a.rewriteFile(ctx, filePath, fmtName, sourceLang, pctx, item, transform); err != nil {
		return err
	}
	if applyErr != nil {
		return applyErr
	}
	if !applied {
		return fmt.Errorf("block %q not found in %q", blockID, filepath.Base(filePath))
	}
	return nil
}

// rewriteFile reads filePath through its format reader, runs transform over
// every block (in stream order), then writes the parts back through the format
// writer atomically (temp file + rename). Non-block parts pass through
// unchanged.
func (a *App) rewriteFile(ctx context.Context, filePath, fmtName, sourceLang string, pctx *project.ProjectContext, item *project.ContentItem, transform func(*model.Block)) error {
	reader, err := a.formatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return fmt.Errorf("no reader for %q: %w", fmtName, err)
	}
	defer reader.Close()

	writer, err := a.formatReg.NewWriter(registry.FormatID(fmtName))
	if err != nil {
		return fmt.Errorf("no writer for %q: %w", fmtName, err)
	}
	defer writer.Close()

	// Both halves of the round-trip carry the recipe's configuration for this
	// item: a rewrite that reads under one configuration and writes under
	// another renumbers the document it is replacing.
	if pctx != nil {
		if cerr := pctx.ConfigureReaderFor(reader, fmtName, item); cerr != nil {
			return fmt.Errorf("configure reader for %s: %w", filepath.Base(filePath), cerr)
		}
		if cerr := pctx.ConfigureWriterFor(writer, fmtName, item); cerr != nil {
			return fmt.Errorf("configure writer for %s: %w", filepath.Base(filePath), cerr)
		}
	}

	// Wire skeleton store when both sides support it so the writer reproduces
	// the original structure (whitespace, key order, untouched values). A store
	// that cannot be created fails the rewrite: this replaces the user's file, so
	// degrading silently would reformat every untouched value in it while the UI
	// reported the fix applied.
	skeletonStore, skelErr := format.NewWiredSkeleton(reader, writer)
	if skelErr != nil {
		return fmt.Errorf("cannot rewrite %s: %w", filepath.Base(filePath), skelErr)
	}
	if skeletonStore != nil {
		defer skeletonStore.Close()
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(filePath), err)
	}

	doc := &model.RawDocument{
		URI:          filePath,
		SourceLocale: model.LocaleID(sourceLang),
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return fmt.Errorf("open %q: %w", filepath.Base(filePath), err)
	}

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return fmt.Errorf("read %q: %w", filepath.Base(filePath), pr.Error)
		}
		if pr.Part != nil && pr.Part.Type == model.PartBlock {
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				transform(b)
			}
		}
		parts = append(parts, pr.Part)
	}
	reader.Close()

	// Write back atomically.
	tmp, err := os.CreateTemp(filepath.Dir(filePath), ".kapi-fix-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if err := writer.SetOutputWriter(tmp); err != nil {
		cleanup()
		return fmt.Errorf("set output: %w", err)
	}
	// Skeleton-driven writers (OpenXML) need the original bytes to rebuild.
	if ocs, ok := writer.(format.OriginalContentSetter); ok {
		ocs.SetOriginalContent(content)
	}

	in := make(chan *model.Part, len(parts)+1)
	for _, p := range parts {
		in <- p
	}
	close(in)
	if err := writer.Write(ctx, in); err != nil {
		cleanup()
		return fmt.Errorf("write %q: %w", filepath.Base(filePath), err)
	}
	if err := writer.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close writer: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("finalize %q: %w", filepath.Base(filePath), err)
	}
	return nil
}

// --- helpers ---------------------------------------------------------------

// resolveTargetPath derives the on-disk path of the translated file for a
// source file and target language, using the content item's Target template
// (e.g. "locales/{lang}.json" or "output/{lang}/*"). Template expansion goes
// through the shared core resolver (project.ResolveTargetPath) — the same one
// the runner uses to write outputs — so checks probe exactly the paths the
// runner produces. Returns "" when the item declares no target template.
func (a *App) resolveTargetPath(rf project.ResolvedFile, op *openProject, targetLang string) string {
	if rf.Item == nil || rf.Item.Target == "" {
		return ""
	}
	root := filepath.Dir(op.Path)
	relSlash := filepath.ToSlash(rf.Relative)
	return filepath.Join(root, project.ResolveTargetPath(rf.Item.Path, rf.Item.Base, rf.Item.Target, relSlash, targetLang))
}

// resolveProjectDNTTerms collects do-not-translate terms from the project's
// auto-opened terms. A concept opts in via a `do_not_translate` property
// (the term must survive verbatim); its source-locale term texts are returned.
// Returns nil when there is no terms or no opted-in concepts — the
// do-not-translate check is then skipped, matching the CLI's flag-driven default.
func (a *App) resolveProjectDNTTerms(ctx context.Context, op *openProject, sourceLang string) []string {
	if op.tbHandle == "" {
		return nil
	}
	tb, ok := a.tbHandles.Get(op.tbHandle)
	if !ok || tb == nil {
		return nil
	}
	srcLoc := model.LocaleID(sourceLang)
	seen := make(map[string]bool)
	var terms []string
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return nil
	}
	for _, c := range concepts {
		if !dntConcept(c.Properties) {
			continue
		}
		for _, t := range c.Terms {
			if (srcLoc == "" || t.Locale == srcLoc) && t.Text != "" && !seen[t.Text] {
				seen[t.Text] = true
				terms = append(terms, t.Text)
			}
		}
	}
	sort.Strings(terms)
	return terms
}

// dntConcept reports whether a concept's properties mark it do-not-translate.
func dntConcept(props map[string]string) bool {
	if props == nil {
		return false
	}
	for _, k := range []string{"do_not_translate", "dnt", "no_translate"} {
		switch strings.ToLower(props[k]) {
		case "true", "1", "yes":
			return true
		}
	}
	return false
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
		Severity:     string(f.Severity),
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
	}
	df.Fixable = replacement != "" && b.ID != "" && f.OriginalText != ""
	return df
}

// findingRule names the rule that fired.
//
// The checker states it on Check; where it does not, the term or pattern the
// rule is about identifies it, and the category is the last resort. A finding
// that named nothing would leave a reader with a complaint and nowhere to go.
func findingRule(f check.Finding) string {
	if f.Check != "" {
		return f.Check
	}
	if f.Metadata != nil {
		for _, key := range []string{"rule", "term", "pattern"} {
			if v := f.Metadata[key]; v != "" {
				return v
			}
		}
	}
	if f.OriginalText != "" {
		return f.OriginalText
	}
	return f.Category
}

// pointRef renders the point a finding is scoped to, as the explorer addresses
// it. The project's own point renders empty rather than as a guessed name.
func pointRef(p ContextPointDTO) string {
	return project.ChannelRef{Profile: p.Profile, Channel: p.Channel}.String()
}

// isSinglePlainTextRun reports whether runs is exactly one TextRun — the only
// shape where a plain substring replace is structurally safe (no placeholders
// or paired inline codes to corrupt).
func isSinglePlainTextRun(runs []model.Run) bool {
	return len(runs) == 1 && runs[0].Text != nil
}

// checkSeverityRank orders findings critical → neutral for stable panel output.
var checkSeverityRank = map[string]int{"critical": 0, "major": 1, "minor": 2, "neutral": 3}

func sortDesktopFindings(fs []DesktopFinding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if checkSeverityRank[fs[i].Severity] != checkSeverityRank[fs[j].Severity] {
			return checkSeverityRank[fs[i].Severity] < checkSeverityRank[fs[j].Severity]
		}
		return fs[i].Category < fs[j].Category
	})
}
