package host

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/segment"
)

// The check family and analyzers of the comment limits a voice profile sets.
// A finding's rule is `comment.<category>`, such as `comment.sentence-length`.
const (
	commentCheck            = "comment"
	commentSentenceAnalyzer = "comment.sentence-length"
	commentLengthAnalyzer   = "comment.length"
)

// sentenceBreak builds the sentence break the sentence-length check reads. It
// is a variable so a test can take the engine away and show that the check then
// does not run.
var sentenceBreak = func() (segment.Segmenter, error) {
	return segment.Build("uax29", segment.BaseConfig{}, nil)
}

// commentSentenceFindings and commentLengthFindings are the checks the comment
// analyzers run over real comments and over their canaries. They are variables
// so a test can put a broken check in place and show that its canary
// invalidates the run.
var (
	commentSentenceFindings = check.CommentSentenceFindings
	commentLengthFindings   = check.CommentLengthFindings
)

// checkCommentLimits holds the comments among blocks, all at one point, to the
// comment limits the voice profile there sets, and records an analyzer for each
// limit with canaries built from the limits in force. Blocks that hold no
// comment record nothing, and a profile that sets no comment limits records
// each analyzer as not requested. A build without the sentence break records
// the sentence-length analyzer as required and not run, so the check does not
// pass on it.
func (a *App) checkCommentLimits(ctx context.Context, blocks []*model.Block, p *profile.VoiceProfile, file string, execution *checkExecution) ([]check.Diagnostic, error) {
	var comments []*model.Block
	for _, b := range blocks {
		if comment.IsBlock(b) {
			comments = append(comments, b)
		}
	}
	if len(comments) == 0 {
		return nil, nil
	}
	if p == nil || p.Style.Comments == nil {
		const reason = "The voice profile at this point sets no comment limits."
		execution.skipped(commentSentenceAnalyzer, file, reason)
		execution.skipped(commentLengthAnalyzer, file, reason)
		return nil, nil
	}
	limits := p.Style.Comments.Limits()
	loc := model.LocaleID(a.SourceLocale())
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
	return diags, nil
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
