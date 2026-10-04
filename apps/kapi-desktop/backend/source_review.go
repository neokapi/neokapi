package backend

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// The Review page's source rows.
//
// Target review asks "is this translation right for that source". Source review
// asks the question underneath it, once for every language: is the source right
// at all. The two are the same act on different content, so both kinds of row
// list in one queue. A source row is edited and approved through the change
// service (Apply): set_content and decide, each with the revision the pane read.

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
	if rf.CommentsOnly() {
		return nil, fmt.Errorf("file %q is declared for its comments alone, so it has no source block", file)
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
