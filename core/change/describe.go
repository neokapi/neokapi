package change

import (
	"context"
	"fmt"
	"slices"
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
}

// Formats answers what the service knows about a format, by name.
type Formats interface {
	Facts(name string) (FormatFacts, bool)
}

// Description says what a format supports: which operations, and how. A nil
// entry in Ops is an operation the format refuses with unsupported.
type Description struct {
	Format string `json:"format"`
	// Editions is in-file for a format that holds every edition in one file
	// and one-per-file for the others.
	Editions Editions `json:"editions"`
	// Ops is every operation of the contract that addresses a document's
	// content, with what the format supports of it, or null.
	Ops map[Kind]*OpCapability `json:"ops"`
	// Native lists the format's own operations.
	Native []NativeOp `json:"native"`
}

// OpCapability says how a format supports one operation. The fields that do
// not apply to the operation are empty.
type OpCapability struct {
	// Forms are the content forms set_content takes: text, runs.
	Forms []string `json:"forms,omitempty"`
	// NewCodes are the code types a writer can synthesize, for set_content
	// in runs form and for mark.
	NewCodes []string `json:"new_codes,omitempty"`
	// Attributes, for set_attribute, maps a code type to the attributes the
	// writer can write.
	Attributes map[string][]string `json:"attributes,omitempty"`
	// Types, for mark, are the code types it can wrap text in; for annotate,
	// the annotation types the writer draws into the document.
	Types []string `json:"types,omitempty"`
}

// NativeOp is a format's own operation with the JSON Schema of its arguments.
type NativeOp struct {
	Name   string `json:"name"`
	Schema []byte `json:"schema,omitempty"`
}

// Capabilities says what a format supports. It is the one function that
// decides it: Describe reports it, Read lists each block's operations from it,
// and Apply refuses an operation it leaves out.
type Capabilities func(FormatFacts) Description

// contentKinds are the operations a format's capabilities decide, in the
// order the contract lists them.
var contentKinds = []Kind{
	KindSetContent, KindReplaceText, KindSetAttribute, KindMark, KindRemoveEdition,
	KindAnnotate, KindUnannotate, KindInsertBlock, KindDeleteBlock,
}

// DefaultCapabilities describes what ApplyBlock does for any format kapi can
// write back: set_content in either form, replace_text, and remove_edition
// where the format holds editions in one file. A writer synthesizes no new
// code, writes no attribute and holds no stand-off annotation here, so
// set_attribute, mark, annotate, unannotate and the structural operations are
// refused. A format kapi cannot write back supports nothing.
func DefaultCapabilities(f FormatFacts) Description {
	d := Description{Format: f.Name, Editions: EditionsPerFile, Ops: map[Kind]*OpCapability{}, Native: []NativeOp{}}
	if f.Interchange {
		d.Editions = EditionsInFile
	}
	for _, k := range contentKinds {
		d.Ops[k] = nil
	}
	if !f.Editable && !f.Interchange {
		return d
	}
	d.Ops[KindSetContent] = &OpCapability{Forms: []string{"text", "runs"}, NewCodes: []string{}}
	d.Ops[KindReplaceText] = &OpCapability{}
	if f.Interchange {
		d.Ops[KindRemoveEdition] = &OpCapability{}
	}
	return d
}

// supports reports whether d supports op, and the refusal when it does not.
func (d Description) supports(op Op) *Error {
	if _, ok := d.Ops[op.Kind]; !ok {
		// decide, provenance and the asset operations are not a format's to
		// decide.
		if op.Kind != KindNative {
			return nil
		}
		if slices.ContainsFunc(d.Native, func(n NativeOp) bool {
			body, _ := op.Body.(*Native)
			return body != nil && n.Name == body.Name
		}) {
			return nil
		}
		return &Error{Code: CodeUnsupported, Capability: string(KindNative),
			Message: fmt.Sprintf("the %s format has no native operations", d.Format)}
	}
	if d.Ops[op.Kind] == nil {
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
		if k == KindInsertBlock || d.Ops[k] == nil {
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
			return nil, asError(err)
		}
		in := sess.Info()
		info = &in
		_ = sess.Close()
		name = in.Format
	}
	facts, ok := s.facts(name)
	if !ok {
		return nil, &Error{Code: CodeNotFound, Field: "format", Message: fmt.Sprintf("no format is named %q", name)}
	}
	d := s.caps(facts)
	if info != nil && info.Editions != "" {
		d.Editions = info.Editions
	}
	return &d, nil
}

// facts looks up a format, falling back to what a name alone says.
func (s *Service) facts(name string) (FormatFacts, bool) {
	if s.formats == nil {
		return FormatFacts{}, false
	}
	return s.formats.Facts(name)
}

// describe is the description of the format of an open document.
func (s *Service) describe(info DocInfo) Description {
	facts, ok := s.facts(info.Format)
	if !ok {
		facts = FormatFacts{Name: info.Format}
	}
	d := s.caps(facts)
	if info.Editions != "" {
		d.Editions = info.Editions
	}
	return d
}
