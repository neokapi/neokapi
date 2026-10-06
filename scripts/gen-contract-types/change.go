package main

// change.go emits packages/contract-types/src/change.gen.ts: the change
// contract (E-09) as every TypeScript client of the change service reads it.
//
// What a sender writes, the change set (kapi.change/v1), is rendered from the
// JSON Schema core/change/changeschema generates from the Go types, so the
// TypeScript states the structure the decoder accepts: each operation is a
// member of a union discriminated by `op`, a choice of exactly one field (text
// or runs; a find, a span or a range) is a union that refuses both, a run or
// a path step holds the fields of one kind, and a runs payload cannot carry
// native data. Patterns, bounds and rules between fields are documented, and
// the decoder alone checks them. What the service answers (the result,
// kapi.change-result/v1, a read page and a format's description) is reflected
// from the structs it marshals, with the doc comments of their Go
// declarations, read from the source.

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changeschema"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// schemaName is the declaration a schema node becomes: its name, and the Go
// type it mirrors, whose doc comment (and, for an enumeration, whose
// constants' docs) the declaration carries.
type schemaName struct {
	name   string
	goType reflect.Type
}

// changeSchemaNames names the schema nodes that become declarations of their
// own, by location. A location is the walk from the root: a property by name
// (dot-separated), [] for an array's items, {} for a map's values, {key} for
// its keys, |label for a oneOf or anyOf member (the member's op, else its one
// required property, else its index), and $name for a definition. Each
// operation is named after its op (SetContentOp) without an entry here. A
// node shaped like a named one renders as that name wherever it appears.
// An entry the schema no longer has fails the generator.
var changeSchemaNames = map[string]schemaName{
	"":                                 {"ChangeSet", reflect.TypeFor[change.Set]()},
	"mode":                             {"ChangeMode", reflect.TypeFor[change.Mode]()},
	"gate":                             {"ChangeGate", reflect.TypeFor[change.Gate]()},
	"evidence[]":                       {"ChangeEvidence", reflect.TypeFor[change.Evidence]()},
	"ops[]":                            {"ChangeOp", reflect.TypeFor[change.Op]()},
	"ops[]|replace_text.edits[]":       {"TextEdit", reflect.TypeFor[change.TextEdit]()},
	"ops[]|replace_text.edits[].range": {"RunRange", reflect.TypeFor[change.Span]()},
	"ops[]|mark.range":                 {"TextSelection", reflect.TypeFor[change.Selection]()},
	"ops[]|annotate.anchor":            {"ChangeAnchor", nil},
	"ops[]|insert_block.editions{}":    {"EditionContent", reflect.TypeFor[change.Content]()},
	"ops[]|decide.outcome":             {"DecideOutcome", reflect.TypeFor[change.Outcome]()},
	"ops[]|term.action":                {"TermAction", nil},
	"ops[]|memory.action":              {"MemoryAction", nil},
	"ops[]|memory.from":                {"MemoryText", reflect.TypeFor[change.MemoryText]()},
	"$ref":                             {"ChangeRef", nil},
	"$block":                           {"BlockRef", nil},
	"$path":                            {"RunPath", nil},
	"$path[]":                          {"PathStep", nil},
	"$path[]|plural.plural":            {"PluralForm", nil},
	"$run":                             {"ChangeRun", nil},
	"$run|ph.ph":                       {"InlineCode", nil},
	"$run|ph.ph.constraints":           {"CodeConstraints", nil},
	"$run|pcClose.pcClose":             {"ClosingCode", nil},
	"$run|sub.sub":                     {"SubblockCode", nil},
	"$run|plural.plural":               {"PluralBranches", nil},
	"$run|select.select":               {"SelectBranches", nil},
}

// changeExternalShapes are types content.gen.ts already declares, by the Go
// type both render: a schema node of the same shape renders as the name and
// is imported rather than declared again.
var changeExternalShapes = []emitType{
	{"RunPos", reflect.TypeFor[model.RunPos](), ""},
}

// changeResultTypes, changeReadTypes, changeDescribeTypes and
// changeHistoryTypes are the structs the service marshals, in the order they
// are declared.
var (
	changeResultTypes = []emitType{
		{"ChangeResult", reflect.TypeFor[change.Result](), ""},
		{"DocResult", reflect.TypeFor[change.DocResult](), ""},
		{"OpResult", reflect.TypeFor[change.OpResult](), ""},
		{"ChangeFinding", reflect.TypeFor[change.Finding](), ""},
		{"ResolvedSpan", reflect.TypeFor[change.Resolved](), ""},
		{"ResultPosition", reflect.TypeFor[change.Position](), ""},
		{"Invalidation", reflect.TypeFor[change.Invalidation](), ""},
		{"CurrentEdition", reflect.TypeFor[change.Current](), ""},
		{"ChangeError", reflect.TypeFor[change.Error](), ""},
		{"ErrorCandidate", reflect.TypeFor[change.Candidate](), ""},
		{"FindSearched", reflect.TypeFor[change.Searched](), ""},
	}
	changeReadTypes = []emitType{
		{"ReadRequest", reflect.TypeFor[change.ReadRequest](), ""},
		{"ReadPage", reflect.TypeFor[change.Page](), ""},
		{"DivergentWrite", reflect.TypeFor[change.DivergentWrite](), ""},
		{"BlockRead", reflect.TypeFor[change.BlockRead](), ""},
		{"CodeRead", reflect.TypeFor[change.CodeRead](), ""},
		{"StructureRead", reflect.TypeFor[change.StructureRead](), ""},
		{"EditionRead", reflect.TypeFor[change.EditionRead](), ""},
	}
	changeDescribeTypes = []emitType{
		{"DescribeRequest", reflect.TypeFor[change.DescribeRequest](), ""},
		{"FormatDescription", reflect.TypeFor[change.Description](), ""},
		{"FormatOpTable", reflect.TypeFor[change.OpTable](), ""},
		{"SetContentSupport", reflect.TypeFor[change.SetContentSupport](), ""},
		{"MarkSupport", reflect.TypeFor[change.MarkSupport](), ""},
		{"AnnotateSupport", reflect.TypeFor[change.AnnotateSupport](), ""},
		{"OpSupported", reflect.TypeFor[change.Supported](), ""},
		{"FormatNativeOp", reflect.TypeFor[format.NativeOp](), ""},
	}
	changeHistoryTypes = []emitType{
		{"HistoryRequest", reflect.TypeFor[change.HistoryRequest](), ""},
		{"EditionHistory", reflect.TypeFor[change.History](), ""},
		{"HistoryEntry", reflect.TypeFor[change.HistoryEntry](), ""},
		{"ChangeActor", reflect.TypeFor[change.Actor](), ""},
	}
)

// changeUnion is a Go string type rendered as a union of its constants.
type changeUnion struct {
	name     string
	typ      reflect.Type
	constant string
}

// changeUnions are the string types the result, read and description name.
// ChangeOpKind is listed apart: its members are change.Kinds(), which leaves
// out the in-process provenance operation.
var changeUnions = []changeUnion{
	{"ChangeSetStatus", reflect.TypeFor[change.SetStatus](), "CHANGE_SET_STATUSES"},
	{"ChangeOpStatus", reflect.TypeFor[change.OpStatus](), "CHANGE_OP_STATUSES"},
	{"ChangeErrorCode", reflect.TypeFor[change.Code](), "CHANGE_ERROR_CODES"},
	{"ChangeGuardSubcode", reflect.TypeFor[change.Subcode](), "CHANGE_GUARD_SUBCODES"},
	{"FormatEditions", reflect.TypeFor[change.Editions](), "FORMAT_EDITIONS"},
	{"ChangeActorKind", reflect.TypeFor[change.ActorKind](), "CHANGE_ACTOR_KINDS"},
}

// changeFieldTypes overrides the reflected type of a field where the Go type
// says less than the contract does. Each entry must name a field that exists.
var changeFieldTypes = map[fieldKey]string{
	// The result names its schema; Result.Schema is always ResultSchemaID.
	{reflect.TypeFor[change.Result](), "Schema"}: "typeof CHANGE_RESULT_SCHEMA_ID",
	// A read's reference always names its block (readBlock sets it from
	// BlockKey), so it is what an operation's at takes; a result's reference
	// to a document alone leaves the block out (ResultRef).
	{reflect.TypeFor[change.BlockRead](), "Ref"}: "ChangeRef",
	// A contested block names its block always (Documents.Rebase sets it).
	{reflect.TypeFor[change.DivergentWrite](), "Contested"}: "ChangeRef[]",
	// A nil map marshals as null: a format that writes no attribute.
	{reflect.TypeFor[change.OpTable](), "SetAttribute"}: "Record<string, string[]> | null",
	// An operation's result names its kind, which may be one a tool applies
	// in process; what a block accepts (BlockRead.Ops) is a ChangeOpKind.
	{reflect.TypeFor[change.OpResult](), "Op"}: "ResultOpKind",
	// A history names a block always (Service.History reads it), so its
	// reference is what an operation's at takes.
	{reflect.TypeFor[change.HistoryRequest](), "Ref"}: "ChangeRef",
	{reflect.TypeFor[change.History](), "Ref"}:        "ChangeRef",
}

// fieldKey names a struct field.
type fieldKey struct {
	typ   reflect.Type
	field string
}

// rawMessageType renders as unknown: any JSON value.
var rawMessageType = reflect.TypeFor[json.RawMessage]()

// emitChange renders change.gen.ts as a string.
func emitChange() (string, error) {
	docs, err := parseGoDocs("core/change", "core/format")
	if err != nil {
		return "", err
	}
	root, err := decodeOrdered(changeschema.Schema())
	if err != nil {
		return "", fmt.Errorf("decode the change-set schema: %w", err)
	}

	st := newSchemaTS(root, docs)
	request, err := st.emit()
	if err != nil {
		return "", fmt.Errorf("render the change-set schema: %w", err)
	}
	vocab, err := emitChangeVocabularies(docs, root)
	if err != nil {
		return "", err
	}

	g := &goTS{docs: docs, imported: st.imported, external: st.external, names: map[reflect.Type]string{
		reflect.TypeFor[change.Ref]():    "ResultRef",
		reflect.TypeFor[model.RunPath](): "RunPath",
		reflect.TypeFor[change.Kind]():   "ChangeOpKind",
		// A time marshals as RFC 3339 text.
		reflect.TypeFor[time.Time](): "string",
	}, used: map[fieldKey]bool{}}
	for _, e := range changeExternalShapes {
		g.names[e.typ] = e.name
	}
	for _, u := range changeUnions {
		g.names[u.typ] = u.name
	}
	declared := slices.Clone(st.declared)
	declared = append(declared, "ResultRef", "ChangeOpKind", "ResultOpKind")
	for _, u := range changeUnions {
		declared = append(declared, u.name)
	}
	sections := []struct {
		title string
		types []emitType
	}{
		{"The result (kapi.change-result/v1)", changeResultTypes},
		{"A read page (Service.Read)", changeReadTypes},
		{"What a format supports (Service.Describe)", changeDescribeTypes},
		{"An edition's recorded changes (Service.History)", changeHistoryTypes},
	}
	for _, sec := range sections {
		for _, e := range sec.types {
			g.names[e.typ] = e.name
			declared = append(declared, e.name)
		}
	}
	slices.Sort(declared)
	for i := 1; i < len(declared); i++ {
		if declared[i] == declared[i-1] {
			return "", fmt.Errorf("two declarations are named %s", declared[i])
		}
	}

	var replies strings.Builder
	for i, sec := range sections {
		fmt.Fprintf(&replies, "\n// ── %s %s\n", sec.title, strings.Repeat("─", max(3, 72-len(sec.title))))
		if i == 0 {
			ref, err := emitResultRef(docs)
			if err != nil {
				return "", err
			}
			replies.WriteString("\n")
			replies.WriteString(ref)
		}
		for _, e := range sec.types {
			decl, err := g.declare(e)
			if err != nil {
				return "", fmt.Errorf("render %s: %w", e.name, err)
			}
			replies.WriteString("\n")
			replies.WriteString(decl)
		}
	}
	for k := range changeFieldTypes {
		if !g.used[k] {
			return "", fmt.Errorf("changeFieldTypes names %s.%s, which no emitted type has", k.typ, k.field)
		}
	}

	var b strings.Builder
	b.WriteString("// GENERATED by scripts/gen-contract-types. DO NOT EDIT.\n")
	b.WriteString("// Regenerate with `make generate-contract-types`; the drift gate is\n")
	b.WriteString("// `make check-contract-types` (mirrors check-reference-docs).\n")
	b.WriteString("//\n")
	b.WriteString("// The change contract (E-09): what a client of the change service sends and\n")
	b.WriteString("// what the service answers. The change set (kapi.change/v1) is rendered from\n")
	b.WriteString("// the JSON Schema core/change/changeschema generates from the Go types, so\n")
	b.WriteString("// these types refuse the structural mistakes the decoder refuses; a pattern,\n")
	b.WriteString("// a bound or a rule between fields is documented and checked by the decoder\n")
	b.WriteString("// alone. The result (kapi.change-result/v1), a read page, a format's\n")
	b.WriteString("// description and an edition's history are reflected from the core/change\n")
	b.WriteString("// structs the service marshals, with the doc comments of their Go\n")
	b.WriteString("// declarations.\n\n")
	if imports := st.importList(); len(imports) > 0 {
		fmt.Fprintf(&b, "import type { %s } from \"./content.gen.ts\";\n\n", strings.Join(imports, ", "))
	}
	b.WriteString("/** The contract version a change set follows. Mirrors core/change.SchemaID. */\n")
	fmt.Fprintf(&b, "export const CHANGE_SCHEMA_ID = %s;\n\n", jsonString(change.SchemaID))
	b.WriteString("/** The version of the result shape. Mirrors core/change.ResultSchemaID. */\n")
	fmt.Fprintf(&b, "export const CHANGE_RESULT_SCHEMA_ID = %s;\n", jsonString(change.ResultSchemaID))
	b.WriteString("\n// ── The change set (kapi.change/v1), from changeschema.Schema() ──────────\n")
	b.WriteString(request)
	b.WriteString("\n// ── Vocabularies ─────────────────────────────────────────────────────────\n")
	b.WriteString(vocab)
	b.WriteString(replies.String())
	return b.String(), nil
}

// emitChangeVocabularies renders the unions of operation kinds, statuses,
// error codes and subcodes, each with a frozen list of its members, and the
// transport mappings of the error codes.
func emitChangeVocabularies(docs *goDocs, root *jnode) (string, error) {
	var b strings.Builder

	kinds := change.Kinds()
	var schemaKinds []string
	for _, m := range root.get("properties").get("ops").get("items").get("oneOf").arr {
		schemaKinds = append(schemaKinds, m.get("properties").get("op").str("const"))
	}
	var members []unionMember
	for _, spec := range change.Operations() {
		members = append(members, unionMember{value: string(spec.Kind), doc: spec.Summary})
	}
	if got := unionValues(members); !slices.Equal(got, kindStrings(kinds)) || !slices.Equal(got, schemaKinds) {
		return "", fmt.Errorf("change.Operations %v, change.Kinds %v and the schema's operations %v disagree", got, kinds, schemaKinds)
	}
	b.WriteString(renderUnion("ChangeOpKind",
		docs.typeDoc(reflect.TypeFor[change.Kind]())+"\n\nThe operations a change set may carry, in the order the schema lists them;\nChangeOp[\"op\"] is the same union.",
		members, "CHANGE_OP_KINDS"))
	resultKinds, err := renderResultOpKind(docs, kinds)
	if err != nil {
		return "", err
	}
	b.WriteString(resultKinds)

	for _, u := range changeUnions {
		consts, err := docs.stringConsts(u.typ)
		if err != nil {
			return "", err
		}
		var members []unionMember
		for _, c := range consts {
			members = append(members, unionMember{value: c.value, doc: constDoc(c)})
		}
		b.WriteString(renderUnion(u.name, docs.typeDoc(u.typ)+"\n\nMirrors "+goPath(u.typ)+".", members, u.constant))
	}

	codes := change.Codes()
	codeConsts, err := docs.stringConsts(reflect.TypeFor[change.Code]())
	if err != nil {
		return "", err
	}
	if got := unionValues(constMembers(codeConsts)); !slices.Equal(got, codeStrings(codes)) {
		return "", fmt.Errorf("the source's Code constants %v and change.Codes %v disagree", got, codes)
	}
	var http, exit strings.Builder
	for _, c := range codes {
		fmt.Fprintf(&http, "  %s: %d,\n", tsKey(string(c)), c.HTTPStatus())
		fmt.Fprintf(&exit, "  %s: %d,\n", tsKey(string(c)), c.ExitCode())
	}
	b.WriteString("\n/** The HTTP status a transport answers each refusal with. Mirrors core/change.Code.HTTPStatus. */\n")
	fmt.Fprintf(&b, "export const CHANGE_ERROR_HTTP_STATUS: Readonly<Record<ChangeErrorCode, number>> = {\n%s};\n", http.String())
	b.WriteString("\n/** The exit code a command line returns for each refusal. Mirrors core/change.Code.ExitCode. */\n")
	fmt.Fprintf(&b, "export const CHANGE_ERROR_EXIT_CODE: Readonly<Record<ChangeErrorCode, number>> = {\n%s};\n", exit.String())
	return b.String(), nil
}

// renderResultOpKind renders ResultOpKind, the operation an OpResult names:
// any a change set may carry, and each Kind constant the schema leaves out,
// which a tool in a flow applies in process through change.ApplyBlock.
func renderResultOpKind(docs *goDocs, public []change.Kind) (string, error) {
	all, err := docs.stringConsts(reflect.TypeFor[change.Kind]())
	if err != nil {
		return "", err
	}
	var values []string
	var inProcess []unionMember
	for _, c := range all {
		values = append(values, c.value)
		if !slices.Contains(public, change.Kind(c.value)) {
			inProcess = append(inProcess, unionMember{value: c.value, doc: constDoc(c)})
		}
	}
	for _, k := range public {
		if !slices.Contains(values, string(k)) {
			return "", fmt.Errorf("change.Kinds names %s, which no Kind constant in the source spells", k)
		}
	}
	doc := "The operation an OpResult names: one a change set may carry, or one a tool\nin a flow applies in process, which no change set carries."
	members := []string{"ChangeOpKind"}
	var list []string
	for _, m := range inProcess {
		members = append(members, jsonString(m.value))
		list = append(list, "- `"+m.value+"`: "+oneLine(m.doc))
	}
	if len(list) > 0 {
		doc += "\n\n" + strings.Join(list, "\n")
	}
	return "\n" + jsDoc(doc, "") + "export type ResultOpKind =" + spaced(wrapUnion(members, "")) + ";\n", nil
}

// unionMember is one member of a rendered union.
type unionMember struct {
	value string
	doc   string
}

func unionValues(ms []unionMember) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.value
	}
	return out
}

func constMembers(cs []goConst) []unionMember {
	out := make([]unionMember, len(cs))
	for i, c := range cs {
		out[i] = unionMember{value: c.value}
	}
	return out
}

func kindStrings(ks []change.Kind) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = string(k)
	}
	return out
}

func codeStrings(cs []change.Code) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = string(c)
	}
	return out
}

// renderUnion renders `export type Name = "a" | "b";` with a doc listing each
// member, and, given constName, a frozen const listing them in order.
func renderUnion(name, doc string, members []unionMember, constName string) string {
	var b strings.Builder
	b.WriteString("\n")
	var lines []string
	if doc != "" {
		lines = append(lines, doc)
	}
	var list []string
	for _, m := range members {
		if m.doc != "" {
			list = append(list, "- `"+m.value+"`: "+oneLine(m.doc))
		}
	}
	if len(list) > 0 {
		lines = append(lines, strings.Join(list, "\n"))
	}
	if len(lines) > 0 {
		b.WriteString(jsDoc(strings.Join(lines, "\n\n"), ""))
	}
	quoted := make([]string, len(members))
	for i, m := range members {
		quoted[i] = jsonString(m.value)
	}
	fmt.Fprintf(&b, "export type %s =%s;\n", name, spaced(wrapUnion(quoted, "")))
	if constName != "" {
		fmt.Fprintf(&b, "export const %s: readonly %s[] = [%s];\n", constName, name, strings.Join(quoted, ", "))
	}
	return b.String()
}

// emitResultRef renders ResultRef, the JSON change.Ref writes. Ref marshals
// itself, so its shape is read from what it writes: every key a full
// reference writes, optional where an empty reference leaves it out.
func emitResultRef(docs *goDocs) (string, error) {
	fr, err := model.ParseEditionKey("fr")
	if err != nil {
		return "", err
	}
	full, err := json.Marshal(change.Ref{Doc: "d", Block: "b", Edition: fr})
	if err != nil {
		return "", err
	}
	empty, err := json.Marshal(change.Ref{})
	if err != nil {
		return "", err
	}
	f, err := decodeOrdered(full)
	if err != nil {
		return "", err
	}
	e, err := decodeOrdered(empty)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	t := reflect.TypeFor[change.Ref]()
	b.WriteString(jsDoc(docs.typeDoc(t)+"\n\nThe JSON "+goPath(t)+" writes, as a result and a finding name what\nthey are about: a key it leaves out when empty is optional here. A read's\nreference always names its block, and is a ChangeRef.", ""))
	b.WriteString("export interface ResultRef {\n")
	for _, k := range f.keys {
		if _, ok := f.obj[k].val.(string); !ok {
			return "", fmt.Errorf("change.Ref writes %q as %v, not a string", k, f.obj[k].val)
		}
		q := ""
		if !e.has(k) {
			q = "?"
		}
		fmt.Fprintf(&b, "  %s%s: string;\n", tsKey(k), q)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// ── Reflected structs ──────────────────────────────────────────────────────

// goTS renders Go structs as TypeScript, naming each type the way the
// generated file declares it.
type goTS struct {
	docs  *goDocs
	names map[reflect.Type]string
	used  map[fieldKey]bool
	// external names are declared in content.gen.ts; one a field uses is
	// added to imported.
	external map[string]bool
	imported map[string]bool
}

// declare renders one struct as an interface, with its Go doc.
func (g *goTS) declare(e emitType) (string, error) {
	t := e.typ
	if t.Kind() != reflect.Struct {
		return "", fmt.Errorf("%s is not a struct", t)
	}
	doc := g.docs.typeDoc(t) + "\n\nMirrors " + goPath(t) + "."
	var fields []string
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}
		if f.Anonymous {
			return "", fmt.Errorf("embedded field %s is unsupported", f.Name)
		}
		name, opts := parseJSONTag(f)
		if name == "-" {
			continue
		}
		omit := hasOption(opts, "omitempty") || hasOption(opts, "omitzero")
		key := fieldKey{t, f.Name}
		ts, override := changeFieldTypes[key]
		if override {
			g.used[key] = true
		} else {
			var err error
			ts, err = g.tsOf(f.Type)
			if err != nil {
				return "", fmt.Errorf("field %s: %w", f.Name, err)
			}
			// A nil pointer is written as null unless the tag leaves it out.
			if f.Type.Kind() == reflect.Pointer && !omit {
				ts += " | null"
			}
		}
		q := ""
		if omit {
			q = "?"
		}
		var fb strings.Builder
		if d := g.docs.fieldDoc(t, f.Name); d != "" {
			fb.WriteString(jsDoc(d, "  "))
		}
		fmt.Fprintf(&fb, "  %s%s: %s;\n", tsKey(name), q, ts)
		fields = append(fields, fb.String())
	}
	var b strings.Builder
	b.WriteString(jsDoc(doc, ""))
	if len(fields) == 0 {
		fmt.Fprintf(&b, "export type %s = Record<string, never>;\n", e.name)
		return b.String(), nil
	}
	fmt.Fprintf(&b, "export interface %s {\n%s}\n", e.name, strings.Join(fields, ""))
	return b.String(), nil
}

// tsOf maps a Go type to its TypeScript rendering. A type that marshals itself
// and has no name here fails, so a new custom encoding is never rendered as
// the Go struct it hides.
func (g *goTS) tsOf(t reflect.Type) (string, error) {
	if n, ok := g.names[t]; ok {
		if g.external[n] {
			g.imported[n] = true
		}
		return n, nil
	}
	if t == rawMessageType {
		return "unknown", nil
	}
	if t.Kind() == reflect.Pointer {
		return g.tsOf(t.Elem())
	}
	jsonMarshaler := reflect.TypeFor[json.Marshaler]()
	if t.Implements(jsonMarshaler) || reflect.PointerTo(t).Implements(jsonMarshaler) {
		return "", fmt.Errorf("%s marshals itself and has no TypeScript name", t)
	}
	if t.Implements(reflect.TypeFor[encoding.TextMarshaler]()) {
		return "string", nil
	}
	switch t.Kind() {
	case reflect.String:
		if t.PkgPath() != "" {
			return "", fmt.Errorf("string type %s has no union; add it to changeUnions", t)
		}
		return "string", nil
	case reflect.Bool:
		return "boolean", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number", nil
	case reflect.Interface:
		return "unknown", nil
	case reflect.Slice, reflect.Array:
		elem, err := g.tsOf(t.Elem())
		if err != nil {
			return "", err
		}
		return arrayOf(elem), nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return "", fmt.Errorf("map key %s is not a string", t.Key())
		}
		val, err := g.tsOf(t.Elem())
		if err != nil {
			return "", err
		}
		return "Record<string, " + val + ">", nil
	}
	return "", fmt.Errorf("%s has no TypeScript name", t)
}

// ── The change-set schema ──────────────────────────────────────────────────

// schemaTS renders the change-set JSON Schema as TypeScript declarations.
type schemaTS struct {
	root     *jnode
	docs     *goDocs
	names    map[string]schemaName // location → declaration
	nodes    map[string]*jnode     // declared name → node
	locs     map[string]string     // declared name → location
	shapes   map[string]string     // shape key → declared or imported name
	external map[string]bool       // names content.gen.ts declares
	imported map[string]bool
	// exclude holds, for a member of a union of closed objects about to be
	// rendered, the other members' fields it refuses (exclusiveKeys).
	exclude map[*jnode][]string
	// declared lists every name a declaration was rendered for.
	declared []string
	queued   map[string]bool
	queue    []string
}

// newSchemaTS renders the change-set schema root with changeSchemaNames, each
// operation named after its op.
func newSchemaTS(root *jnode, docs *goDocs) *schemaTS {
	names := maps.Clone(changeSchemaNames)
	for _, m := range root.get("properties").get("ops").get("items").get("oneOf").arr {
		kind := m.get("properties").get("op").str("const")
		names["ops[]|"+kind] = schemaName{name: pascal(kind) + "Op"}
	}
	return newSchemaRenderer(root, docs, names, changeExternalShapes)
}

// newSchemaRenderer renders the schema root with the declarations names lists
// by location; the root's own location is "". A node shaped like one of
// external renders as its name, imported.
func newSchemaRenderer(root *jnode, docs *goDocs, names map[string]schemaName, external []emitType) *schemaTS {
	s := &schemaTS{
		root:     root,
		docs:     docs,
		names:    names,
		nodes:    map[string]*jnode{},
		locs:     map[string]string{},
		shapes:   map[string]string{},
		external: map[string]bool{},
		imported: map[string]bool{},
		exclude:  map[*jnode][]string{},
		queued:   map[string]bool{},
	}
	for _, e := range external {
		s.shapes[shapeKey(flatStructShape(e.typ))] = e.name
		s.external[e.name] = true
	}
	return s
}

// importList is the content.gen.ts names the rendering used, sorted.
func (s *schemaTS) importList() []string {
	out := slices.Collect(maps.Keys(s.imported))
	slices.Sort(out)
	return out
}

// emit renders every declaration, the change set first and each named type
// after the first declaration that uses it.
func (s *schemaTS) emit() (string, error) {
	if err := s.index(s.root, ""); err != nil {
		return "", err
	}
	for loc, sn := range s.names {
		if _, ok := s.nodes[sn.name]; !ok {
			return "", fmt.Errorf("changeSchemaNames names %q at %q, which the schema has no node at", sn.name, loc)
		}
	}
	rootName, ok := s.names[""]
	if !ok {
		return "", errors.New("the schema root has no name")
	}
	var b strings.Builder
	s.ref(rootName.name)
	for len(s.queue) > 0 {
		name := s.queue[0]
		s.queue = s.queue[1:]
		decl, err := s.declare(name)
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		s.declared = append(s.declared, name)
		b.WriteString("\n")
		b.WriteString(decl)
	}
	return b.String(), nil
}

// index walks the schema and records the node of every named location and
// its shape.
func (s *schemaTS) index(n *jnode, loc string) error {
	if sn, ok := s.names[loc]; ok {
		if _, dup := s.nodes[sn.name]; dup {
			return fmt.Errorf("%s names two locations", sn.name)
		}
		s.nodes[sn.name] = n
		s.locs[sn.name] = loc
		key := shapeKey(n)
		if other, ok := s.shapes[key]; ok && other != sn.name {
			return fmt.Errorf("%s and %s have one shape; name one of them", other, sn.name)
		}
		s.shapes[key] = sn.name
	}
	return eachChild(n, loc, s.index)
}

// eachChild calls fn for every subschema of n with its location.
func eachChild(n *jnode, loc string, fn func(*jnode, string) error) error {
	if n == nil || !n.isObj {
		return nil
	}
	if props := n.get("properties"); props != nil {
		for _, k := range props.keys {
			if err := fn(props.obj[k], join(loc, k)); err != nil {
				return err
			}
		}
	}
	if items := n.get("items"); items != nil {
		if err := fn(items, loc+"[]"); err != nil {
			return err
		}
	}
	if ap := n.get("additionalProperties"); ap != nil && !isClosed(ap) {
		if err := fn(ap, loc+"{}"); err != nil {
			return err
		}
	}
	if pn := n.get("propertyNames"); pn != nil {
		if err := fn(pn, loc+"{key}"); err != nil {
			return err
		}
	}
	for _, kw := range []string{"oneOf", "anyOf"} {
		list := n.get(kw)
		if list == nil || requiredOnly(list) {
			continue
		}
		for i, m := range list.arr {
			if err := fn(m, loc+"|"+memberLabel(m, i)); err != nil {
				return err
			}
		}
	}
	if defs := n.get("$defs"); defs != nil {
		for _, k := range defs.keys {
			if err := fn(defs.obj[k], "$"+k); err != nil {
				return err
			}
		}
	}
	return nil
}

// ref returns name, queueing its declaration the first time.
func (s *schemaTS) ref(name string) string {
	if s.external[name] {
		s.imported[name] = true
		return name
	}
	if !s.queued[name] {
		s.queued[name] = true
		s.queue = append(s.queue, name)
	}
	return name
}

// declare renders the declaration of a named node. One that mirrors a Go
// type carries its doc comment, and an enumeration of a Go string type
// carries each constant's doc; its values must be the constants'.
func (s *schemaTS) declare(name string) (string, error) {
	n, loc := s.nodes[name], s.locs[name]
	sn := s.names[loc]
	doc := schemaDoc(n)
	if sn.goType != nil {
		doc = s.docs.typeDoc(sn.goType) + "\n\nMirrors " + goPath(sn.goType) + "."
		if e := n.get("enum"); e != nil {
			consts, err := s.docs.stringConsts(sn.goType)
			if err != nil {
				return "", err
			}
			var members []unionMember
			for _, c := range consts {
				members = append(members, unionMember{value: c.value, doc: constDoc(c)})
			}
			if !slices.Equal(unionValues(members), e.strings()) {
				return "", fmt.Errorf("the schema lists %v and the constants of %s are %v", e.strings(), goPath(sn.goType), unionValues(members))
			}
			return strings.TrimPrefix(renderUnion(name, doc, members, ""), "\n"), nil
		}
	}
	body, err := s.inline(n, loc, 0, "")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(jsDoc(doc, ""))
	if strings.HasPrefix(body, "{") && !strings.Contains(body, "} & (") && isObjectSchema(n) {
		fmt.Fprintf(&b, "export interface %s %s\n", name, body)
	} else {
		fmt.Fprintf(&b, "export type %s =%s;\n", name, spaced(body))
	}
	return b.String(), nil
}

// spaced puts a space before a type that continues on the line it starts on;
// a union that starts on the next line needs none.
func spaced(body string) string {
	if strings.HasPrefix(body, "\n") {
		return body
	}
	return " " + body
}

// typeOf renders the type of a node in place: the name of a named or
// same-shaped node, else the node itself.
func (s *schemaTS) typeOf(n *jnode, loc string, indent int, inherit string) (string, error) {
	if sn, ok := s.names[loc]; ok {
		return s.ref(sn.name), nil
	}
	if r := n.str("$ref"); r != "" {
		def, ok := strings.CutPrefix(r, "#/$defs/")
		name := s.names["$"+def].name
		if !ok || name == "" {
			return "", fmt.Errorf("%s: $ref %q names no declared definition", loc, r)
		}
		return s.ref(name), nil
	}
	if name, ok := s.shapes[shapeKey(n)]; ok {
		return s.ref(name), nil
	}
	return s.inline(n, loc, indent, inherit)
}

// schemaKeywords are the keywords the renderer understands. Any other fails
// the generator, so a schema feature is never dropped from the types
// silently.
var schemaKeywords = map[string]bool{
	"$schema": true, "$defs": true, "$ref": true, "title": true, "description": true,
	"type": true, "const": true, "enum": true, "pattern": true,
	"minimum": true, "maximum": true, "minItems": true,
	"properties": true, "required": true, "additionalProperties": true, "propertyNames": true,
	"items": true, "oneOf": true, "anyOf": true,
}

// inline renders a node itself. inherit is the type a oneOf or anyOf member
// takes from its parent when it names none.
func (s *schemaTS) inline(n *jnode, loc string, indent int, inherit string) (string, error) {
	if n == nil || !n.isObj {
		return "", fmt.Errorf("%s: not a schema object", loc)
	}
	for _, k := range n.keys {
		if !schemaKeywords[k] {
			return "", fmt.Errorf("%s: keyword %q is not rendered; teach change.go what it means", loc, k)
		}
	}
	if c := n.get("const"); c != nil {
		return literal(c)
	}
	if e := n.get("enum"); e != nil {
		var vals []string
		for _, v := range e.arr {
			lit, err := literal(v)
			if err != nil {
				return "", fmt.Errorf("%s: %w", loc, err)
			}
			vals = append(vals, lit)
		}
		return wrapUnion(vals, pad(indent)), nil
	}
	types := n.types()
	if len(types) == 0 && inherit != "" {
		types = []string{inherit}
	}
	parentType := ""
	if len(types) == 1 {
		parentType = types[0]
	}
	for _, kw := range []string{"oneOf", "anyOf"} {
		list := n.get(kw)
		if list == nil {
			continue
		}
		if requiredOnly(list) {
			// object renders an exactly-one choice; at least one of several
			// fields has no rendering.
			if kw == "anyOf" {
				return "", fmt.Errorf("%s: anyOf of required fields is not rendered; teach change.go what it means", loc)
			}
			continue
		}
		if n.has("properties") {
			return "", fmt.Errorf("%s: %s beside properties is not rendered; teach change.go what it means", loc, kw)
		}
		exclusive, err := s.exclusiveKeys(list)
		if err != nil {
			return "", fmt.Errorf("%s: %w", loc, err)
		}
		var members []string
		for i, m := range list.arr {
			if len(exclusive[i]) > 0 {
				s.exclude[m] = exclusive[i]
			}
			t, err := s.typeOf(m, loc+"|"+memberLabel(m, i), indent+1, parentType)
			if err != nil {
				return "", err
			}
			// A member rendered as a name keeps its declaration, and refuses
			// the other members' fields beside it.
			if keys, pending := s.exclude[m]; pending {
				t += " & { " + strings.TrimSuffix(strings.Join(neverFields(keys), " "), ";") + " }"
				delete(s.exclude, m)
			}
			members = append(members, t)
		}
		return wrapUnion(members, pad(indent)), nil
	}
	if len(types) == 0 {
		if n.has("properties") {
			types = []string{"object"}
		} else {
			return "unknown", nil
		}
	}
	var alts []string
	for _, t := range types {
		var out string
		var err error
		switch t {
		case "string":
			out = patternType(n.str("pattern"))
		case "integer", "number":
			out = "number"
		case "boolean":
			out = "boolean"
		case "null":
			out = "null"
		case "array":
			items := n.get("items")
			if items == nil {
				return "", fmt.Errorf("%s: an array with no items", loc)
			}
			var elem string
			elem, err = s.typeOf(items, loc+"[]", indent, "")
			out = arrayOf(elem)
		case "object":
			out, err = s.object(n, loc, indent)
		default:
			err = fmt.Errorf("%s: type %q is not rendered", loc, t)
		}
		if err != nil {
			return "", err
		}
		alts = append(alts, out)
	}
	return wrapUnion(alts, pad(indent)), nil
}

// object renders an object schema: a map, or properties with an exactly-one
// choice among some of them as a union that refuses the others.
func (s *schemaTS) object(n *jnode, loc string, indent int) (string, error) {
	props := n.get("properties")
	ap := n.get("additionalProperties")
	if props == nil || len(props.keys) == 0 {
		if ap == nil || isClosed(ap) {
			return "Record<string, never>", nil
		}
		val, err := s.typeOf(ap, loc+"{}", indent, "")
		if err != nil {
			return "", err
		}
		if pn := n.get("propertyNames"); pn != nil {
			key, err := s.typeOf(pn, loc+"{key}", indent, "string")
			if err != nil {
				return "", err
			}
			return "Partial<Record<" + key + ", " + val + ">>", nil
		}
		return "Record<string, " + val + ">", nil
	}
	if ap == nil || !isClosed(ap) {
		return "", fmt.Errorf("%s: an object with properties that is not closed is not rendered", loc)
	}
	required := map[string]bool{}
	for _, r := range n.get("required").strings() {
		required[r] = true
	}
	types := map[string]string{}
	var fields []string
	multiline := false
	for _, k := range props.keys {
		p := props.obj[k]
		t, err := s.typeOf(p, join(loc, k), indent+1, "")
		if err != nil {
			return "", err
		}
		types[k] = t
		q := "?"
		if required[k] {
			q = ""
		}
		doc := schemaDoc(p)
		if doc != "" || strings.Contains(t, "\n") {
			multiline = true
		}
		fields = append(fields, jsDoc(doc, pad(indent+1))+pad(indent+1)+tsKey(k)+q+": "+t+";")
	}
	// A member of a union of closed objects refuses the fields of the others.
	if keys, ok := s.exclude[n]; ok {
		for _, f := range neverFields(keys) {
			fields = append(fields, pad(indent+1)+f)
		}
		delete(s.exclude, n)
	}
	var body string
	single := "{ " + strings.TrimSuffix(strings.Join(trimAll(fields), " "), ";") + " }"
	// A declaration's own fields go one per line; a nested object short
	// enough and without docs stays on one.
	if !multiline && len(single) <= 80 && indent > 0 {
		body = single
	} else {
		body = "{\n" + strings.Join(fields, "\n") + "\n" + pad(indent) + "}"
	}
	variants := n.get("oneOf")
	if variants == nil || !requiredOnly(variants) {
		return body, nil
	}
	// Exactly one of the variants' fields: each variant requires its own and
	// refuses the others'.
	var choice []string
	for _, v := range variants.arr {
		for _, r := range v.get("required").strings() {
			if !slices.Contains(choice, r) {
				choice = append(choice, r)
			}
		}
	}
	var alts []string
	for _, v := range variants.arr {
		own := v.get("required").strings()
		var parts []string
		for _, k := range own {
			t, ok := types[k]
			if !ok {
				return "", fmt.Errorf("%s: oneOf requires %q, which is not a property", loc, k)
			}
			parts = append(parts, tsKey(k)+": "+t)
		}
		for _, k := range choice {
			if !slices.Contains(own, k) {
				parts = append(parts, tsKey(k)+"?: never")
			}
		}
		alts = append(alts, "{ "+strings.Join(parts, "; ")+" }")
	}
	return body + " & (\n" + pad(indent+1) + "| " + strings.Join(alts, "\n"+pad(indent+1)+"| ") + "\n" + pad(indent) + ")", nil
}

// exclusiveKeys lists, for each member of a oneOf or anyOf, the fields of the
// other members it lacks, when two or more members are closed objects told
// apart by which fields they hold rather than by a constant (a run is text,
// ph, pcOpen and so on). A value holding the fields of two such members
// matches neither, and the decoder refuses it, but TypeScript checks excess
// properties against the union as a whole, so each member has to refuse the
// others' fields itself. A member that is no closed object lists none, and
// members a required constant tells apart (an operation's op) list none,
// because TypeScript checks a discriminated union member by member.
func (s *schemaTS) exclusiveKeys(list *jnode) ([][]string, error) {
	out := make([][]string, len(list.arr))
	own := make([][]string, len(list.arr))
	var objects []*jnode
	var at []int
	for i, m := range list.arr {
		r, err := s.resolve(m)
		if err != nil {
			return nil, err
		}
		props := r.get("properties")
		ap := r.get("additionalProperties")
		if props == nil || len(props.keys) == 0 || ap == nil || !isClosed(ap) {
			continue
		}
		own[i] = props.keys
		objects = append(objects, r)
		at = append(at, i)
	}
	if len(objects) < 2 || discriminated(objects) {
		return out, nil
	}
	for _, i := range at {
		for _, j := range at {
			if i == j {
				continue
			}
			for _, k := range own[j] {
				if !slices.Contains(own[i], k) && !slices.Contains(out[i], k) {
					out[i] = append(out[i], k)
				}
			}
		}
	}
	return out, nil
}

// discriminated reports whether one field is required in every member and a
// constant in each.
func discriminated(members []*jnode) bool {
	for _, k := range members[0].get("properties").keys {
		every := true
		for _, m := range members {
			p := m.get("properties").get(k)
			if p == nil || !p.has("const") || !slices.Contains(m.get("required").strings(), k) {
				every = false
				break
			}
		}
		if every {
			return true
		}
	}
	return false
}

// resolve is the node a $ref names, or n itself.
func (s *schemaTS) resolve(n *jnode) (*jnode, error) {
	r := n.str("$ref")
	if r == "" {
		return n, nil
	}
	def, ok := strings.CutPrefix(r, "#/$defs/")
	if target := s.root.get("$defs").get(def); ok && target != nil {
		return target, nil
	}
	return nil, fmt.Errorf("$ref %q names no definition", r)
}

// neverFields renders fields a type refuses: k?: never.
func neverFields(keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = tsKey(k) + "?: never;"
	}
	return out
}

// requiredOnly reports whether a oneOf lists only which properties are
// required: an exactly-one choice among an object's own properties.
func requiredOnly(list *jnode) bool {
	if list == nil || len(list.arr) == 0 {
		return false
	}
	for _, m := range list.arr {
		if !m.isObj || len(m.keys) != 1 || m.keys[0] != "required" {
			return false
		}
	}
	return true
}

// memberLabel names a oneOf or anyOf member in a location: its op, else its
// one required property, else its index.
func memberLabel(m *jnode, i int) string {
	if op := m.get("properties").get("op").str("const"); op != "" {
		return op
	}
	if req := m.get("required").strings(); len(req) == 1 && m.has("properties") {
		return req[0]
	}
	return strconv.Itoa(i)
}

func isObjectSchema(n *jnode) bool {
	return slices.Contains(n.types(), "object") || n.has("properties")
}

// isClosed reports whether additionalProperties allows nothing: false, or a
// schema nothing satisfies.
func isClosed(ap *jnode) bool {
	if b, ok := ap.val.(bool); ok {
		return !b
	}
	if not := ap.get("not"); not != nil && not.isObj && len(not.keys) == 0 && len(ap.keys) == 1 {
		return true
	}
	return false
}

// schemaDoc is a node's description, with the constraints the types cannot
// carry as tags.
func schemaDoc(n *jnode) string {
	var lines []string
	if d := n.str("description"); d != "" {
		lines = append(lines, d)
	}
	var tags []string
	if p := n.str("pattern"); p != "" {
		tags = append(tags, "@pattern "+p)
	}
	for _, kw := range []string{"minimum", "maximum", "minItems"} {
		if v := n.get(kw); v != nil {
			tags = append(tags, fmt.Sprintf("@%s %v", kw, v.val))
		}
	}
	if len(tags) > 0 {
		lines = append(lines, strings.Join(tags, "\n"))
	}
	return strings.Join(lines, "\n\n")
}

// patternType renders a string pattern as a template literal type where its
// variable part is a number (a plural form such as =0), and as string
// otherwise: a revision or a digest is a token a client copies from a read,
// which the read types as string.
func patternType(p string) string {
	body, ok := strings.CutPrefix(p, "^")
	if !ok {
		return "string"
	}
	if body, ok = strings.CutSuffix(body, "$"); !ok {
		return "string"
	}
	for _, num := range []string{`[0-9]+(\.[0-9]+)?`, `[0-9]+`} {
		prefix, ok := strings.CutSuffix(body, num)
		if !ok {
			continue
		}
		if strings.Trim(prefix, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789=:_-") == "" {
			return "`" + prefix + "${number}`"
		}
	}
	return "string"
}

// flatStructShape is the schema jsonschema-go generates for a struct of
// scalar fields, as a node: each field typed, the ones without omitempty
// required, nothing else allowed.
func flatStructShape(t reflect.Type) *jnode {
	n := &jnode{isObj: true, obj: map[string]*jnode{}}
	props := &jnode{isObj: true, obj: map[string]*jnode{}}
	var required []*jnode
	for f := range t.Fields() {
		name, opts := parseJSONTag(f)
		var typ string
		switch f.Type.Kind() {
		case reflect.Int, reflect.Int64, reflect.Int32:
			typ = "integer"
		case reflect.String:
			typ = "string"
		case reflect.Bool:
			typ = "boolean"
		default:
			panic(fmt.Sprintf("flatStructShape: %s.%s is not a scalar", t, f.Name))
		}
		props.set(name, &jnode{isObj: true, obj: map[string]*jnode{"type": {val: typ}}, keys: []string{"type"}})
		if !hasOption(opts, "omitempty") && !hasOption(opts, "omitzero") {
			required = append(required, &jnode{val: name})
		}
	}
	n.set("type", &jnode{val: "object"})
	n.set("properties", props)
	n.set("required", &jnode{isArr: true, arr: required})
	n.set("additionalProperties", &jnode{val: false})
	return n
}

// shapeKey is a node's shape: its canonical JSON without descriptions, with
// keys sorted and a closed object's additionalProperties spelled one way.
func shapeKey(n *jnode) string {
	var b strings.Builder
	writeShape(&b, n)
	return b.String()
}

func writeShape(b *strings.Builder, n *jnode) {
	switch {
	case n == nil:
		b.WriteString("null")
	case n.isObj:
		keys := slices.Clone(n.keys)
		slices.Sort(keys)
		b.WriteString("{")
		first := true
		for _, k := range keys {
			if k == "description" || k == "title" || k == "$schema" {
				continue
			}
			if !first {
				b.WriteString(",")
			}
			first = false
			b.WriteString(strconv.Quote(k) + ":")
			if k == "additionalProperties" && isClosed(n.obj[k]) {
				b.WriteString("false")
				continue
			}
			writeShape(b, n.obj[k])
		}
		b.WriteString("}")
	case n.isArr:
		b.WriteString("[")
		for i, v := range n.arr {
			if i > 0 {
				b.WriteString(",")
			}
			writeShape(b, v)
		}
		b.WriteString("]")
	default:
		v, _ := json.Marshal(n.val)
		b.Write(v)
	}
}

// literal renders a JSON scalar as a TypeScript literal type.
func literal(n *jnode) (string, error) {
	switch v := n.val.(type) {
	case string:
		return jsonString(v), nil
	case json.Number:
		return v.String(), nil
	case bool:
		return strconv.FormatBool(v), nil
	}
	return "", fmt.Errorf("literal %v is not a scalar", n.val)
}

// wrapUnion joins union members, one per line when they do not fit on one.
func wrapUnion(members []string, indent string) string {
	if len(members) == 1 {
		return members[0]
	}
	one := strings.Join(members, " | ")
	if len(one) <= 72 && !strings.Contains(one, "\n") {
		return one
	}
	return "\n" + indent + "  | " + strings.Join(members, "\n"+indent+"  | ")
}

// arrayOf renders an array of elem, parenthesizing a union or intersection.
func arrayOf(elem string) string {
	if strings.ContainsAny(elem, "|&\n") {
		return "(" + elem + ")[]"
	}
	return elem + "[]"
}

func join(loc, key string) string {
	if loc == "" {
		return key
	}
	return loc + "." + key
}

func pad(indent int) string { return strings.Repeat("  ", indent) }

// trimAll trims the indentation of single-line fields.
func trimAll(fields []string) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = strings.TrimSpace(f)
	}
	return out
}

// pascal turns an op such as set_content into SetContent.
func pascal(s string) string {
	var b strings.Builder
	for part := range strings.SplitSeq(s, "_") {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}

// jsonString quotes s as a JSON string, which is a TypeScript string literal.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// oneLine joins a doc's lines.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// jsDoc renders a JSDoc block at indent: on one line when the doc is one
// short paragraph, else with each paragraph and list item reflowed to the
// width. Empty renders nothing.
func jsDoc(doc, indent string) string {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return ""
	}
	doc = strings.ReplaceAll(doc, "*/", "*\\/")
	width := 77 - len(indent)
	units := docUnits(doc)
	if len(units) == 1 && units[0] != "" && !strings.Contains(units[0], "\n") && utf8.RuneCountInString(units[0])+4 <= width {
		return indent + "/** " + units[0] + " */\n"
	}
	var b strings.Builder
	b.WriteString(indent + "/**\n")
	for _, u := range units {
		for _, l := range wrapUnit(u, width) {
			if l == "" {
				b.WriteString(indent + " *\n")
			} else {
				b.WriteString(indent + " * " + l + "\n")
			}
		}
	}
	b.WriteString(indent + " */\n")
	return b.String()
}

// docUnits splits a doc into what wraps on its own: each paragraph, joined
// into one line, and each list item, with an empty unit between paragraphs.
// A paragraph with an indented line is preformatted and kept as it is.
func docUnits(doc string) []string {
	var units []string
	for i, para := range strings.Split(doc, "\n\n") {
		if i > 0 {
			units = append(units, "")
		}
		lines := strings.Split(para, "\n")
		if slices.ContainsFunc(lines, func(l string) bool {
			return strings.HasPrefix(l, "\t") || strings.HasPrefix(l, "    ")
		}) {
			units = append(units, para)
			continue
		}
		cur := ""
		for _, l := range lines {
			l = strings.TrimSpace(l)
			switch {
			case strings.HasPrefix(l, "- "):
				if cur != "" {
					units = append(units, cur)
				}
				cur = l
			case cur == "":
				cur = l
			default:
				cur += " " + l
			}
		}
		if cur != "" {
			units = append(units, cur)
		}
	}
	return units
}

// wrapUnit wraps one unit at width, keeping a list item's continuation
// indented under its text. A preformatted unit keeps its lines.
func wrapUnit(unit string, width int) []string {
	if strings.Contains(unit, "\n") {
		return strings.Split(unit, "\n")
	}
	if utf8.RuneCountInString(unit) <= width {
		return []string{unit}
	}
	cont := ""
	if strings.HasPrefix(unit, "- ") {
		cont = "  "
	}
	var out []string
	cur := ""
	for w := range strings.FieldsSeq(unit) {
		switch {
		case cur == "":
			cur = w
		case utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(w) > width:
			out = append(out, cur)
			cur = cont + w
		default:
			cur += " " + w
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
