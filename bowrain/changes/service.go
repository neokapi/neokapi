package changes

import (
	"slices"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/registry"
)

// Origin is the surface the server's change service reports in each record.
const Origin = "server"

// streamOps are the operations a row keeps whatever the item's format: the
// content in either form, an edition removed, and stand-off annotations, which
// a row stores beside the content.
var streamOps = []change.Kind{
	change.KindSetContent, change.KindReplaceText, change.KindRemoveEdition,
	change.KindAnnotate, change.KindUnannotate,
}

// Describe is what a stream holds of the contract for an item of format f:
// the operations every row keeps, and what the format's writer declares it can
// write (set_attribute, mark, new codes), which an export of the item writes.
// A stream adds and removes no block through a change set: its blocks come
// from a push or an upload.
func Describe(f change.FormatFacts) change.Description {
	edit := f.Edit.Clone()
	edit.Structural = nil
	return change.FormatOps(change.FormatDecl{
		Format:   f.Name,
		Editions: change.EditionsInFile,
		Base:     slices.Clone(streamOps),
		Edit:     edit,
	})
}

// NewService returns the change service over home: the formats reg declares,
// the stream's description of them, and opts (the policy, the commit check,
// the recorder and the decisions the caller brings).
func NewService(home *Home, reg *registry.FormatRegistry, opts ...change.Option) *change.Service {
	base := []change.Option{change.WithDescriber(Describe), change.WithOrigin(Origin)}
	return change.NewService(filehome.Formats{Registry: reg}, change.OneHome(home), append(base, opts...)...)
}
