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

// GetSourceQueue returns the source units awaiting authoring attention, narrowed
// to the project's Active Filter. Languages in the filter are ignored here: a
// source unit is the same content for every target, which is why the gate holds
// the whole fan-out rather than one language of it.
func (a *App) GetSourceQueue(tabID string, filter ProjectFilter) ([]host.SourceQueueItem, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return []host.SourceQueueItem{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	items, err := a.hostEngine().SourceQueue(ctx, op.Path, string(op.Project.Defaults.SourceLanguage))
	if err != nil {
		return nil, err
	}
	out := make([]host.SourceQueueItem, 0, len(items))
	for _, it := range items {
		if filter.FilesNarrowed() && !filter.MatchesFile(it.Collection, it.Relative) {
			continue
		}
		out = append(out, it)
	}
	return out, nil
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

// UpdateSourceText rewrites one source unit and clears that unit's translation
// in every target locale, so the next run re-drafts them.
//
// Clearing is the whole point, and it is not the obvious half. The converge flow
// translates with `skipMatched`, so a block that still carries a target is
// skipped: leaving the old translations in place would leave every language
// holding a translation of a sentence that no longer exists, and the loop would
// never notice. This is the local counterpart of the server's
// applySourceProposal, which clears targets for exactly the same reason.
//
// It returns the locales whose target was cleared.
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

	return a.clearTargetsForUnit(ctx, op, pctx, rf, fmtName, sourceLang, key), nil
}

// clearTargetsForUnit empties one unit's text in every target locale that has a
// file for it, returning the locales it cleared.
//
// Failures are collected as skips rather than aborting: the source is already
// rewritten by the time this runs, and refusing to clear the rest because one
// locale's file is unreadable would leave more stale translations behind, not
// fewer.
func (a *App) clearTargetsForUnit(ctx context.Context, op *openProject, pctx *project.ProjectContext, rf project.ResolvedFile, fmtName, sourceLang, key string) []string {
	if rf.Item == nil || rf.Item.Target == "" {
		return nil
	}
	root := filepath.Dir(op.Path)
	localeFormat := op.Project.Defaults.LocaleFormat

	var cleared []string
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
		emptied := false
		blank := func(b *model.Block) {
			if convergence.BlockKey(b) != key || !isSinglePlainTextRun(b.SourceRuns()) {
				return
			}
			b.SetSourceText("")
			emptied = true
		}
		if err := a.rewriteFile(ctx, tgt, fmtName, sourceLang, pctx, rf.Item, blank); err != nil {
			a.logger.Printf("source edit: could not clear %s in %s: %v", key, tgt, err)
			continue
		}
		if emptied {
			cleared = append(cleared, lang)
		}
	}
	slices.Sort(cleared)
	return cleared
}
