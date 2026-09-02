package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// The Review page's source lane.
//
// Target review asks "is this translation right for that source". Source review
// asks the question underneath it, once for every language: is the source right
// at all. The two are the same act on different content, so they are two lanes
// of one page rather than two pages.

// GetSourceUnitContext returns the review model for one SOURCE unit: the point
// its file sits at and the blocks either side of it in document order.
//
// Both lanes of the Review page render one model. Approving source wording
// without seeing the voice it is approved against is the target defect in
// reverse, so the lane that judges the source reads the same point the lane
// that judges a translation reads.
//
// The locale under review is the source language: the source content is the
// content, and the neighbours therefore carry source alone.
func (a *App) GetSourceUnitContext(tabID, file, key string) (*host.ReviewContext, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return nil, errors.New("project has no recipe loaded")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pctx := project.NewProjectContext(op.Project, op.Path)
	sourceLang := string(pctx.SourceLocale)
	path := inspectAbsPath(op, file)
	rf := a.resolvedFileFor(pctx, path)
	if rf == nil {
		return nil, fmt.Errorf("file %q is not part of this project's content", file)
	}

	blocks, err := a.readBlocksForChecks(ctx, rf.Path, rf.Format,
		pctx.FormatConfigFor(rf.Format, rf.Item), sourceLang)
	if err != nil {
		return nil, err
	}

	cmd, cerr := a.contextCommand(ctx, op)
	if cerr != nil {
		return nil, cerr
	}
	return a.hostEngine().AssembleReviewContext(ctx, host.ReviewContextRequest{
		Cmd:        cmd,
		Root:       filepath.Dir(op.Path),
		SourcePath: rf.Path,
		Collection: rf.Collection,
		Locale:     sourceLang,
		SourceLang: sourceLang,
		Blocks:     blocks,
		Key:        key,
	}), nil
}

// ApproveSourceUnit records a human approval of one source unit, lifting it to
// the top of the source ladder. The record binds to the wording approved, so a
// later edit to that sentence drops the approval rather than letting it stand
// over text nobody read.
func (a *App) ApproveSourceUnit(tabID, file, key string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return errors.New("project has no recipe loaded")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := a.hostEngine().ApproveSourceUnit(ctx, op.Path,
		string(op.Project.Defaults.SourceLanguage), host.SourceUnitRef{File: file, Key: key})
	return err
}

// UpdateSourceText rewrites one source unit and reports the locales whose
// translation the next run will re-draft.
//
// The translations stay where they are. Each one renders the sentence that was
// there a moment ago, and the loop knows that: `kapi up` records the source it
// translated for every target it writes, so the rewrite reads as drift on the
// next run, the unit is re-drafted against the wording the project has now, and
// the run reports how many it re-drafted. Emptying them here destroyed the
// previous translation, which is what the content memory recycles from and what
// a reviewer compares the new draft against.
func (a *App) UpdateSourceText(tabID, file, key, text string) ([]string, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return nil, errors.New("project has no recipe loaded")
	}
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("the edited source is empty — a source unit with no text has nothing to translate")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pctx := project.NewProjectContext(op.Project, op.Path)
	resolved, err := pctx.ResolveContent(a.formatReg)
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}
	root := filepath.Dir(op.Path)

	var rf project.ResolvedFile
	found := false
	for _, cand := range resolved {
		rel, rerr := filepath.Rel(root, cand.Path)
		if rerr == nil && filepath.ToSlash(rel) == file {
			rf, found = cand, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("source file %q is not content this project declares", file)
	}

	sourceLang := string(pctx.SourceLocale)
	fmtName := rf.Format
	if fmtName == "" {
		fmtName = pctx.DetectFormat(a.formatReg, rf.Path)
	}
	if fmtName == "" {
		return nil, fmt.Errorf("could not detect a format for %q", filepath.Base(rf.Path))
	}

	applied := false
	var applyErr error
	rewrite := func(b *model.Block) {
		if convergence.BlockKey(b) != key {
			return
		}
		if !isSinglePlainTextRun(b.SourceRuns()) {
			applyErr = fmt.Errorf("manual edit needed (formatted content): unit %q has inline markup or multiple runs, so a plain-text rewrite could corrupt it", key)
			return
		}
		b.SetSourceText(text)
		applied = true
	}
	if err := a.rewriteFile(ctx, rf.Path, fmtName, sourceLang, pctx, rf.Item, rewrite); err != nil {
		return nil, err
	}
	if applyErr != nil {
		return nil, applyErr
	}
	if !applied {
		return nil, fmt.Errorf("unit %q not found in %q", key, filepath.Base(rf.Path))
	}

	return a.localesAwaitingRedraft(ctx, op, pctx, rf, fmtName, key), nil
}

// localesAwaitingRedraft names the target locales holding a translation of one
// unit, so the source lane can say which languages the next run will re-draft.
//
// It reads and writes nothing else. Failures are skipped rather than aborting:
// the source is already rewritten by the time this runs, and one unreadable
// locale file is a reason to say less, never a reason to fail the edit.
func (a *App) localesAwaitingRedraft(ctx context.Context, op *openProject, pctx *project.ProjectContext, rf project.ResolvedFile, fmtName, key string) []string {
	if rf.Item == nil || rf.Item.Target == "" {
		return nil
	}
	root := filepath.Dir(op.Path)
	localeFormat := op.Project.Defaults.LocaleFormat
	sourceLang := string(pctx.SourceLocale)
	fmtCfg := pctx.FormatConfigFor(rf.Format, rf.Item)

	var pending []string
	for _, loc := range op.Project.Defaults.TargetLanguages {
		lang := string(loc)
		tgt := expandReviewTargetPath(rf, localeFormat, lang, root)
		if tgt == "" || tgt == rf.Path {
			continue
		}
		if _, serr := os.Stat(tgt); serr != nil {
			continue // nothing translated for this locale yet
		}
		// A target file is read monolingually: its translated text lives in each
		// block's SOURCE runs, the same convention UpdateReviewTarget relies on.
		blocks, berr := a.readBlocksForChecks(ctx, tgt, fmtName, fmtCfg, sourceLang)
		if berr != nil {
			a.logger.Printf("source edit: could not read %s to report the pending re-draft: %v", tgt, berr)
			continue
		}
		for _, b := range blocks {
			if convergence.BlockKey(b) == key && model.RunsHaveContent(b.SourceRuns()) {
				pending = append(pending, lang)
				break
			}
		}
	}
	slices.Sort(pending)
	return pending
}
