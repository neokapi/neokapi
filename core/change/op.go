package change

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// SchemaID names the contract version a change set follows.
const SchemaID = "kapi.change/v1"

// ResultSchemaID names the version of the result shape.
const ResultSchemaID = "kapi.change-result/v1"

// AnyRevision as IfMatch applies an operation to whatever the edition holds.
// It is a blind write: a porcelain fills IfMatch from its own read and never
// sends it.
const AnyRevision = "*"

// Kind names an operation.
type Kind string

// The operations of kapi.change/v1.
const (
	// Content operations, each addressed to one edition of one block.
	KindSetContent    Kind = "set_content"
	KindReplaceText   Kind = "replace_text"
	KindSetAttribute  Kind = "set_attribute"
	KindMark          Kind = "mark"
	KindRemoveEdition Kind = "remove_edition"
	KindAnnotate      Kind = "annotate"
	KindUnannotate    Kind = "unannotate"
	KindInsertBlock   Kind = "insert_block"
	KindDeleteBlock   Kind = "delete_block"
	KindNative        Kind = "native"
	// A review decision bound to the revision its sender read.
	KindDecide Kind = "decide"
	// Asset operations.
	KindTerm   Kind = "term"
	KindMemory Kind = "memory"
	KindRecipe Kind = "recipe"

	// KindProvenance records how a tool produced an edition: its status and
	// origin. Only a tool in a flow sends it, in process; no change set
	// carries it, and the schema does not list it.
	KindProvenance Kind = "provenance"
)

// Kinds returns the operations a change set may carry, in the order the
// schema lists them.
func Kinds() []Kind {
	out := make([]Kind, 0, len(specs))
	for _, s := range specs {
		if s.public {
			out = append(out, s.kind)
		}
	}
	return out
}

// Op is one operation of a change set. Its wire form is flat: {"op": kind,
// "at": …, "if_match": …, …the body's fields}.
type Op struct {
	Kind Kind
	// At is what the operation addresses: an edition of a block for the
	// content operations and decide; a block, with no edition, for
	// delete_block; a document alone (At.Doc, the "doc" field on the wire) for
	// insert_block and native. The asset operations address nothing.
	At Ref
	// IfMatch is the revision of the edition the sender read
	// (model.EditionRevision), model.AbsentRevision for an edition that must not
	// exist yet, or AnyRevision. For native it is the document digest.
	// delete_block carries one revision per edition in its body instead.
	IfMatch string
	// Basis is, for set_content on a derived edition, the revision of the
	// authoritative edition the content was made from. It is recorded always,
	// and refuses only under the envelope's require_basis.
	Basis string
	// Body holds the kind's own fields: *SetContent for set_content, and so
	// on. Its concrete type always matches Kind.
	Body Body
}

// Body is the kind-specific part of an operation.
type Body interface {
	opKind() Kind
}

// Content is an edition's content in one of its two forms: Text, the
// placeholder form a read shows (inline codes as <x id="…"/>), or Runs. Exactly
// one is set.
type Content struct {
	Text *string     `json:"text,omitempty" jsonschema:"the content with inline codes as the <x id=\"…\"/> placeholders a read shows"`
	Runs []model.Run `json:"runs,omitzero" jsonschema:"the content as runs; a code carries type and attributes and never data"`
}

// SetContent replaces an edition's content, or with IfMatch absent creates
// the edition.
type SetContent struct {
	Content
	// Path names a plural form or a select case to replace instead of the
	// whole edition: it walks to the plural or select run, then names the
	// branch.
	Path model.RunPath `json:"path,omitempty" jsonschema:"a plural form or select case to replace instead of the whole edition, for example [1, {\"plural\": \"one\"}]"`
	// Origin says how a tool produced the content, as a run printed with
	// --print-ops states it. Nothing records it: the edition takes the origin
	// its sender's edit gives it (Consequences), kapi apply says so, and a
	// tool in a flow records how it produced content with the provenance
	// operation.
	Origin *ToolOrigin `json:"origin,omitempty" jsonschema:"how a tool produced the content; an edit a person or an agent sends is recorded as theirs, and this is not kept"`
	// Overlays says how the overlays on the edition follow the new content. It
	// is set in process only.
	Overlays OverlayRebase `json:"-"`
}

// ToolOrigin is how a tool produced content: the tool, and what it drew on.
type ToolOrigin struct {
	Tool   string `json:"tool" jsonschema:"the tool that produced the content"`
	Kind   string `json:"kind,omitempty" jsonschema:"what the tool drew on, such as mt, ai or memory"`
	Engine string `json:"engine,omitempty" jsonschema:"the engine or model the tool used"`
}

// OriginOf is the tool origin a model origin names, or nil when it names no
// tool.
func OriginOf(o model.Origin) *ToolOrigin {
	if o.Tool == "" {
		return nil
	}
	return &ToolOrigin{Tool: o.Tool, Kind: o.Kind, Engine: o.Engine}
}

// OverlayRebase says how overlays on an edition follow a rewrite of its
// content. The zero value maps them across the region the old and the new
// text differ in.
type OverlayRebase struct {
	// Drop removes every overlay on the edition: the rewrite has no mapping
	// from the old text to the new.
	Drop bool
	// Edits is the mapping, in the flattened-text coordinates of the old runs
	// (model.RunEdit). Nil computes it.
	Edits []model.RunEdit
}

// ReplaceText changes text inside an edition and keeps everything else:
// inline codes, structure and run flags.
type ReplaceText struct {
	Edits []TextEdit `json:"edits" jsonschema:"the replacements, each naming its text by find, by start and end, or by range"`
	// Origin says how a tool produced the edited content, as SetContent's
	// does.
	Origin *ToolOrigin `json:"origin,omitempty" jsonschema:"how a tool produced the content; an edit a person or an agent sends is recorded as theirs, and this is not kept"`
}

// Selection names text in an edition: exactly one of Find, Start with End, or
// Range.
type Selection struct {
	// Path walks into a plural form or select case; empty is the edition's own
	// runs.
	Path model.RunPath `json:"path,omitempty" jsonschema:"a plural form or select case the text is in, for example [1, {\"plural\": \"one\"}]"`
	// Find is literal text to match.
	Find *string `json:"find,omitempty" jsonschema:"literal text to match"`
	// Occurrence chooses one match of Find, counting from 1; 0 requires
	// exactly one match.
	Occurrence int `json:"occurrence,omitempty" jsonschema:"which match of find, counting from 1; omitted requires exactly one match"`
	// Start and End are code-point offsets into the text of the sequence Path
	// reaches; inline codes have zero width. They, Range and Path's run
	// indexes name the edition as the change set found it; Find matches it as
	// the operations before this one left it.
	Start *int `json:"start,omitempty" jsonschema:"start offset in Unicode code points of the text; inline codes have zero width"`
	End   *int `json:"end,omitempty" jsonschema:"end offset, exclusive"`
	// Range is a span between run positions.
	Range *Span `json:"range,omitempty" jsonschema:"a span between run positions"`
}

// TextEdit replaces the text a Selection names.
type TextEdit struct {
	Selection
	Text string `json:"text" jsonschema:"the replacement text"`
}

// Span is a half-open range between two run positions: a run index and a
// code-point offset into that run's text.
type Span struct {
	Start model.RunPos `json:"start"`
	End   model.RunPos `json:"end"`
}

// SetAttribute changes one writable attribute of an inline code.
type SetAttribute struct {
	Code  string `json:"code" jsonschema:"the code's id, as the placeholder <x id=\"1\"/> shows it"`
	Name  string `json:"name" jsonschema:"the attribute, such as href, src, alt or title, where the format declares it writable"`
	Value string `json:"value" jsonschema:"the new value"`
}

// Mark wraps text in a new paired code of a vocabulary type.
type Mark struct {
	Range Selection         `json:"range" jsonschema:"the text to mark"`
	Type  string            `json:"type" jsonschema:"a vocabulary type such as fmt:bold or link:hyperlink"`
	Attrs map[string]string `json:"attrs,omitempty" jsonschema:"the new code's attributes, such as href"`
}

// RemoveEdition removes one edition of a block. It has no fields of its own.
type RemoveEdition struct{}

// Annotate adds a stand-off annotation to an edition, or replaces the one with
// the same type and id.
type Annotate struct {
	Type   string          `json:"type" jsonschema:"the annotation type, such as note, entity or term"`
	ID     string          `json:"id,omitempty" jsonschema:"the annotation's id; omitted mints one and the result names it"`
	Anchor *model.Anchor   `json:"anchor,omitempty" jsonschema:"where in the edition it applies; omitted is the whole block"`
	Value  json.RawMessage `json:"value,omitempty" jsonschema:"the annotation's payload"`

	// Spans, set in process only, are written as given in place of the one
	// annotation ID, Anchor and Value describe: a tool's view writes whole
	// spans, whose Props and typed Value the wire form does not carry. They
	// are added to the edition's overlay of Type and Layer, or with Replace
	// take its place; Replace with no spans removes it.
	Spans   []model.Span `json:"-"`
	Layer   string       `json:"-"`
	Replace bool         `json:"-"`
}

// Unannotate removes one annotation by type and id.
type Unannotate struct {
	Type string `json:"type" jsonschema:"the annotation type"`
	ID   string `json:"id" jsonschema:"the annotation's id"`

	// All, set in process only, removes every annotation of Type on the
	// edition, in every layer, and needs no ID.
	All bool `json:"-"`
}

// InsertBlock adds a block to a document.
type InsertBlock struct {
	After    string             `json:"after,omitempty" jsonschema:"the key of the block the new one follows"`
	Before   string             `json:"before,omitempty" jsonschema:"the key of the block the new one precedes"`
	Name     string             `json:"name,omitempty" jsonschema:"the new block's key, where the format names blocks by key"`
	Editions map[string]Content `json:"editions" jsonschema:"the new block's content per edition key"`
}

// DeleteBlock removes a block from a document.
type DeleteBlock struct {
	// IfMatch maps each edition key of the block to the revision read.
	IfMatch map[string]string `json:"if_match" jsonschema:"the revision read of each edition of the block, by edition key"`
}

// Native is a format-specific operation, validated against the schema the
// format publishes for it.
type Native struct {
	Name string          `json:"name" jsonschema:"the operation, such as docx.append_paragraph"`
	Args json.RawMessage `json:"args" jsonschema:"the arguments, as the format's schema for the operation describes them"`
}

// Outcome is what a review decision says.
type Outcome string

// The outcomes of decide.
const (
	OutcomeEstablish Outcome = "establish"
	OutcomeReject    Outcome = "reject"
	OutcomeWithdraw  Outcome = "withdraw"
	OutcomeAdvise    Outcome = "advise"
)

// Decide records a review decision on the edition revision its sender read.
type Decide struct {
	Outcome Outcome  `json:"outcome" jsonschema:"establish, reject, withdraw, or advise (an agent's pre-review)"`
	Score   *int     `json:"score,omitempty" jsonschema:"a score from 0 to 100"`
	Reasons []string `json:"reasons,omitempty" jsonschema:"why"`
}

// Term writes or deletes a term in the terms store.
type Term struct {
	Action         string `json:"action" jsonschema:"upsert or delete"`
	Term           string `json:"term" jsonschema:"the term"`
	Locale         string `json:"locale,omitempty" jsonschema:"the term's language; omitted is the project's source language"`
	Status         string `json:"status,omitempty" jsonschema:"preferred, admitted, deprecated or forbidden"`
	Replacement    string `json:"replacement,omitempty" jsonschema:"the wording to use instead of a discouraged term"`
	Replaces       string `json:"replaces,omitempty" jsonschema:"a term this one replaces"`
	DoNotTranslate *bool  `json:"do_not_translate,omitempty" jsonschema:"true keeps the term verbatim in every language, false clears that"`
	Advisory       bool   `json:"advisory,omitempty" jsonschema:"a use of a discouraged term reports without failing a check"`
	Competitor     bool   `json:"competitor,omitempty" jsonschema:"the term is a competitor's name"`
}

// MemoryText is one side of a content-memory pair.
type MemoryText struct {
	Edition string `json:"edition" jsonschema:"the edition key of this side"`
	Text    string `json:"text" jsonschema:"the text"`
}

// Memory adds or deletes a content-memory pair.
type Memory struct {
	Action string     `json:"action" jsonschema:"add or delete"`
	From   MemoryText `json:"from" jsonschema:"the side the pair translates from"`
	To     MemoryText `json:"to" jsonschema:"the side the pair translates to"`
}

// Recipe sets one field of the recipe through its allowlist.
type Recipe struct {
	Path  string          `json:"path" jsonschema:"the recipe field, as a dotted path"`
	Value json.RawMessage `json:"value" jsonschema:"the new value"`
}

// Provenance records how a tool produced an edition. It is the body of
// KindProvenance, which only a tool sends in process.
type Provenance struct {
	Status model.Status `json:"status,omitempty"`
	Origin model.Origin `json:"origin,omitzero"`
	// Score, when set, replaces the edition's score.
	Score *float64 `json:"score,omitempty"`
}

func (*SetContent) opKind() Kind    { return KindSetContent }
func (*ReplaceText) opKind() Kind   { return KindReplaceText }
func (*SetAttribute) opKind() Kind  { return KindSetAttribute }
func (*Mark) opKind() Kind          { return KindMark }
func (*RemoveEdition) opKind() Kind { return KindRemoveEdition }
func (*Annotate) opKind() Kind      { return KindAnnotate }
func (*Unannotate) opKind() Kind    { return KindUnannotate }
func (*InsertBlock) opKind() Kind   { return KindInsertBlock }
func (*DeleteBlock) opKind() Kind   { return KindDeleteBlock }
func (*Native) opKind() Kind        { return KindNative }
func (*Decide) opKind() Kind        { return KindDecide }
func (*Term) opKind() Kind          { return KindTerm }
func (*Memory) opKind() Kind        { return KindMemory }
func (*Recipe) opKind() Kind        { return KindRecipe }
func (*Provenance) opKind() Kind    { return KindProvenance }

// The short names the spec table and the decoder use for each Address and
// IfMatchRule.
const (
	atNone    = AddressNone
	atEdition = AddressEdition
	atBlock   = AddressBlock
	atDoc     = AddressDoc

	ifMatchNone     = IfMatchNone
	ifMatchRequired = IfMatchRequired
	ifMatchOptional = IfMatchOptional
	ifMatchDigest   = IfMatchDigest
	ifMatchBody     = IfMatchInBody
)

// opSpec is the wire shape of one operation kind.
type opSpec struct {
	kind    Kind
	newBody func() Body
	at      Address
	ifMatch IfMatchRule
	basis   bool
	public  bool
	// summary describes the operation in the schema.
	summary string
}

var specs = []opSpec{
	{KindSetContent, func() Body { return &SetContent{} }, atEdition, ifMatchRequired, true, true,
		`Replace an edition's content with text (placeholder form) or runs. With if_match "absent" it creates the edition.`},
	{KindReplaceText, func() Body { return &ReplaceText{} }, atEdition, ifMatchRequired, false, true,
		"Change text inside an edition by find, by code-point offsets or by run positions, keeping inline codes and structure."},
	{KindSetAttribute, func() Body { return &SetAttribute{} }, atEdition, ifMatchRequired, false, true,
		"Change a writable attribute of an inline code, such as a link's href."},
	{KindMark, func() Body { return &Mark{} }, atEdition, ifMatchRequired, false, true,
		"Wrap text in a new paired code of a vocabulary type, such as fmt:bold."},
	{KindRemoveEdition, func() Body { return &RemoveEdition{} }, atEdition, ifMatchRequired, false, true,
		"Remove one edition of a block."},
	{KindAnnotate, func() Body { return &Annotate{} }, atEdition, ifMatchOptional, false, true,
		"Add a stand-off annotation to an edition, or replace the one with the same type and id."},
	{KindUnannotate, func() Body { return &Unannotate{} }, atEdition, ifMatchNone, false, true,
		"Remove an annotation by type and id."},
	{KindInsertBlock, func() Body { return &InsertBlock{} }, atDoc, ifMatchNone, false, true,
		"Add a block to a document, after or before a block key."},
	{KindDeleteBlock, func() Body { return &DeleteBlock{} }, atBlock, ifMatchBody, false, true,
		"Remove a block from a document."},
	{KindNative, func() Body { return &Native{} }, atDoc, ifMatchDigest, false, true,
		"A format-specific operation, guarded by the document digest."},
	{KindDecide, func() Body { return &Decide{} }, atEdition, ifMatchRequired, false, true,
		"Record a review decision on the edition revision read. An agent sends only advise."},
	{KindTerm, func() Body { return &Term{} }, atNone, ifMatchNone, false, true,
		"Write or delete a term in the terms store."},
	{KindMemory, func() Body { return &Memory{} }, atNone, ifMatchNone, false, true,
		"Add or delete a content-memory pair."},
	{KindRecipe, func() Body { return &Recipe{} }, atNone, ifMatchNone, false, true,
		"Set one recipe field through its allowlist."},
	{KindProvenance, func() Body { return &Provenance{} }, atEdition, ifMatchNone, false, false,
		"Record how a tool produced an edition."},
}

// Address is how an operation names what it changes on the wire.
type Address int

const (
	// AddressNone: the operation names no document (an asset operation).
	AddressNone Address = iota
	// AddressEdition: "at" is {doc, block, edition?}.
	AddressEdition
	// AddressBlock: "at" is {doc, block}, with no edition.
	AddressBlock
	// AddressDoc: "doc" names the document.
	AddressDoc
)

// IfMatchRule is what an operation's if_match is on the wire.
type IfMatchRule int

const (
	// IfMatchNone: the operation takes no if_match.
	IfMatchNone IfMatchRule = iota
	// IfMatchRequired: a revision, "absent" or "*", required.
	IfMatchRequired
	// IfMatchOptional: a revision, optional.
	IfMatchOptional
	// IfMatchDigest: a document digest, required.
	IfMatchDigest
	// IfMatchInBody: the body carries the revisions (delete_block).
	IfMatchInBody
)

// OperationSpec describes one operation of the contract, for a transport that
// publishes its shape (the JSON Schema in changeschema).
type OperationSpec struct {
	Kind    Kind
	Summary string
	Address Address
	IfMatch IfMatchRule
	// Basis says the operation records the basis of a derived edition.
	Basis bool
	// Body is a new, empty body of the operation's type.
	Body Body
}

// Operations describes the operations a change set may carry, in the order
// Kinds lists them.
func Operations() []OperationSpec {
	out := make([]OperationSpec, 0, len(specs))
	for _, s := range specs {
		if !s.public {
			continue
		}
		out = append(out, OperationSpec{Kind: s.kind, Summary: s.summary, Address: s.at, IfMatch: s.ifMatch, Basis: s.basis, Body: s.newBody()})
	}
	return out
}

func specOf(k Kind) (opSpec, bool) {
	for _, s := range specs {
		if s.kind == k {
			return s, true
		}
	}
	return opSpec{}, false
}

var (
	revisionRe = regexp.MustCompile(`^r:[0-9a-f]{16}$`)
	digestRe   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// validRevision reports whether s is a revision token.
func validRevision(s string) bool { return revisionRe.MatchString(s) }

// MarshalJSON writes the flat wire form: op, the address, if_match and basis,
// then the body's fields.
func (o Op) MarshalJSON() ([]byte, error) {
	spec, ok := specOf(o.Kind)
	if !ok {
		return nil, fmt.Errorf("change: unknown operation %q", o.Kind)
	}
	if o.Body == nil || o.Body.opKind() != o.Kind {
		return nil, fmt.Errorf("change: %s operation has a %T body", o.Kind, o.Body)
	}
	var buf bytes.Buffer
	buf.WriteString(`{"op":`)
	writeJSON(&buf, string(o.Kind))
	switch spec.at {
	case atEdition, atBlock:
		buf.WriteString(`,"at":`)
		b, err := o.At.MarshalJSON()
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	case atDoc:
		buf.WriteString(`,"doc":`)
		writeJSON(&buf, o.At.Doc)
	}
	if o.IfMatch != "" && spec.ifMatch != ifMatchNone && spec.ifMatch != ifMatchBody {
		buf.WriteString(`,"if_match":`)
		writeJSON(&buf, o.IfMatch)
	}
	if o.Basis != "" && spec.basis {
		buf.WriteString(`,"basis":`)
		writeJSON(&buf, o.Basis)
	}
	body, err := marshalNoEscape(o.Body)
	if err != nil {
		return nil, err
	}
	if inner := bytes.TrimSpace(body); len(inner) > 2 {
		buf.WriteByte(',')
		buf.Write(inner[1 : len(inner)-1])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func writeJSON(buf *bytes.Buffer, v any) {
	b, _ := marshalNoEscape(v)
	buf.Write(b)
}

// marshalNoEscape encodes v as JSON with HTML escaping off, so markup in text
// reads as written.
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// bodyFields lists the JSON field names a body type carries, with whether
// each is required (no omitempty or omitzero).
func bodyFields(t reflect.Type) []wireField {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	var out []wireField
	for _, f := range reflect.VisibleFields(t) {
		if f.Anonymous || !f.IsExported() {
			continue
		}
		name, required, ok := jsonName(f)
		if !ok {
			continue
		}
		out = append(out, wireField{name: name, required: required, index: f.Index})
	}
	return out
}

// wireField is one JSON field of a struct.
type wireField struct {
	name     string
	required bool
	index    []int
}

// jsonName reads a field's json tag: its name, whether it is required, and
// whether it is on the wire at all.
func jsonName(f reflect.StructField) (name string, required, ok bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false, false
	}
	name = f.Name
	required = true
	parts := strings.Split(tag, ",")
	if parts[0] != "" {
		name = parts[0]
	}
	if slices.Contains(parts[1:], "omitempty") || slices.Contains(parts[1:], "omitzero") {
		required = false
	}
	return name, required, true
}
