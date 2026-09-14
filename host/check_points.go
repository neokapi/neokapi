package host

import (
	"context"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// atPoint is the governance a check holds blocks to at one point.
type atPoint struct {
	// point is where the project resolved it, nil outside a project.
	point        *check.Point
	profile      *profile.VoiceProfile
	voiceContext check.VoiceContext
	terms        terms.Terminology
}

// fileGovernance is what one file's blocks are held to: the governance at the
// file's own point, and at the point its comments sit at.
type fileGovernance struct {
	content  atPoint
	comments atPoint
}

// apart reports that the file's comments sit at a point of their own.
func (g fileGovernance) apart() bool {
	a, b := g.content.point, g.comments.point
	if a == nil || b == nil {
		return false
	}
	return a.Profile != b.Profile || a.Channel != b.Channel
}

// governFile resolves the governance at both of a file's points. voice is nil
// when the invocation named a profile, and fixed then carries that profile to
// both points. vocab is nil when the caller supplied terms in fixed, which then
// govern both points too.
func (a *App) governFile(ctx context.Context, voice *checkVoice, vocab *checkTerms, file string, fixed atPoint) (fileGovernance, error) {
	var g fileGovernance
	for _, comments := range []bool{false, true} {
		at := fixed
		var err error
		if voice != nil {
			if comments {
				at.profile, at.voiceContext, err = voice.forComments(ctx, file)
			} else {
				at.profile, at.voiceContext, err = voice.forFile(ctx, file)
			}
			if err != nil {
				return fileGovernance{}, err
			}
		}
		if vocab != nil && vocab.proj != nil {
			point := a.governancePointForFile(vocab.root, file)
			point.Comments = comments
			if at.terms, err = vocab.forPoint(ctx, point, file); err != nil {
				return fileGovernance{}, err
			}
			rc, err := vocab.resolve(point)
			if err != nil {
				return fileGovernance{}, err
			}
			at.point = &check.Point{Profile: rc.Profile, Channel: rc.Channel}
		}
		if comments {
			g.comments = at
		} else {
			g.content = at
		}
	}
	if g.apart() {
		g.comments.point.Comments = true
	}
	return g, nil
}

// govern holds the options to the governance resolved for one file: the file's
// own point, and the comments' point when they sit apart.
func (o checkRunOptions) govern(g fileGovernance) checkRunOptions {
	o.profile, o.voiceContext, o.terms, o.point = g.content.profile, g.content.voiceContext, g.content.terms, g.content.point
	o.comments = nil
	if g.apart() {
		comments := g.comments
		o.comments = &comments
	}
	return o
}

// here is the governance at the file's own point.
func (o checkRunOptions) here() atPoint {
	return atPoint{point: o.point, profile: o.profile, voiceContext: o.voiceContext, terms: o.terms}
}

// pointGroup is the blocks of one file held to one point.
type pointGroup struct {
	blocks []*model.Block
	// doc is the document the group's document-scope rules read.
	doc []*model.Block
	at  atPoint
	// apart reports that the file's comments sit at a point of their own, so
	// the file has a group for each point.
	apart bool
}

// pointGroups divides a file's blocks by the point each is checked at. A file
// whose comments share its point is one group. Otherwise the comments form one
// group and the other content another, and a group holding no block is left
// out.
func (o checkRunOptions) pointGroups(blocks, doc []*model.Block) []pointGroup {
	if o.comments == nil {
		return []pointGroup{{blocks: blocks, doc: doc, at: o.here()}}
	}
	content, comments := splitCommentBlocks(blocks)
	contentDoc, commentsDoc := splitCommentBlocks(doc)
	var groups []pointGroup
	if len(content) > 0 {
		groups = append(groups, pointGroup{blocks: content, doc: contentDoc, at: o.here(), apart: true})
	}
	if len(comments) > 0 {
		groups = append(groups, pointGroup{blocks: comments, doc: commentsDoc, at: *o.comments, apart: true})
	}
	return groups
}

// pointOf returns the point a block is checked at.
func (o checkRunOptions) pointOf(b *model.Block) *check.Point {
	if o.comments != nil && comment.IsBlock(b) {
		return o.comments.point
	}
	return o.point
}

// stampPoints gives each diagnostic that has no point the point of the block
// it sits in, or the file's own point when it sits in none of blocks.
func (o checkRunOptions) stampPoints(diags []check.Diagnostic, blocks []*model.Block) {
	byKey := make(map[string]*check.Point, len(blocks))
	for _, b := range blocks {
		byKey[blockKey(b)] = o.pointOf(b)
	}
	for i := range diags {
		if diags[i].Point != nil {
			continue
		}
		point := o.point
		if p, ok := byKey[diags[i].Location.Block]; ok && diags[i].Location.Block != "" {
			point = p
		}
		diags[i].Point = clonePoint(point)
	}
}

// recordContexts records the guidance applied to one file's blocks: the file's
// own point for its other content, and the comments' point for its comments
// when they sit apart. A point that holds none of blocks is not recorded,
// unless it is the file's only one.
func (e *checkExecution) recordContexts(file, destination string, opts checkRunOptions, blocks []*model.Block) {
	if e == nil {
		return
	}
	content, comments := splitCommentBlocks(blocks)
	if opts.comments == nil || len(content) > 0 || len(comments) == 0 {
		e.recordContext(file, destination, opts)
	}
	if opts.comments != nil && len(comments) > 0 {
		at := opts
		at.profile, at.voiceContext, at.terms, at.point = opts.comments.profile, opts.comments.voiceContext, opts.comments.terms, opts.comments.point
		at.comments = nil
		e.recordContext(file, destination, at)
	}
}

// splitCommentBlocks separates the blocks a comment layer built from the rest,
// keeping each group in its order.
func splitCommentBlocks(blocks []*model.Block) (content, comments []*model.Block) {
	for _, b := range blocks {
		if comment.IsBlock(b) {
			comments = append(comments, b)
		} else {
			content = append(content, b)
		}
	}
	return content, comments
}

func clonePoint(p *check.Point) *check.Point {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

// analyzerCount is how many analyzers the execution has recorded.
func (e *checkExecution) analyzerCount() int {
	if e == nil {
		return 0
	}
	return len(e.Analyzers)
}

// pointAnalyzers sets point on the analyzers recorded since from.
func (e *checkExecution) pointAnalyzers(from int, point *check.Point) {
	if e == nil || point == nil {
		return
	}
	for i := from; i < len(e.Analyzers); i++ {
		e.Analyzers[i].Point = clonePoint(point)
	}
}
