package changeschema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// ResultSchema returns the JSON Schema (draft 2020-12) of a change set's
// result, kapi.change-result/v1, indented. It is generated from the Go types
// the service marshals, change.Result and what it holds, so the shape a
// transport answers with and the schema it declares are one. It is the output
// schema of the apply_edits tool.
func ResultSchema() []byte {
	b, err := resultBytes()
	if err != nil {
		panic(fmt.Sprintf("changeschema: build the result schema: %v", err))
	}
	return slices.Clone(b)
}

// ReadSchema returns the JSON Schema (draft 2020-12) of a page a read
// returns, change.Page and the block records it holds, indented: what
// kapi inspect prints per block and read_blocks returns per page.
func ReadSchema() []byte {
	b, err := readBytes()
	if err != nil {
		panic(fmt.Sprintf("changeschema: build the read schema: %v", err))
	}
	return slices.Clone(b)
}

// refReply is a change.Ref as a reply writes it: an empty block and the
// document's own edition are left out.
type refReply struct {
	Doc     string `json:"doc" jsonschema:"the document, as references name it"`
	Block   string `json:"block,omitempty" jsonschema:"the block key"`
	Edition string `json:"edition,omitempty" jsonschema:"the edition; absent for the document's own edition where a reply leaves it out"`
}

// replyOptions maps the types a reply marshals in a form of their own.
func replyOptions() (*jsonschema.ForOptions, error) {
	ref, err := jsonschema.ForType(reflect.TypeFor[refReply](), nil)
	if err != nil {
		return nil, err
	}
	ref.Description = "an edition of a block in a document"
	return &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[change.Ref]():      ref,
		reflect.TypeFor[model.RunPath]():   {Ref: "#/$defs/path"},
		reflect.TypeFor[json.RawMessage](): {Description: "any JSON value"},
	}}, nil
}

var resultBytes = sync.OnceValues(func() ([]byte, error) {
	opts, err := replyOptions()
	if err != nil {
		return nil, err
	}
	root, err := jsonschema.ForType(reflect.TypeFor[change.Result](), opts)
	if err != nil {
		return nil, err
	}
	root.Schema = "https://json-schema.org/draft/2020-12/schema"
	root.Title = change.ResultSchemaID
	root.Description = "The outcome of a change set: its status, each document written or read, and each operation's outcome."
	var schema any = change.ResultSchemaID
	root.Properties["schema"].Const = &schema
	root.Properties["status"].Enum = anys(change.SetApplied, change.SetRefused, change.SetPreviewed, change.SetPartial)
	ops := root.Properties["ops"].Items
	ops.Properties["status"].Enum = anys(change.OpApplied, change.OpUnchanged, change.OpRefused, change.OpNotApplied, change.OpPreviewed)
	kinds := anys(change.Kinds()...)
	ops.Properties["op"].Enum = append(kinds, string(change.KindProvenance))
	for _, e := range []*jsonschema.Schema{root.Properties["error"], ops.Properties["error"]} {
		errorEnums(e)
	}
	root.Defs = map[string]*jsonschema.Schema{"path": pathSchema()}
	return json.MarshalIndent(root, "", "  ")
})

var readBytes = sync.OnceValues(func() ([]byte, error) {
	opts, err := replyOptions()
	if err != nil {
		return nil, err
	}
	root, err := jsonschema.ForType(reflect.TypeFor[change.Page](), opts)
	if err != nil {
		return nil, err
	}
	root.Schema = "https://json-schema.org/draft/2020-12/schema"
	root.Title = "kapi.change/v1 read"
	root.Description = "A page of a document's blocks: each block's reference, revision and content, its codes, structures and other editions, and the operations it accepts."
	root.Properties["blocks"].Items.Properties["ops"].Items.Enum = anys(change.Kinds()...)
	root.Defs = map[string]*jsonschema.Schema{"path": pathSchema()}
	return json.MarshalIndent(root, "", "  ")
})

// errorEnums lists the closed sets of a refusal: its code and its subcode.
func errorEnums(e *jsonschema.Schema) {
	if e == nil {
		return
	}
	codes := make([]any, 0, len(change.Codes()))
	for _, c := range change.Codes() {
		codes = append(codes, string(c))
	}
	e.Properties["code"].Enum = codes
	e.Properties["subcode"].Enum = anys(change.SubcodeCodesChanged, change.SubcodeStructureLost, change.SubcodeBadPosition, change.SubcodeOverlap)
}

// pathSchema is a run path: a walk into a plural or select.
func pathSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "array", Items: pathStep(),
		Description: "a walk into a plural or select: a run index, then the form or case, repeated"}
}

func anys[S ~string](values ...S) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}
