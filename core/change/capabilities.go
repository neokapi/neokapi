package change

import (
	"slices"

	"github.com/neokapi/neokapi/core/format"
)

// Capabilities is what the writer of a block's format can write into the block
// beyond what its reader read: the attributes of an inline code set_attribute
// may change, and the vocabulary types mark, and a new code in a runs payload,
// may create. ApplyBlock applies those operations exactly where the format
// declares them and refuses every other request as unsupported. The zero value
// declares nothing.
type Capabilities struct {
	// Format names the format, for messages.
	Format string
	// Declared is what the format's writer declares (registry.FormatInfo).
	Declared format.EditCapabilities
	// Attrs spells a changed attribute into the code's native data. It is the
	// format's writer for a format kapi writes in process. Nil means the writer
	// runs outside the process (a plugin): the code records the new value
	// among its attributes, and the plugin's writer spells it when it writes.
	Attrs format.AttrWriter
	// Codes spells a new code's native data, as Attrs does for an attribute.
	// Nil leaves a new code with its type and attributes and no native data,
	// for a writer outside the process to spell.
	Codes format.CodeSynthesizer
	// Values spells runs as the value the writer writes for them, for a
	// writer whose reader reads syntax in a value as a plural or select. A
	// new block's content is compared as it spells it. Nil compares the runs'
	// data (model.RenderRunsWithData).
	Values format.ValueSpeller
}

// WriterCapabilities returns what an in-process writer declares, with the
// writer itself spelling what it declares. It is how the change service gives
// ApplyBlock the capabilities of the format it writes.
func WriterCapabilities(name string, w format.DataFormatWriter) Capabilities {
	c := Capabilities{Format: name, Declared: format.ProbeEditCapabilities(w)}
	if aw, ok := w.(format.AttrWriter); ok {
		c.Attrs = aw
	}
	if cs, ok := w.(format.CodeSynthesizer); ok {
		c.Codes = cs
	}
	if vs, ok := w.(format.ValueSpeller); ok {
		c.Values = vs
	}
	return c
}

// DeclaredCapabilities returns the capabilities a format declares without an
// in-process writer to spell them: a plugin format, whose writer spells new
// attribute values and new codes when it writes the document.
func DeclaredCapabilities(name string, declared format.EditCapabilities) Capabilities {
	return Capabilities{Format: name, Declared: declared.Clone()}
}

// formatName names the format in a message.
func (c Capabilities) formatName() string {
	if c.Format == "" {
		return "this format"
	}
	return c.Format
}

// Editions says how a format holds a document's editions.
type Editions string

const (
	// EditionsPerFile: a file holds one edition, and each other edition of
	// the document lives in a file of its own, as a project's target files
	// do: the monolingual formats.
	EditionsPerFile Editions = "one-per-file"
	// EditionsInFile: one file holds every edition, as a bilingual
	// interchange file (PO, XLIFF, TMX, xcstrings) does.
	EditionsInFile Editions = "in-file"
)

// FormatDecl is what FormatOps describes: the operations a format's round trip
// carries and what its writer declares.
type FormatDecl struct {
	// Format is the format id.
	Format string
	// Editions says where the format keeps a block's editions.
	Editions Editions
	// Base lists the operations the format's round trip carries without a
	// writer capability: any of set_content, replace_text, remove_edition,
	// annotate and unannotate. The change service names them from what the
	// operations matrix proves for the format; others are ignored.
	Base []Kind
	// InlineAnnotations are the annotation types the writer draws inline
	// (registry.FormatInfo.InlineAnnotations).
	InlineAnnotations []string
	// Edit is what the writer declares (registry.FormatInfo.EditCapabilities).
	Edit format.EditCapabilities
}

// Description is what one format supports of the contract: the table
// describe_format and `kapi formats --ops` publish. An operation the format
// refuses as unsupported is null.
type Description struct {
	Format   string            `json:"format"`
	Editions Editions          `json:"editions"`
	Ops      OpTable           `json:"ops"`
	Native   []format.NativeOp `json:"native"`

	// doc is the document a description of an open document describes, and
	// narrowed the operations its format supports and its home cannot write
	// there.
	doc      string
	narrowed []Kind
}

// OpTable lists, per content operation, what a format supports; nil is
// unsupported.
type OpTable struct {
	SetContent    *SetContentSupport  `json:"set_content"`
	ReplaceText   *Supported          `json:"replace_text"`
	SetAttribute  map[string][]string `json:"set_attribute"`
	Mark          *MarkSupport        `json:"mark"`
	RemoveEdition *Supported          `json:"remove_edition"`
	Annotate      *AnnotateSupport    `json:"annotate"`
	Unannotate    *Supported          `json:"unannotate"`
	InsertBlock   *Supported          `json:"insert_block"`
	DeleteBlock   *Supported          `json:"delete_block"`
}

// drop marks the structural operation k unsupported.
func (t *OpTable) drop(k Kind) {
	switch k {
	case KindInsertBlock:
		t.InsertBlock = nil
	case KindDeleteBlock:
		t.DeleteBlock = nil
	}
}

// supports reports whether k is an operation the table decides (content), and
// whether the format supports it (ok).
func (t OpTable) supports(k Kind) (content, ok bool) {
	switch k {
	case KindSetContent:
		return true, t.SetContent != nil
	case KindReplaceText:
		return true, t.ReplaceText != nil
	case KindSetAttribute:
		return true, len(t.SetAttribute) > 0
	case KindMark:
		return true, t.Mark != nil
	case KindRemoveEdition:
		return true, t.RemoveEdition != nil
	case KindAnnotate:
		return true, t.Annotate != nil
	case KindUnannotate:
		return true, t.Unannotate != nil
	case KindInsertBlock:
		return true, t.InsertBlock != nil
	case KindDeleteBlock:
		return true, t.DeleteBlock != nil
	}
	return false, false
}

// Supported marks an operation a format supports with nothing to qualify it.
type Supported struct{}

// SetContentSupport qualifies set_content: the content forms accepted, and
// the vocabulary types a runs payload may introduce as new codes.
type SetContentSupport struct {
	Forms    []string `json:"forms"`
	NewCodes []string `json:"new_codes"`
}

// MarkSupport lists the vocabulary types mark may create.
type MarkSupport struct {
	Types []string `json:"types"`
}

// AnnotateSupport lists the annotation types the format writes inline; every
// other annotation stays stand-off.
type AnnotateSupport struct {
	Inline []string `json:"inline"`
}

// FormatOps describes what a format supports of the contract: the operations
// its round trip carries (d.Base) and the ones its writer declares. The
// change service's Describe returns it for one format.
func FormatOps(d FormatDecl) Description {
	edit := d.Edit.Clone()
	editions := d.Editions
	if editions == "" {
		editions = EditionsPerFile
	}
	desc := Description{Format: d.Format, Editions: editions, Native: edit.NativeOps}
	if desc.Native == nil {
		desc.Native = []format.NativeOp{}
	}
	ops := &desc.Ops
	for _, k := range d.Base {
		switch k {
		case KindSetContent:
			newCodes := edit.Synthesizes
			if newCodes == nil {
				newCodes = []string{}
			}
			ops.SetContent = &SetContentSupport{Forms: []string{"text", "runs"}, NewCodes: newCodes}
		case KindReplaceText:
			ops.ReplaceText = &Supported{}
		case KindRemoveEdition:
			ops.RemoveEdition = &Supported{}
		case KindAnnotate:
			inline := slices.Clone(d.InlineAnnotations)
			slices.Sort(inline)
			if inline == nil {
				inline = []string{}
			}
			ops.Annotate = &AnnotateSupport{Inline: inline}
		case KindUnannotate:
			ops.Unannotate = &Supported{}
		}
	}
	if len(edit.WritableAttrs) > 0 {
		ops.SetAttribute = edit.WritableAttrs
	}
	if len(edit.Synthesizes) > 0 {
		ops.Mark = &MarkSupport{Types: edit.Synthesizes}
	}
	for _, s := range edit.Structural {
		switch Kind(s) {
		case KindInsertBlock:
			ops.InsertBlock = &Supported{}
		case KindDeleteBlock:
			ops.DeleteBlock = &Supported{}
		}
	}
	return desc
}
