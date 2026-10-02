package change

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// Decode reads a change set in any of its three input forms:
//
//   - one JSON object, the envelope with its ops;
//   - JSONL: the envelope fields on the first line and one operation per line
//     after it, or operations only, one per line, under the default envelope;
//   - a JSON array of operations under the default envelope.
//
// Decoding is strict. An unknown field, an unknown operation, a missing
// required field, a value of the wrong type, a runs payload carrying native
// data, and a self-contradictory operation are refused with an *Error of code
// invalid whose Pointer names the value at fault. The returned set has its
// defaults filled in: Schema is SchemaID, Mode is apply and Gate is enforce
// when the input left them out, and every edition key is canonical.
func Decode(r io.Reader) (Set, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Set{}, fmt.Errorf("change: read change set: %w", err)
	}
	var values []json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var raw json.RawMessage
		err := dec.Decode(&raw)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Set{}, invalidAt("", "not JSON: %v", err)
		}
		values = append(values, raw)
	}
	if len(values) == 0 {
		return Set{}, invalidAt("", "empty input; a change set is an object with ops, JSONL, or an array of operations")
	}

	var (
		envelope map[string]json.RawMessage
		opsRaw   []json.RawMessage
	)
	first := bytes.TrimSpace(values[0])
	switch {
	case len(first) > 0 && first[0] == '[':
		if len(values) > 1 {
			return Set{}, invalidAt("", "an array of operations is the whole input; found more after it")
		}
		if err := json.Unmarshal(first, &opsRaw); err != nil {
			return Set{}, invalidAt("", "not an array of operations: %v", err)
		}
	case len(first) > 0 && first[0] == '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(first, &obj); err != nil {
			return Set{}, invalidAt("", "%v", err)
		}
		if _, isOp := obj["op"]; isOp {
			opsRaw = values
			break
		}
		envelope = obj
		if len(values) > 1 {
			if _, ok := envelope["ops"]; ok {
				return Set{}, invalidAt("/ops", "a JSONL change set carries its operations on the lines after the envelope, not in ops")
			}
			opsRaw = values[1:]
		} else if rawOps, ok := envelope["ops"]; ok {
			if bytes.Equal(bytes.TrimSpace(rawOps), []byte("null")) {
				return Set{}, invalidAt("/ops", "must be an array of operations")
			}
			if err := json.Unmarshal(rawOps, &opsRaw); err != nil {
				return Set{}, invalidAt("/ops", "must be an array of operations")
			}
		}
	default:
		return Set{}, invalidAt("", "a change set is an object with ops, JSONL, or an array of operations")
	}

	var set Set
	if envelope != nil {
		if err := decodeEnvelope(envelope, &set); err != nil {
			return Set{}, err
		}
	}
	if len(opsRaw) == 0 {
		return Set{}, invalidAt("/ops", "a change set has at least one operation")
	}
	set.Ops = make([]Op, len(opsRaw))
	for i, raw := range opsRaw {
		op, err := decodeOp(raw, "/ops/"+strconv.Itoa(i))
		if err != nil {
			return Set{}, err
		}
		set.Ops[i] = op
	}
	if set.Schema == "" {
		set.Schema = SchemaID
	}
	if set.Mode == "" {
		set.Mode = ModeApply
	}
	if set.Gate == "" {
		set.Gate = GateEnforce
	}
	return set, nil
}

// decodeEnvelope decodes the envelope fields other than ops.
func decodeEnvelope(obj map[string]json.RawMessage, set *Set) *Error {
	allowed := []string{"schema", "mode", "gate", "require_basis", "note", "evidence", "ops"}
	for _, k := range sortedKeys(obj) {
		if !slices.Contains(allowed, k) {
			return invalidAt("/"+escapePointer(k), "unknown field %q; a change set takes %s", k, strings.Join(allowed, ", "))
		}
	}
	type envelope struct {
		Schema       string     `json:"schema,omitempty"`
		Mode         Mode       `json:"mode,omitempty"`
		Gate         Gate       `json:"gate,omitempty"`
		RequireBasis bool       `json:"require_basis,omitempty"`
		Note         string     `json:"note,omitempty"`
		Evidence     []Evidence `json:"evidence,omitempty"`
	}
	fields := without(obj, "ops")
	var e envelope
	if err := decodeStruct(fields, reflect.ValueOf(&e).Elem(), ""); err != nil {
		return err
	}
	if e.Schema != "" && e.Schema != SchemaID {
		return invalidAt("/schema", "schema %q; this decoder reads %s", e.Schema, SchemaID)
	}
	switch e.Mode {
	case "", ModeApply, ModePreview:
	case "propose":
		return invalidAt("/mode", "propose is reserved and not yet accepted; send apply or preview")
	default:
		return invalidAt("/mode", "mode %q; one of apply, preview", e.Mode)
	}
	switch e.Gate {
	case "", GateEnforce, GateReport:
	default:
		return invalidAt("/gate", "gate %q; one of enforce, report", e.Gate)
	}
	*set = Set{Schema: e.Schema, Mode: e.Mode, Gate: e.Gate, RequireBasis: e.RequireBasis, Note: e.Note, Evidence: e.Evidence}
	return nil
}

// UnmarshalJSON decodes one operation strictly, as Decode decodes each
// operation of a change set. An error is an *Error whose Pointer is relative
// to the operation.
func (o *Op) UnmarshalJSON(b []byte) error {
	op, err := decodeOp(b, "")
	if err != nil {
		return err
	}
	*o = op
	return nil
}

// decodeOp decodes one operation at ptr.
func decodeOp(raw json.RawMessage, ptr string) (Op, *Error) {
	obj, err := objectAt(raw, ptr)
	if err != nil {
		return Op{}, err
	}
	kindRaw, ok := obj["op"]
	if !ok {
		return Op{}, invalidAt(ptr, "missing op; one of %s", kindList())
	}
	var kind Kind
	if json.Unmarshal(kindRaw, &kind) != nil {
		return Op{}, invalidAt(ptr+"/op", "must be a string naming the operation")
	}
	spec, ok := specOf(kind)
	if !ok || !spec.public {
		return Op{}, invalidAt(ptr+"/op", "unknown operation %q; one of %s", kind, kindList())
	}
	op := Op{Kind: kind, Body: spec.newBody()}
	bodyV := reflect.ValueOf(op.Body).Elem()

	common := map[string]bool{"op": true}
	switch spec.at {
	case atEdition, atBlock:
		common["at"] = true
	case atDoc:
		common["doc"] = true
	}
	if spec.ifMatch != ifMatchNone && spec.ifMatch != ifMatchBody {
		common["if_match"] = true
	}
	if spec.basis {
		common["basis"] = true
	}
	known := map[string]bool{}
	for k := range common {
		known[k] = true
	}
	for _, f := range bodyFields(bodyV.Type()) {
		known[f.name] = true
	}
	for _, k := range sortedKeys(obj) {
		if !known[k] {
			return Op{}, invalidAt(ptr+"/"+escapePointer(k), "unknown field %q; %s takes %s", k, kind, strings.Join(sortedKeys(known), ", "))
		}
	}

	switch spec.at {
	case atEdition, atBlock:
		raw, ok := obj["at"]
		if !ok {
			return Op{}, invalidAt(ptr, "missing required field %q", "at")
		}
		ref, err := decodeRef(raw, ptr+"/at")
		if err != nil {
			return Op{}, err
		}
		if spec.at == atBlock && !ref.Edition.IsZero() {
			return Op{}, invalidAt(ptr+"/at/edition", "%s addresses a whole block; leave out the edition", kind)
		}
		op.At = ref
	case atDoc:
		raw, ok := obj["doc"]
		if !ok {
			return Op{}, invalidAt(ptr, "missing required field %q", "doc")
		}
		var doc string
		if err := decodeValue(raw, reflect.ValueOf(&doc).Elem(), ptr+"/doc"); err != nil {
			return Op{}, err
		}
		if doc == "" {
			return Op{}, invalidAt(ptr+"/doc", "names no document")
		}
		op.At = Ref{Doc: doc}
	}

	if common["if_match"] {
		raw, ok := obj["if_match"]
		switch {
		case !ok && spec.ifMatch == ifMatchOptional:
		case !ok:
			return Op{}, invalidAt(ptr, `missing required field "if_match": send the revision you read, "absent" to create, or "*" to write whatever is there`)
		default:
			var v string
			if err := decodeValue(raw, reflect.ValueOf(&v).Elem(), ptr+"/if_match"); err != nil {
				return Op{}, err
			}
			if err := checkIfMatch(spec.ifMatch, v, ptr+"/if_match"); err != nil {
				return Op{}, err
			}
			op.IfMatch = v
		}
	}
	if raw, ok := obj["basis"]; ok && spec.basis {
		var v string
		if err := decodeValue(raw, reflect.ValueOf(&v).Elem(), ptr+"/basis"); err != nil {
			return Op{}, err
		}
		if !validRevision(v) {
			return Op{}, invalidAt(ptr+"/basis", "%q is not a revision (r: and 16 hex digits)", v)
		}
		op.Basis = v
	}

	if err := decodeStruct(without(obj, sortedKeys(common)...), bodyV, ptr); err != nil {
		return Op{}, err
	}
	if err := validateBody(op, ptr); err != nil {
		return Op{}, err
	}
	return op, nil
}

func checkIfMatch(shape ifMatchShape, v, ptr string) *Error {
	switch shape {
	case ifMatchRequired:
		if v == model.AbsentRevision || v == AnyRevision || validRevision(v) {
			return nil
		}
		return invalidAt(ptr, `%q is not a revision (r: and 16 hex digits), "absent" or "*"`, v)
	case ifMatchOptional:
		if validRevision(v) {
			return nil
		}
		return invalidAt(ptr, "%q is not a revision (r: and 16 hex digits)", v)
	case ifMatchDigest:
		if digestRe.MatchString(v) {
			return nil
		}
		return invalidAt(ptr, "%q is not a document digest (sha256: and 64 hex digits)", v)
	}
	return nil
}

// validateBody checks what a type cannot say: exactly-one-of choices, enums
// and ranges. Edition keys are parsed and kept canonical.
func validateBody(op Op, ptr string) *Error {
	switch b := op.Body.(type) {
	case *SetContent:
		return validateContent(b.Content, ptr)
	case *ReplaceText:
		if len(b.Edits) == 0 {
			return invalidAt(ptr+"/edits", "has no edits")
		}
		for i, e := range b.Edits {
			if err := validateSelection(e.Selection, ptr+"/edits/"+strconv.Itoa(i)); err != nil {
				return err
			}
		}
	case *SetAttribute:
		if b.Code == "" {
			return invalidAt(ptr+"/code", "names no code")
		}
		if b.Name == "" {
			return invalidAt(ptr+"/name", "names no attribute")
		}
	case *Mark:
		if err := validateSelection(b.Range, ptr+"/range"); err != nil {
			return err
		}
		if b.Type == "" {
			return invalidAt(ptr+"/type", "names no type")
		}
	case *Annotate:
		if b.Type == "" {
			return invalidAt(ptr+"/type", "names no annotation type")
		}
		if b.Anchor != nil {
			switch b.Anchor.Kind {
			case model.AnchorBlock, model.AnchorRun, model.AnchorRange, model.AnchorForm:
			default:
				return invalidAt(ptr+"/anchor/kind", "anchor kind %q; one of block, run, range, form", b.Anchor.Kind)
			}
		}
	case *Unannotate:
		if b.Type == "" {
			return invalidAt(ptr+"/type", "names no annotation type")
		}
		if b.ID == "" && !b.All {
			return invalidAt(ptr+"/id", "names no annotation")
		}
	case *InsertBlock:
		if b.After != "" && b.Before != "" {
			return invalidAt(ptr, "insert_block takes after or before, not both")
		}
		if len(b.Editions) == 0 {
			return invalidAt(ptr+"/editions", "has no editions")
		}
		canon := make(map[string]Content, len(b.Editions))
		for _, k := range sortedKeys(b.Editions) {
			at := ptr + "/editions/" + escapePointer(k)
			key, err := model.ParseEditionKey(k)
			if err != nil {
				return invalidAt(at, "%v", err)
			}
			if err := validateContent(b.Editions[k], at); err != nil {
				return err
			}
			canon[keyText(key)] = b.Editions[k]
		}
		b.Editions = canon
	case *DeleteBlock:
		if len(b.IfMatch) == 0 {
			return invalidAt(ptr+"/if_match", "names no edition revision")
		}
		canon := make(map[string]string, len(b.IfMatch))
		for _, k := range sortedKeys(b.IfMatch) {
			at := ptr + "/if_match/" + escapePointer(k)
			key, err := model.ParseEditionKey(k)
			if err != nil {
				return invalidAt(at, "%v", err)
			}
			if !validRevision(b.IfMatch[k]) {
				return invalidAt(at, "%q is not a revision (r: and 16 hex digits)", b.IfMatch[k])
			}
			canon[keyText(key)] = b.IfMatch[k]
		}
		b.IfMatch = canon
	case *Native:
		if b.Name == "" {
			return invalidAt(ptr+"/name", "names no operation")
		}
	case *Decide:
		switch b.Outcome {
		case OutcomeEstablish, OutcomeReject, OutcomeWithdraw, OutcomeAdvise:
		default:
			return invalidAt(ptr+"/outcome", "outcome %q; one of establish, reject, withdraw, advise", b.Outcome)
		}
		if b.Score != nil && (*b.Score < 0 || *b.Score > 100) {
			return invalidAt(ptr+"/score", "score %d is outside 0 to 100", *b.Score)
		}
	case *Term:
		if b.Action != "upsert" && b.Action != "delete" {
			return invalidAt(ptr+"/action", "action %q; one of upsert, delete", b.Action)
		}
		if b.Term == "" {
			return invalidAt(ptr+"/term", "names no term")
		}
	case *Memory:
		if b.Action != "add" && b.Action != "delete" {
			return invalidAt(ptr+"/action", "action %q; one of add, delete", b.Action)
		}
		for side, m := range map[string]*MemoryText{"from": &b.From, "to": &b.To} {
			key, err := model.ParseEditionKey(m.Edition)
			if err != nil || key.IsZero() {
				return invalidAt(ptr+"/"+side+"/edition", "names no edition: %q", m.Edition)
			}
			m.Edition = keyText(key)
		}
	case *Recipe:
		if b.Path == "" {
			return invalidAt(ptr+"/path", "names no recipe field")
		}
	}
	return nil
}

func validateContent(c Content, ptr string) *Error {
	if (c.Text == nil) == (c.Runs == nil) {
		return invalidAt(ptr, "takes exactly one of text and runs")
	}
	return nil
}

func validateSelection(s Selection, ptr string) *Error {
	forms := 0
	if s.Find != nil {
		forms++
		if *s.Find == "" {
			return invalidAt(ptr+"/find", "is empty")
		}
	}
	if s.Start != nil || s.End != nil {
		forms++
		if s.Start == nil || s.End == nil {
			return invalidAt(ptr, "start and end go together")
		}
		if *s.Start < 0 || *s.End < *s.Start {
			return invalidAt(ptr, "start %d and end %d do not make a range", *s.Start, *s.End)
		}
	}
	if s.Range != nil {
		forms++
	}
	if forms != 1 {
		return invalidAt(ptr, "names its text by exactly one of find, start and end, or range")
	}
	if s.Occurrence < 0 {
		return invalidAt(ptr+"/occurrence", "counts from 1")
	}
	if s.Occurrence != 0 && s.Find == nil {
		return invalidAt(ptr+"/occurrence", "goes with find")
	}
	return nil
}

// decodeRef decodes an "at" reference strictly.
func decodeRef(raw json.RawMessage, ptr string) (Ref, *Error) {
	var w refWire
	if err := decodeValue(raw, reflect.ValueOf(&w).Elem(), ptr); err != nil {
		return Ref{}, err
	}
	if w.Doc == "" {
		return Ref{}, invalidAt(ptr+"/doc", "names no document")
	}
	if w.Block == "" {
		return Ref{}, invalidAt(ptr+"/block", "names no block")
	}
	k, err := model.ParseEditionKey(w.Edition)
	if err != nil {
		return Ref{}, invalidAt(ptr+"/edition", "%v", err)
	}
	return Ref{Doc: w.Doc, Block: w.Block, Edition: k}, nil
}

var (
	rawMessageType = reflect.TypeFor[json.RawMessage]()
	runType        = reflect.TypeFor[model.Run]()
	unmarshalerTyp = reflect.TypeFor[json.Unmarshaler]()
)

// decodeValue decodes raw into v strictly, naming ptr in any error.
func decodeValue(raw json.RawMessage, v reflect.Value, ptr string) *Error {
	raw = bytes.TrimSpace(raw)
	t := v.Type()
	if t == rawMessageType {
		v.SetBytes(append([]byte(nil), raw...))
		return nil
	}
	if bytes.Equal(raw, []byte("null")) {
		return invalidAt(ptr, "null is not a value here; leave the field out")
	}
	if t == runType {
		if err := checkRunJSON(raw, ptr); err != nil {
			return err
		}
		if err := json.Unmarshal(raw, v.Addr().Interface()); err != nil {
			return invalidAt(ptr, "%v", err)
		}
		return nil
	}
	if t.Kind() == reflect.Pointer {
		elem := reflect.New(t.Elem())
		if err := decodeValue(raw, elem.Elem(), ptr); err != nil {
			return err
		}
		v.Set(elem)
		return nil
	}
	if reflect.PointerTo(t).Implements(unmarshalerTyp) {
		if err := json.Unmarshal(raw, v.Addr().Interface()); err != nil {
			return invalidAt(ptr, "%v", err)
		}
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		obj, err := objectAt(raw, ptr)
		if err != nil {
			return err
		}
		return decodeStruct(obj, v, ptr)
	case reflect.Slice:
		if len(raw) == 0 || raw[0] != '[' {
			return invalidAt(ptr, "must be an array")
		}
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return invalidAt(ptr, "%v", err)
		}
		s := reflect.MakeSlice(t, len(items), len(items))
		for i, item := range items {
			if err := decodeValue(item, s.Index(i), ptr+"/"+strconv.Itoa(i)); err != nil {
				return err
			}
		}
		v.Set(s)
		return nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return invalidAt(ptr, "cannot decode a map keyed by %s", t.Key())
		}
		obj, err := objectAt(raw, ptr)
		if err != nil {
			return err
		}
		m := reflect.MakeMapWithSize(t, len(obj))
		for _, k := range sortedKeys(obj) {
			elem := reflect.New(t.Elem()).Elem()
			if err := decodeValue(obj[k], elem, ptr+"/"+escapePointer(k)); err != nil {
				return err
			}
			m.SetMapIndex(reflect.ValueOf(k).Convert(t.Key()), elem)
		}
		v.Set(m)
		return nil
	case reflect.String:
		if raw[0] != '"' {
			return invalidAt(ptr, "must be a string")
		}
	case reflect.Bool:
		if string(raw) != "true" && string(raw) != "false" {
			return invalidAt(ptr, "must be true or false")
		}
	case reflect.Int, reflect.Int64, reflect.Int32:
		if raw[0] != '-' && (raw[0] < '0' || raw[0] > '9') {
			return invalidAt(ptr, "must be an integer")
		}
	case reflect.Float64, reflect.Float32:
		if raw[0] != '-' && (raw[0] < '0' || raw[0] > '9') {
			return invalidAt(ptr, "must be a number")
		}
	}
	if err := json.Unmarshal(raw, v.Addr().Interface()); err != nil {
		return invalidAt(ptr, "%v", err)
	}
	return nil
}

// decodeStruct decodes the fields of obj into the struct v strictly: an
// unknown field or a missing required one is an error.
func decodeStruct(obj map[string]json.RawMessage, v reflect.Value, ptr string) *Error {
	fields := bodyFields(v.Type())
	byName := make(map[string]wireField, len(fields))
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		byName[f.name] = f
		names = append(names, f.name)
	}
	for _, k := range sortedKeys(obj) {
		f, ok := byName[k]
		if !ok {
			return invalidAt(ptr+"/"+escapePointer(k), "unknown field %q; it takes %s", k, strings.Join(names, ", "))
		}
		if err := decodeValue(obj[k], v.FieldByIndex(f.index), ptr+"/"+escapePointer(k)); err != nil {
			return err
		}
	}
	for _, f := range fields {
		if _, ok := obj[f.name]; f.required && !ok {
			return invalidAt(ptr, "missing required field %q", f.name)
		}
	}
	return nil
}

func objectAt(raw json.RawMessage, ptr string) (map[string]json.RawMessage, *Error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil, invalidAt(ptr, "must be an object")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, invalidAt(ptr, "%v", err)
	}
	return obj, nil
}

// Run payloads. A code carries its type and attributes; its native bytes
// (data) stay with the format, which spells a code from its type, so data is
// empty when present at all. The fields each run kind may carry on the wire:
var runFields = map[string][]string{
	"ph":      {"id", "type", "subType", "data", "equiv", "disp", "attrs", "constraints"},
	"pcOpen":  {"id", "type", "subType", "data", "equiv", "disp", "attrs", "constraints"},
	"pcClose": {"id", "type", "subType", "data", "equiv"},
	"sub":     {"id", "ref", "equiv"},
	"plural":  {"pivot", "forms"},
	"select":  {"pivot", "cases"},
}

// checkRunJSON checks one run of a payload: exactly one kind, only the fields
// that kind carries, no data, and branches that are runs in turn.
func checkRunJSON(raw json.RawMessage, ptr string) *Error {
	obj, err := objectAt(raw, ptr)
	if err != nil {
		return err
	}
	kinds := 0
	for _, k := range sortedKeys(obj) {
		switch k {
		case "text", "ph", "pcOpen", "pcClose", "sub", "plural", "select":
			kinds++
		case "noTranslate":
			if _, ok := obj["text"]; !ok {
				return invalidAt(ptr+"/noTranslate", "goes with a text run")
			}
		default:
			return invalidAt(ptr+"/"+escapePointer(k), "unknown run field %q; a run is one of text, ph, pcOpen, pcClose, sub, plural, select", k)
		}
	}
	if kinds != 1 {
		return invalidAt(ptr, "a run has exactly one of text, ph, pcOpen, pcClose, sub, plural, select; this one has %d", kinds)
	}
	for kind, allowed := range runFields {
		inner, ok := obj[kind]
		if !ok {
			continue
		}
		at := ptr + "/" + kind
		body, err := objectAt(inner, at)
		if err != nil {
			return err
		}
		for _, k := range sortedKeys(body) {
			if k == "data" && kind != "plural" && kind != "select" && kind != "sub" {
				var data string
				if json.Unmarshal(body[k], &data) != nil || data != "" {
					return invalidAt(at+"/data", "a run in a change set carries no data: name the code by its id and the writer keeps its native form")
				}
				continue
			}
			if !slices.Contains(allowed, k) {
				return invalidAt(at+"/"+escapePointer(k), "unknown field %q; %s takes %s", k, kind, strings.Join(allowed, ", "))
			}
		}
		branchKey := map[string]string{"plural": "forms", "select": "cases"}[kind]
		if branchKey == "" {
			continue
		}
		branches, ok := body[branchKey]
		if !ok {
			return invalidAt(at, "missing required field %q", branchKey)
		}
		forms, err := objectAt(branches, at+"/"+branchKey)
		if err != nil {
			return err
		}
		for _, name := range sortedKeys(forms) {
			fat := at + "/" + branchKey + "/" + escapePointer(name)
			var items []json.RawMessage
			if err := json.Unmarshal(forms[name], &items); err != nil {
				return invalidAt(fat, "must be an array of runs")
			}
			for i, item := range items {
				if err := checkRunJSON(item, fat+"/"+strconv.Itoa(i)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// escapePointer escapes a JSON pointer token (RFC 6901).
func escapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// without returns obj with the given keys left out.
func without(obj map[string]json.RawMessage, drop ...string) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(obj))
	for k, v := range obj {
		if !slices.Contains(drop, k) {
			out[k] = v
		}
	}
	return out
}

func kindList() string {
	names := make([]string, 0, len(specs))
	for _, k := range Kinds() {
		names = append(names, string(k))
	}
	return strings.Join(names, ", ")
}
