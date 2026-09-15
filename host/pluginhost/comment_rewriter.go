package pluginhost

import (
	"bytes"
	"strings"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// daemonCommentRewriter writes the comments of a language whose manifest
// declares a rewrite (manifest.CommentRewrite).
//
// The host renders the text into the comment's layout, read from the file's
// bytes through the markers the manifest declares, and comment.Rewrite splices
// it in and holds the result to comment.Contain, which has the plugin locate
// the rewritten file through LocateComments. The renderer runs in the host and
// the reading runs in the plugin's grammar, so the check a rewrite is held to
// shares no code with what rendered it, and the plugin needs no call of its
// own for writing.
type daemonCommentRewriter struct {
	*daemonCommentProvider
}

var _ comment.Rewriter = (*daemonCommentRewriter)(nil)

// commentLayout is the layout of one comment, delimited or made of line
// comments.
type commentLayout interface {
	Text() string
	Render(lines []string) ([]byte, error)
}

// layout reads the layout of c, located in src: a delimited comment's through
// the block delimiters it opens with, and a line comment's through the
// language's line markers.
func (p *daemonCommentRewriter) layout(src []byte, c comment.Comment) (commentLayout, error) {
	m := p.markers()
	if c.Style != comment.StyleBlock {
		return comment.ParseLineLayout(src, c, m)
	}
	span := src[c.Start:c.End]
	for _, b := range m.Block {
		if !bytes.HasPrefix(span, []byte(b.Open)) {
			continue
		}
		lineStart := bytes.LastIndexByte(src[:c.Start], '\n') + 1
		indent := string(src[lineStart:c.Start])
		if strings.TrimLeft(indent, " \t") != "" {
			indent = ""
		}
		return comment.ParseLayout(span, indent, b)
	}
	return nil, &comment.Refusal{Reason: comment.RefusedLayout, Detail: "the comment opens with none of the delimiters the plugin declares for " + p.lang.Language}
}

// Prose implements comment.Rewriter: the comment's text as its layout reads it.
func (p *daemonCommentRewriter) Prose(src []byte, c comment.Comment) (string, error) {
	l, err := p.layout(src, c)
	if err != nil {
		return "", err
	}
	return l.Text(), nil
}

// Render implements comment.Rewriter. Each line of text is one line of the
// comment, laid out as the comment already lays out its lines. The text is not
// reflowed, and RenderOptions.Width does not apply: the formatters these
// languages use keep a comment's line breaks as they are.
func (p *daemonCommentRewriter) Render(_ string, src []byte, c comment.Comment, text string, _ comment.RenderOptions) ([]byte, error) {
	l, err := p.layout(src, c)
	if err != nil {
		return nil, err
	}
	lines, err := comment.TextLines(text)
	if err != nil {
		return nil, err
	}
	return l.Render(lines)
}

// Rewrite returns the rewrite declaration the manifest makes for the language,
// which names the formatters the host holds a rewrite to.
func (p *daemonCommentRewriter) Rewrite() *manifest.CommentRewrite {
	return p.lang.Rewrite
}

// RewriteCanary implements comment.Rewriter with the canary the manifest
// declares.
func (p *daemonCommentRewriter) RewriteCanary() comment.RewriteCanary {
	c := p.lang.Rewrite.Canary
	return comment.RewriteCanary{
		Name:       c.Name,
		Source:     []byte(c.Source),
		Block:      c.Block,
		Refused:    c.Refused,
		Delimited:  c.Delimited,
		Terminator: c.Terminator,
	}
}
