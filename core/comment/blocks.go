package comment

import (
	"strings"

	"github.com/neokapi/neokapi/core/format"
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
	// PropPackageDoc is "true" on the doc comment of the package or module the
	// file belongs to (PackageDoc).
	PropPackageDoc = "comment.package"
)

// PackageDoc reports whether a doc comment on subject documents the package or
// module its file belongs to rather than one declaration in it. The Go
// provider and the sourcecode plugin's Java provider name that comment's
// subject "package", and the Rust provider names a file's inner doc comment
// "module". No other provider has one: the comments of YAML, the XML family,
// HTML, Markdown, MDX, PO and properties document no declaration, and the other
// languages have no package doc comment.
func PackageDoc(subject string) bool {
	return subject == "package" || subject == "module"
}

// Blocks returns the file's comments as blocks, in file order.
//
// A block is named for what the comment sits on, so its name survives an edit
// above it: `func/Parse` stays `func/Parse` when a line is added at the top of
// the file. Two comments on one subject are told apart by an ordinal, and the
// id is the name, held unique within the file by model.IDBuilder. Where the
// block sits is its extent (Extents), which moves with the file and so stays
// out of the block's identity.
func (f *File) Blocks() []*model.Block {
	names, ids := f.names()
	blocks := make([]*model.Block, 0, len(f.Comments))
	for i := range f.Comments {
		c := &f.Comments[i]
		b := model.NewRunsBlock(ids[i], c.Runs)
		b.Name = names[i]
		b.Type = BlockType
		b.Properties[PropLanguage] = f.Language
		b.Properties[PropStyle] = string(c.Style)
		if c.Doc {
			b.Properties[PropDoc] = "true"
			if PackageDoc(c.Subject) {
				b.Properties[PropPackageDoc] = "true"
			}
		}
		if c.Deprecated {
			b.Properties[PropDeprecated] = "true"
		}
		b.Identity = model.ComputeIdentity(b)
		blocks = append(blocks, b)
	}
	return blocks
}

// IsBlock reports whether b is a block Blocks built from a comment.
func IsBlock(b *model.Block) bool {
	return b != nil && b.Type == BlockType && b.Properties[PropLanguage] != ""
}

// Extents returns where each comment's block sits in the file, in the order and
// under the ids Blocks gives them.
func (f *File) Extents() []format.Extent {
	_, ids := f.names()
	extents := make([]format.Extent, len(f.Comments))
	for i, c := range f.Comments {
		extents[i] = format.Extent{Block: ids[i], Start: c.Start, End: c.End, Lines: c.Lines}
	}
	return extents
}

// names assigns each comment its block name and id.
func (f *File) names() (names, ids []string) {
	var nb model.NameBuilder
	var ib model.IDBuilder
	names = make([]string, len(f.Comments))
	ids = make([]string, len(f.Comments))
	for i, c := range f.Comments {
		names[i] = nb.Name(model.StructuralPath(strings.Split(c.Subject, "/")...))
		b := &model.Block{ID: names[i]}
		ib.Assign(b)
		ids[i] = b.ID
	}
	return names, ids
}
