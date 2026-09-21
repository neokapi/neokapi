package host

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/golang"
	"github.com/neokapi/neokapi/core/diffscope"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
)

// The check family and analyzers of the comment limits a voice profile sets.
// A finding's rule is `comment.<category>`, such as `comment.sentence-length`.
const (
	commentCheck            = "comment"
	commentSentenceAnalyzer = "comment.sentence-length"
	commentLengthAnalyzer   = "comment.length"
	commentDensityAnalyzer  = "comment.density"
)

// sentenceBreak builds the sentence break the sentence-length check reads. It
// is a variable so a test can take the engine away and show that the check then
// does not run.
var sentenceBreak = func() (segment.Segmenter, error) {
	return segment.Build("uax29", segment.BaseConfig{}, nil)
}

// The checks the comment analyzers run over real comments and over their
// canaries. They are variables so a test can put a broken check in place and
// show that its canary invalidates the run.
var (
	commentSentenceFindings = check.CommentSentenceFindings
	commentLengthFindings   = check.CommentLengthFindings
	commentDensityFindings  = check.CommentDensityFindings
	lineKinds               = func(f *comment.File, src []byte) []comment.LineKind { return f.LineKinds(src) }
)

// commentChange is a change to one file, as the density check reads it.
type commentChange struct {
	// kinds classifies each line of the file after the change, as its comment
	// layer reads it.
	kinds []comment.LineKind
	// added are the lines the change added.
	added []format.LineRange
}

// addedLines returns the lines changes added, leaving out each deletion, which
// adds none.
func addedLines(changes []diffscope.Change) []format.LineRange {
	var out []format.LineRange
	for _, c := range changes {
		if !c.Deletion {
			out = append(out, c.Lines)
		}
	}
	return out
}

// checkCommentLimits holds the comments among blocks, all at the point at, to
// the comment limits the voice profile there sets, and records an analyzer for
// each limit with canaries built from the limits in force. Blocks that hold no
// comment record nothing, and a profile that sets no comment limits records
// each analyzer as not requested. A build without the sentence break records
// the sentence-length analyzer as required and not run, so the check does not
// pass on it. Density reads opts.change, the file's change in a diff-scoped
// check; with no change it is unsupported.
func (a *App) checkCommentLimits(ctx context.Context, blocks []*model.Block, at atPoint, file string, opts checkRunOptions) ([]check.Diagnostic, error) {
	change, execution := opts.change, opts.execution
	var comments []*model.Block
	for _, b := range blocks {
		if comment.IsBlock(b) {
			comments = append(comments, b)
		}
	}
	if len(comments) == 0 {
		return nil, nil
	}
	if at.profile == nil || at.profile.Style.Comments == nil {
		const reason = "The voice profile at this point sets no comment limits."
		for _, id := range []string{commentSentenceAnalyzer, commentLengthAnalyzer, commentDensityAnalyzer} {
			execution.skipped(id, file, reason)
		}
		return nil, nil
	}
	limits := at.profile.Style.Comments.Limits()
	loc := model.LocaleID(opts.source(a))
	var diags []check.Diagnostic
	add := func(b *model.Block, found []check.Finding) {
		for _, f := range found {
			diags = append(diags, check.DiagnosticFrom(f, commentCheck, check.Location{File: DisplayName(file), Block: blockKey(b)}))
		}
	}

	start := time.Now()
	seg, err := sentenceBreak()
	switch {
	case errors.Is(err, segment.ErrEngineUnavailable):
		execution.notChecked(commentSentenceAnalyzer, file,
			"Sentence length needs the UAX #29 sentence break, which this build does not include.")
	case err != nil:
		return nil, fmt.Errorf("%s %s: %w", commentSentenceAnalyzer, DisplayName(file), err)
	default:
		sentences := func(b *model.Block) ([]check.Finding, error) {
			return commentSentenceFindings(ctx, seg, b, limits, loc)
		}
		for _, b := range comments {
			found, err := sentences(b)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", commentSentenceAnalyzer, DisplayName(file), err)
			}
			add(b, found)
		}
		if err := recordCommentAnalyzer(execution, commentSentenceAnalyzer, file, len(diags), start, check.CommentSentenceCanaries(limits), sentences); err != nil {
			return nil, err
		}
	}

	start, before := time.Now(), len(diags)
	lengths := func(b *model.Block) ([]check.Finding, error) { return commentLengthFindings(b, limits), nil }
	for _, b := range comments {
		found, _ := lengths(b)
		add(b, found)
	}
	if err := recordCommentAnalyzer(execution, commentLengthAnalyzer, file, len(diags)-before, start, check.CommentLengthCanaries(limits), lengths); err != nil {
		return nil, err
	}

	if change == nil {
		execution.unsupported(commentDensityAnalyzer, file,
			"Comment density is a property of a change, and this check reads whole files. Scope the check to a diff to measure it.")
		return diags, nil
	}
	start, before = time.Now(), len(diags)
	lines := check.CountChangeLines(change.kinds, change.added)
	for _, f := range commentDensityFindings(lines, limits) {
		d := check.DiagnosticFrom(f, commentCheck, check.Location{
			File: DisplayName(file), Lines: &format.LineRange{First: lines.First, Last: lines.Last},
		})
		d.Point = clonePoint(at.point)
		diags = append(diags, d)
	}
	if err := recordCommentAnalyzer(execution, commentDensityAnalyzer, file, len(diags)-before, start,
		[]check.Canary{check.CommentDensityCanary(limits)}, densityProbe(limits)); err != nil {
		return nil, err
	}
	return diags, nil
}

// densityProbe evaluates the density canary: a Go file whose every line a
// change adds, its lines classified by the Go comment provider and counted by
// the same density check the real change went through.
func densityProbe(limits check.CommentLimits) func(*model.Block) ([]check.Finding, error) {
	return func(b *model.Block) ([]check.Finding, error) {
		src := []byte(model.RunsText(b.SourceRuns()))
		located, err := golang.Provider{}.Locate("canary.go", src)
		if err != nil {
			return nil, err
		}
		kinds := lineKinds(located, src)
		added := []format.LineRange{{First: 1, Last: len(kinds) - 1}}
		return commentDensityFindings(check.CountChangeLines(kinds, added), limits), nil
	}
}

// recordCommentAnalyzer gives a comment analyzer its canaries through the same
// check the comments went through, and records it as required. A nil execution
// records nothing and probes no canary.
func recordCommentAnalyzer(execution *checkExecution, id, file string, findings int, start time.Time, canaries []check.Canary, run func(*model.Block) ([]check.Finding, error)) error {
	if execution == nil {
		return nil
	}
	canary, err := check.Probe(canaries, "", run)
	if err != nil {
		return fmt.Errorf("%s %s: %w", id, DisplayName(file), err)
	}
	execution.completed(id, file, findings, start, canary, true)
	return nil
}
