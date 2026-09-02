package main

// content.go emits packages/contract-types/src/content.gen.ts — the
// content-model TypeScript types, generated from the two Go sources of truth
// so no frontend package hand-mirrors the model again (AD-034):
//
//  1. Wire shapes: the neokapi.content.v1 proto descriptors
//     (core/proto/content/v1/content.proto), rendered as they appear in the
//     canonical protojson encoding (contentv1.MarshalJSON): lowerCamelCase
//     JSON names, unpopulated fields omitted, bytes as base64 strings.
//     Oneof-only messages (RunMessage, ContentRef) render as discriminated
//     unions.
//  2. Projection shapes: the model.Run JSON (RFC 0001 — the run form used
//     inside ContentTree, KBF, and flow traces) and the core/editor
//     ContentTree family, reflected from the Go structs like the IO-contract
//     atoms in emit.go.

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
	contentv1 "github.com/neokapi/neokapi/core/proto/content/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// modelRunType is special-cased in tsType (see emit.go): model.Run has a
// custom MarshalJSON (flat `{"text":"…"}` text runs), so struct reflection
// would emit the wrong shape. References render as "Run" and the union is
// hand-emitted by emitModelRun.
var modelRunType = reflect.TypeFor[model.Run]()

// modelRunConstraintsType renders as a reference to the wire-section
// RunConstraints interface rather than emitting a second interface: the model
// JSON always populates all three booleans, so the wire shape (all optional)
// covers both encodings.
var modelRunConstraintsType = reflect.TypeFor[model.RunConstraints]()

// projectionTypes is the ordered set of projection structs reflected into TS:
// the model.Run payload structs (the Run union itself is hand-emitted), the
// run-anchored range, and the core/editor ContentTree family (an
// explicitly-labeled projection — see AD-034 and core/editor/anatomy.go).
var projectionTypes = []emitType{
	{"PlaceholderRun", reflect.TypeFor[model.PlaceholderRun](),
		"A self-closing placeholder run (variable, JSX expression, <br/>, icon…).\nMirrors model.PlaceholderRun."},
	{"PcOpenRun", reflect.TypeFor[model.PcOpenRun](),
		"The opening half of a paired inline code. Mirrors model.PcOpenRun."},
	{"PcCloseRun", reflect.TypeFor[model.PcCloseRun](),
		"The closing half of a paired inline code (shares `id` with its PcOpen).\nMirrors model.PcCloseRun."},
	{"SubRun", reflect.TypeFor[model.SubRun](),
		"A reference to a subblock (sub-filter output). Mirrors model.SubRun."},
	{"PluralRun", reflect.TypeFor[model.PluralRun](),
		"A structured plural construct, keyed by ICU plural form.\nMirrors model.PluralRun."},
	{"SelectRun", reflect.TypeFor[model.SelectRun](),
		"A structured select construct, keyed by arbitrary case values.\nMirrors model.SelectRun."},
	{"RunPos", reflect.TypeFor[model.RunPos](),
		"A character boundary in a run sequence: an index into the sequence and a\nrune offset into that run's text. Mirrors model.RunPos."},
	{"Anchor", reflect.TypeFor[model.Anchor](),
		"Where inside a block something is: the whole block, one run, a span of\ncharacters, or a branch of a structured run. Positions are run-relative so\nthey survive edits to neighbouring runs, and pathed so a position inside a\nplural form or select case is addressable.\nMirrors model.Anchor."},
	{"ContentTree", reflect.TypeFor[editor.ContentTree](),
		"The hierarchical anatomy view of a document's Part stream, a labeled\nPROJECTION of the content model (AD-034) rather than a wire contract. Its run\npayloads use the model's canonical Run JSON (the `Run` union here).\nMirrors core/editor.ContentTree."},
	{"ContentNode", reflect.TypeFor[editor.ContentNode](),
		"One node in a ContentTree: container kinds (layer, group) carry\n`children`; leaf kinds (block, data, media) carry their own payload.\nMirrors core/editor.ContentNode."},
	{"ContentStats", reflect.TypeFor[editor.ContentStats](),
		"Document-wide part counts. Mirrors core/editor.ContentStats."},
	{"SegmentSpan", reflect.TypeFor[editor.SegmentSpan](),
		"A segment boundary as a half-open run-index range [start, end).\nMirrors core/editor.SegmentSpan."},
	{"TargetMeta", reflect.TypeFor[editor.TargetMeta](),
		"Lifecycle + provenance metadata for one committed target variant.\nMirrors core/editor.TargetMeta."},
	{"OriginView", reflect.TypeFor[editor.OriginView](),
		"Provenance of a committed translation. Mirrors core/editor.OriginView\n(model.Origin on the wire)."},
	{"OverlayView", reflect.TypeFor[editor.OverlayView](),
		"A typed stand-off interpretation layered over one side of a block.\nMirrors core/editor.OverlayView."},
	{"OverlaySpanView", reflect.TypeFor[editor.OverlaySpanView](),
		"One run-anchored span of an overlay, with its extracted text.\nMirrors core/editor.OverlaySpanView."},
	{"AnnotationView", reflect.TypeFor[editor.AnnotationView](),
		"A block-level annotation (alt-translation, note, or generic).\nMirrors core/editor.AnnotationView."},
	{"StructureView", reflect.TypeFor[editor.StructureView](),
		"The block's structural role: semantic role, layout layer, level,\nreading order. Mirrors core/editor.StructureView."},
	{"RelationView", reflect.TypeFor[editor.RelationView](),
		"A cross-block edge (caption-of / footnote-of / label-for / …).\nMirrors core/editor.RelationView."},
	{"GeometryView", reflect.TypeFor[editor.GeometryView](),
		"Where the block sits on a rendered page: page number + bounding box.\nMirrors core/editor.GeometryView."},
	{"GlyphView", reflect.TypeFor[editor.GlyphView](),
		"One character's text and box (GeometryView.glyphs).\nMirrors core/editor.GlyphView."},
	{"TimingView", reflect.TypeFor[editor.TimingView](),
		"The block's temporal anchor on a timed source: [startMs, endMs) plus an\noptional source cue reference. Mirrors core/editor.TimingView."},
	{"MediaView", reflect.TypeFor[editor.MediaView](),
		"Structured descriptor of the binary media a node carries or refers to.\nMirrors core/editor.MediaView."},
	{"RenderNode", reflect.TypeFor[projection.RenderNode](),
		"The normalized generative-projection render AST built in Go\n(core/projection), shipped on ContentTree.render.\nMirrors core/projection.RenderNode."},
	{"GeometryAnnotation", reflect.TypeFor[model.GeometryAnnotation](),
		"Block-scoped page geometry (RenderNode.geometry).\nMirrors model.GeometryAnnotation."},
	{"Rect", reflect.TypeFor[model.Rect](),
		"An axis-aligned bounding box. Mirrors model.Rect."},
	{"GlyphBox", reflect.TypeFor[model.GlyphBox](),
		"One character's text and bounding box (GeometryAnnotation.glyphs).\nMirrors model.GlyphBox."},
}

// emitContent renders content.gen.ts as a string.
func emitContent() (string, error) {
	var b strings.Builder
	b.WriteString("// GENERATED by scripts/gen-contract-types. DO NOT EDIT.\n")
	b.WriteString("// Regenerate with `make generate-contract-types`; the drift gate is\n")
	b.WriteString("// `make check-contract-types` (mirrors check-reference-docs).\n")
	b.WriteString("//\n")
	b.WriteString("// Content-model types from the two Go sources of truth (AD-034):\n")
	b.WriteString("//   1. Wire shapes: the neokapi.content.v1 proto\n")
	b.WriteString("//      (core/proto/content/v1/content.proto) as encoded by the canonical\n")
	b.WriteString("//      protojson (contentv1.MarshalJSON): lowerCamelCase names, unpopulated\n")
	b.WriteString("//      fields omitted, bytes as base64 strings. The golden files under\n")
	b.WriteString("//      core/proto/content/v1/testdata lock this encoding.\n")
	b.WriteString("//   2. Projection shapes: the model.Run JSON (RFC 0001; flat\n")
	b.WriteString("//      `{\"text\":\"…\"}` text runs) and the core/editor ContentTree family,\n")
	b.WriteString("//      reflected from the Go structs. ContentTree is a labeled projection,\n")
	b.WriteString("//      not a wire contract.\n")

	b.WriteString("\n// ── Wire shapes (neokapi.content.v1 protojson) ──────────────────────────────\n")
	wire, err := emitWireShapes()
	if err != nil {
		return "", err
	}
	b.WriteString(wire)

	b.WriteString("\n// ── Projection shapes (model.Run JSON + core/editor ContentTree) ────────────\n")
	b.WriteString(emitModelRun())
	b.WriteString(emitRunPathStep())
	for _, e := range projectionTypes {
		iface, err := renderInterface(e)
		if err != nil {
			return "", fmt.Errorf("render %s: %w", e.name, err)
		}
		b.WriteString("\n")
		b.WriteString(iface)
	}
	return b.String(), nil
}

// emitRunPathStep hand-emits the model.RunPathStep union. Reflection sees a
// struct with a discriminator field; the JSON is one of three shapes, because a
// path reads as the walk it describes rather than as a record of tagged unions.
func emitRunPathStep() string {
	return `
/**
 * One step of a RunPath. Discriminated by shape:
 * - ` + "`number`" + `: index into a Run[] sequence.
 * - ` + "`{ plural: PluralForm }`" + `: step into a plural run's form.
 * - ` + "`{ select: string }`" + `: step into a select run's case.
 * Mirrors model.RunPathStep.
 */
export type RunPathStep = number | { plural: string } | { select: string };
`
}

// emitModelRun hand-emits the model.Run discriminated union. Reflection cannot
// derive it: model.Run has a custom MarshalJSON where text runs serialize flat
// (`{"text":"literal"}`, AD-002) while every other kind nests its payload
// under the discriminator key.
func emitModelRun() string {
	return `
/**
 * The model's canonical Run JSON (RFC 0001): an object with exactly one
 * discriminator key. Text runs serialize flat, as ` + "`" + `{"text":"literal"}` + "`" + `, per
 * Framework AD-002; every other kind nests its payload struct.
 * Mirrors model.Run.MarshalJSON. This is the run form used by ContentTree,
 * KBF, and flow traces; the wire form is the RunMessage union above.
 *
 * ` + "`noTranslate`" + ` rides beside ` + "`text`" + ` rather than nesting, so the flat shape
 * holds. It marks a run whose text reads as prose but is not: the contents of
 * a code span, a <kbd>, a <samp>, and is written only when set, so a document
 * with no protected run serializes as it always did. A consumer that rebuilds
 * runs must carry it: dropping it turns a command into something a reader
 * cannot type.
 */
export type Run =
  | { text: string; noTranslate?: boolean }
  | { ph: PlaceholderRun }
  | { pcOpen: PcOpenRun }
  | { pcClose: PcCloseRun }
  | { sub: SubRun }
  | { plural: PluralRun }
  | { select: SelectRun };
`
}

// emitWireShapes walks the neokapi.content.v1 file descriptor in declaration
// order and renders each message. A message whose fields all belong to a
// single oneof renders as a discriminated union; everything else renders as
// an interface with optional properties (canonical protojson omits
// unpopulated fields).
func emitWireShapes() (string, error) {
	var b strings.Builder
	msgs := contentv1.File_core_proto_content_v1_content_proto.Messages()
	for i := range msgs.Len() {
		md := msgs.Get(i)
		b.WriteString("\n")
		rendered, err := renderWireMessage(md)
		if err != nil {
			return "", fmt.Errorf("render %s: %w", md.Name(), err)
		}
		b.WriteString(rendered)
	}
	return b.String(), nil
}

// oneofCoversAll reports whether every field of md belongs to one oneof.
func oneofCoversAll(md protoreflect.MessageDescriptor) bool {
	if md.Oneofs().Len() != 1 {
		return false
	}
	return md.Oneofs().Get(0).Fields().Len() == md.Fields().Len()
}

func renderWireMessage(md protoreflect.MessageDescriptor) (string, error) {
	name := string(md.Name())
	doc := fmt.Sprintf("`neokapi.content.v1.%s` in its canonical protojson form.", name)
	if oneofCoversAll(md) {
		var b strings.Builder
		b.WriteString(blockComment(doc + "\nExactly one discriminator key is present per value."))
		fmt.Fprintf(&b, "export type %s =\n", name)
		fields := md.Oneofs().Get(0).Fields()
		for i := range fields.Len() {
			f := fields.Get(i)
			ts, err := wireTSType(f)
			if err != nil {
				return "", fmt.Errorf("field %s: %w", f.Name(), err)
			}
			sep := ";"
			if i < fields.Len()-1 {
				sep = ""
			}
			fmt.Fprintf(&b, "  | { %s: %s }%s\n", f.JSONName(), ts, sep)
		}
		return b.String(), nil
	}

	var b strings.Builder
	b.WriteString(blockComment(doc))
	fmt.Fprintf(&b, "export interface %s {\n", name)
	fields := md.Fields()
	for i := range fields.Len() {
		f := fields.Get(i)
		ts, err := wireTSType(f)
		if err != nil {
			return "", fmt.Errorf("field %s: %w", f.Name(), err)
		}
		// Every field is optional: canonical protojson omits unpopulated fields.
		fmt.Fprintf(&b, "  %s?: %s;\n", tsKey(f.JSONName()), ts)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// wireTSType maps a proto field to its TS rendering under canonical protojson.
func wireTSType(f protoreflect.FieldDescriptor) (string, error) {
	if f.IsMap() {
		// protojson map keys are always JSON strings.
		val, err := wireScalarTSType(f.MapValue())
		if err != nil {
			return "", err
		}
		return "Record<string, " + val + ">", nil
	}
	elem, err := wireScalarTSType(f)
	if err != nil {
		return "", err
	}
	if f.IsList() {
		return elem + "[]", nil
	}
	return elem, nil
}

func wireScalarTSType(f protoreflect.FieldDescriptor) (string, error) {
	switch f.Kind() {
	case protoreflect.BoolKind:
		return "boolean", nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
		protoreflect.FloatKind, protoreflect.DoubleKind:
		return "number", nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		// protojson encodes 64-bit integers as JSON strings.
		return "string", nil
	case protoreflect.StringKind:
		return "string", nil
	case protoreflect.BytesKind:
		// protojson encodes bytes as base64 strings.
		return "string", nil
	case protoreflect.MessageKind:
		return string(f.Message().Name()), nil
	default:
		return "", fmt.Errorf("unsupported proto kind %s", f.Kind())
	}
}
