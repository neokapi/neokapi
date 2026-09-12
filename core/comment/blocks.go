package comment

import (
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// BlockType is the Type of every block built from a comment.
const BlockType = "comment"

// Block properties a comment block carries.
const (
	// PropLanguage names the language of the file the comment came from.
	PropLanguage = "comment.language"
	// PropStyle is the comment's Style.
	PropStyle = "comment.style"
	// PropDoc is "true" on a comment that documents a declaration.
	PropDoc = "comment.doc"
	// PropDeprecated is "true" on a comment carrying a deprecation marker.
	PropDeprecated = "comment.deprecated"
	// PropSpan and PropLines are the comment's place in the file, written
	// "start-end" and "first-last". They are derived locators, which move
	// whenever anything above the comment moves, so they are advisory and stay
	// out of the block's identity.
	PropSpan  = model.AdvisoryPropertyPrefix + "comment.span"
	PropLines = model.AdvisoryPropertyPrefix + "comment.lines"
)

// Blocks returns the file's comments as blocks, in file order.
//
// A block is named for what the comment sits on, so its name survives an edit
// above it: `func/Parse` stays `func/Parse` when a line is added at the top of
// the file. Two comments on one subject are told apart by an ordinal, and the
// id is the name, held unique within the file by model.IDBuilder.
func (f *File) Blocks() []*model.Block {
	var names model.NameBuilder
	var ids model.IDBuilder
	blocks := make([]*model.Block, 0, len(f.Comments))
	for i := range f.Comments {
		c := &f.Comments[i]
		name := names.Name(model.StructuralPath(strings.Split(c.Subject, "/")...))
		b := model.NewRunsBlock(name, c.Runs)
		b.Name = name
		b.Type = BlockType
		ids.Assign(b)
		b.Properties[PropLanguage] = f.Language
		b.Properties[PropStyle] = string(c.Style)
		if c.Doc {
			b.Properties[PropDoc] = "true"
		}
		if c.Deprecated {
			b.Properties[PropDeprecated] = "true"
		}
		b.Properties[PropSpan] = strconv.Itoa(c.Span.Start) + "-" + strconv.Itoa(c.Span.End)
		b.Properties[PropLines] = strconv.Itoa(c.Lines.First) + "-" + strconv.Itoa(c.Lines.Last)
		b.Identity = model.ComputeIdentity(b)
		blocks = append(blocks, b)
	}
	return blocks
}
