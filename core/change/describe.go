package change

import (
	"context"
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/format"
)

// FormatFacts is what the service knows about a format: what the registry
// declares for it.
type FormatFacts struct {
	Name string
	// Editable says kapi can write the format back after an edit: it has a
	// reader and a writer.
	Editable bool
	// Interchange says the format holds a document's editions in one file
	// (PO, XLIFF, TMX, xcstrings).
	Interchange bool
	// RoundTrip says the writer rebuilds the document from a skeleton, so an
	// edit changes only the edited text.
	RoundTrip bool
	// InlineAnnotations are the annotation types the writer draws into the
	// document.
	InlineAnnotations []string
	// Edit is what the format's writer declares it can write beyond what its
	// reader read (registry.FormatInfo.EditCapabilities).
	Edit format.EditCapabilities
}

// Formats answers what the service knows about a format, by name.
type Formats interface {
	Facts(name string) (FormatFacts, bool)
}

// Describer says what a format supports. It is the one function that decides
// it: Describe reports it, Read lists each block's operations from it, and
// Apply refuses an operation it leaves out.
type Describer func(FormatFacts) Description

// contentKinds are the operations a format's capabilities decide, in the
// order the contract lists them.
var contentKinds = []Kind{
	KindSetContent, KindReplaceText, KindSetAttribute, KindMark, KindRemoveEdition,
	KindAnnotate, KindUnannotate, KindInsertBlock, KindDeleteBlock,
}

// BaseOps are the operations a format's round trip carries without a writer
// capability, for any format kapi can write back: set_content in either form,
// replace_text, and remove_edition where the format holds editions in one
// file. A format kapi cannot write back carries none.
func BaseOps(f FormatFacts) []Kind {
	if !f.Editable && !f.Interchange {
		return nil
	}
	base := []Kind{KindSetContent, KindReplaceText}
	if f.Interchange {
		base = append(base, KindRemoveEdition)
	}
	return base
}

// DescribeFormat describes what ApplyBlock does for a format: the operations
// its round trip carries (BaseOps) and the ones its writer declares
// (FormatFacts.Edit), through FormatOps. A writer that declares nothing
// synthesizes no new code and writes no attribute, so set_attribute and mark
// are refused; annotate, unannotate and the structural operations are refused
// too.
func DescribeFormat(f FormatFacts) Description {
	editions := EditionsPerFile
	if f.Interchange {
		editions = EditionsInFile
	}
	return FormatOps(FormatDecl{
		Format:            f.Name,
		Editions:          editions,
		Base:              BaseOps(f),
		InlineAnnotations: f.InlineAnnotations,
		Edit:              f.Edit,
	})
}

// supports reports whether d supports op, and the refusal when it does not.
func (d Description) supports(op Op) *Error {
	content, ok := d.Ops.supports(op.Kind)
	if !content {
		// decide, provenance and the asset operations are not a format's to
		// decide.
		if op.Kind != KindNative {
			return nil
		}
		if slices.ContainsFunc(d.Native, func(n format.NativeOp) bool {
			body, _ := op.Body.(*Native)
			return body != nil && n.Name == body.Name
		}) {
			return nil
		}
		return &Error{Code: CodeUnsupported, Capability: string(KindNative),
			Message: fmt.Sprintf("the %s format has no native operations", d.Format)}
	}
	if !ok {
		return &Error{Code: CodeUnsupported, Capability: string(op.Kind),
			Message: fmt.Sprintf("the %s format does not support %s; describe the format to see what it supports", d.Format, op.Kind)}
	}
	return nil
}

// blockOps lists the operations a block of a format with description d
// accepts.
func (d Description) blockOps(editable bool) []Kind {
	out := []Kind{}
	if !editable {
		return out
	}
	for _, k := range contentKinds {
		if _, ok := d.Ops.supports(k); k == KindInsertBlock || !ok {
			continue
		}
		out = append(out, k)
	}
	return out
}

// DescribeRequest names what to describe: a format by name, or the format of
// a document.
type DescribeRequest struct {
	Format string `json:"format,omitempty"`
	Doc    string `json:"doc,omitempty"`
}

// Describe says what a format supports: the format named, or the format of
// the document named.
func (s *Service) Describe(ctx context.Context, q DescribeRequest) (*Description, error) {
	name := q.Format
	var info *DocInfo
	if name == "" {
		if q.Doc == "" {
			return nil, &Error{Code: CodeInvalid, Field: "format", Message: "name a format or a document to describe"}
		}
		sess, err := s.open(ctx, q.Doc)
		if err != nil {
			if e := asError(err); e != nil {
				return nil, e
			}
			return nil, err
		}
		in := sess.Info()
		info = &in
		_ = sess.Close()
		name = in.Format
	}
	if info != nil {
		if _, ok := s.facts(name); !ok {
			return nil, &Error{Code: CodeNotFound, Field: "format", Message: fmt.Sprintf("no format is named %q", name)}
		}
		d := s.describe(*info)
		return &d, nil
	}
	facts, ok := s.facts(name)
	if !ok {
		return nil, &Error{Code: CodeNotFound, Field: "format", Message: fmt.Sprintf("no format is named %q", name)}
	}
	d := s.describer(facts)
	return &d, nil
}

// facts looks up a format, falling back to what a name alone says.
func (s *Service) facts(name string) (FormatFacts, bool) {
	if s.formats == nil {
		return FormatFacts{}, false
	}
	return s.formats.Facts(name)
}

// describe is the description of the format of an open document. What its
// writer declares is what the home reports for the document
// (DocInfo.Capabilities), the declaration ApplyBlock applies its operations
// with, so a description never offers an operation the write would refuse.
func (s *Service) describe(info DocInfo) Description {
	facts, ok := s.facts(info.Format)
	if !ok {
		facts = FormatFacts{Name: info.Format}
	}
	facts.Edit = info.Capabilities.Declared
	d := s.describer(facts)
	if info.Editions != "" {
		d.Editions = info.Editions
	}
	return d
}
