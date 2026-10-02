// Package changeschema generates the JSON Schema of a kapi.change/v1 change
// set from the Go types of package change. It is its own package so that the
// applier, which every tool links, carries no schema generator.
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

// Schema returns the JSON Schema (draft 2020-12) of a kapi.change/v1 change
// set, indented. It is generated from the Go types of package change: each
// operation is a oneOf member whose op property is a const and which allows
// no other properties. The same schema is the input schema of every transport
// that takes a change set.
func Schema() []byte {
	b, err := schemaBytes()
	if err != nil {
		panic(fmt.Sprintf("changeschema: build the change-set schema: %v", err))
	}
	return slices.Clone(b)
}

// refWire is the JSON shape of a change.Ref, as change.Ref decodes it.
type refWire struct {
	Doc     string `json:"doc" jsonschema:"the document: a project-relative path, container!entry for an archive member, or a workspace document key"`
	Block   string `json:"block" jsonschema:"the block key a read reports"`
	Edition string `json:"edition,omitempty" jsonschema:"the edition: a language tag with optional ;tone= and ;channel=, for example fr or en;channel=short; omitted is the document's own edition"`
}

var schemaBytes = sync.OnceValues(func() ([]byte, error) {
	s, err := buildSchema()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(s, "", "  ")
})

const (
	patternRevision = `^r:[0-9a-f]{16}$`
	patternIfMatch  = `^(absent|\*|r:[0-9a-f]{16})$`
	patternDigest   = `^sha256:[0-9a-f]{64}$`
)

func str(desc string) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Description: desc}
}

func enum(desc string, values ...string) *jsonschema.Schema {
	s := str(desc)
	for _, v := range values {
		s.Enum = append(s.Enum, v)
	}
	return s
}

func object(props map[string]*jsonschema.Schema, order []string, required ...string) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:                 "object",
		Properties:           props,
		PropertyOrder:        order,
		Required:             required,
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

func constant(v string) *jsonschema.Schema {
	var c any = v
	return &jsonschema.Schema{Type: "string", Const: &c}
}

func empty(desc string) *jsonschema.Schema {
	s := constant("")
	s.Description = desc
	return s
}

// runSchemas are the $defs of the content model: a run, and a code's fields.
func runSchemas() map[string]*jsonschema.Schema {
	runRef := &jsonschema.Schema{Ref: "#/$defs/run"}
	runs := &jsonschema.Schema{Type: "array", Items: runRef}
	attrs := &jsonschema.Schema{Type: "object", AdditionalProperties: &jsonschema.Schema{Type: "string"}, Description: "the code's attributes, such as href"}
	constraints := object(map[string]*jsonschema.Schema{
		"deletable":   {Type: "boolean"},
		"cloneable":   {Type: "boolean"},
		"reorderable": {Type: "boolean"},
	}, []string{"deletable", "cloneable", "reorderable"})
	code := func(fields ...string) *jsonschema.Schema {
		all := map[string]*jsonschema.Schema{
			"id":          str("the code's id, as the placeholder <x id=\"…\"/> shows it"),
			"type":        str("the vocabulary type, such as fmt:bold or link:hyperlink"),
			"subType":     str("the format's own type"),
			"equiv":       str("the code's text equivalent"),
			"disp":        str("what an editor shows for the code"),
			"attrs":       attrs,
			"constraints": constraints,
			"ref":         str("the subblock the reference points at"),
			"data":        empty("always empty: a code's native form stays with the format, which spells a new code from its type"),
		}
		props := map[string]*jsonschema.Schema{}
		for _, f := range fields {
			props[f] = all[f]
		}
		return object(props, fields, "id")
	}
	kind := func(name string, body *jsonschema.Schema) *jsonschema.Schema {
		return object(map[string]*jsonschema.Schema{name: body}, []string{name}, name)
	}
	branches := func(key string, names *jsonschema.Schema) *jsonschema.Schema {
		return object(map[string]*jsonschema.Schema{
			"pivot": str("the variable the branches choose by"),
			key:     {Type: "object", AdditionalProperties: runs, PropertyNames: names},
		}, []string{"pivot", key}, "pivot", key)
	}
	run := &jsonschema.Schema{
		Description: "one run of content: text, or an inline code named by its id, type and attributes (never its native data), or a plural or select whose branches are runs",
		OneOf: []*jsonschema.Schema{
			object(map[string]*jsonschema.Schema{
				"text":        str("text"),
				"noTranslate": {Type: "boolean", Description: "the text is not for translation, such as a command in a code span"},
			}, []string{"text", "noTranslate"}, "text"),
			kind("ph", code("id", "type", "subType", "data", "equiv", "disp", "attrs", "constraints")),
			kind("pcOpen", code("id", "type", "subType", "data", "equiv", "disp", "attrs", "constraints")),
			kind("pcClose", code("id", "type", "subType", "data", "equiv")),
			kind("sub", code("id", "ref", "equiv")),
			kind("plural", branches("forms", pluralForm())),
			kind("select", branches("cases", nil)),
		},
	}
	return map[string]*jsonschema.Schema{"run": run}
}

// pluralForm is the schema of a plural form's name: a CLDR plural category,
// or an explicit value selector such as =0.
func pluralForm() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Description: "a plural form: zero, one, two, few, many, other, or =N",
		AnyOf: []*jsonschema.Schema{
			{Enum: []any{"zero", "one", "two", "few", "many", "other"}},
			{Pattern: `^=[0-9]+(\.[0-9]+)?$`},
		}}
}

// pathStep is the schema of one step of a run path.
func pathStep() *jsonschema.Schema {
	return &jsonschema.Schema{
		Description: "a run index, {\"plural\": form} or {\"select\": case}",
		OneOf: []*jsonschema.Schema{
			{Type: "integer", Minimum: new(0.0)},
			object(map[string]*jsonschema.Schema{"plural": pluralForm()}, []string{"plural"}, "plural"),
			object(map[string]*jsonschema.Schema{"select": str("a select case")}, []string{"select"}, "select"),
		},
	}
}

func buildSchema() (*jsonschema.Schema, error) {
	opts := &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[model.Run]():       {Ref: "#/$defs/run"},
		reflect.TypeFor[model.RunPath]():   {Ref: "#/$defs/path"},
		reflect.TypeFor[json.RawMessage](): {Description: "any JSON value"},
	}}
	ref, err := jsonschema.ForType(reflect.TypeFor[refWire](), opts)
	if err != nil {
		return nil, err
	}
	ref.Description = "an edition of a block in a document"
	block := ref.CloneSchemas()
	delete(block.Properties, "edition")
	block.PropertyOrder = []string{"doc", "block"}
	block.Description = "a block of a document"

	var members []*jsonschema.Schema
	for _, spec := range change.Operations() {
		body, err := jsonschema.ForType(reflect.TypeOf(spec.Body), opts)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", spec.Kind, err)
		}
		m := &jsonschema.Schema{
			Type:                 "object",
			Description:          spec.Summary,
			Properties:           map[string]*jsonschema.Schema{"op": constant(string(spec.Kind))},
			PropertyOrder:        []string{"op"},
			Required:             []string{"op"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		}
		add := func(name string, s *jsonschema.Schema, required bool) {
			m.Properties[name] = s
			m.PropertyOrder = append(m.PropertyOrder, name)
			if required {
				m.Required = append(m.Required, name)
			}
		}
		switch spec.Address {
		case change.AddressEdition:
			add("at", &jsonschema.Schema{Ref: "#/$defs/ref"}, true)
		case change.AddressBlock:
			add("at", &jsonschema.Schema{Ref: "#/$defs/block"}, true)
		case change.AddressDoc:
			add("doc", str("the document"), true)
		}
		switch spec.IfMatch {
		case change.IfMatchRequired:
			s := str(`the revision of the edition you read, "absent" for an edition that must not exist yet, or "*" for whatever is there`)
			s.Pattern = patternIfMatch
			add("if_match", s, true)
		case change.IfMatchOptional:
			s := str("the revision of the edition you read the anchor against")
			s.Pattern = patternRevision
			add("if_match", s, false)
		case change.IfMatchDigest:
			s := str("the digest of the document you read")
			s.Pattern = patternDigest
			add("if_match", s, true)
		}
		if spec.Basis {
			s := str("for a derived edition: the revision of the authoritative edition the content was made from")
			s.Pattern = patternRevision
			add("basis", s, false)
		}
		for _, name := range body.PropertyOrder {
			add(name, body.Properties[name], slices.Contains(body.Required, name))
		}
		refine(spec.Kind, m)
		members = append(members, m)
	}

	evidence, err := jsonschema.ForType(reflect.TypeFor[change.Evidence](), opts)
	if err != nil {
		return nil, err
	}
	root := object(map[string]*jsonschema.Schema{
		"schema":        constant(change.SchemaID),
		"mode":          enum("apply (the default), or preview: compute and check, write nothing", string(change.ModeApply), string(change.ModePreview)),
		"gate":          enum("enforce (the default): a failing finding the change introduces refuses it; report: the change lands with its findings", string(change.GateEnforce), string(change.GateReport)),
		"require_basis": {Type: "boolean", Description: "refuse a derived-edition write whose authoritative edition moved since it was read"},
		"note":          str("one line a person reads in history and review"),
		"evidence":      {Type: "array", Items: evidence, Description: "where the wording behind the change was seen"},
		"ops":           {Type: "array", MinItems: new(1), Items: &jsonschema.Schema{OneOf: members}, Description: "the operations, applied in order"},
	}, []string{"schema", "mode", "gate", "require_basis", "note", "evidence", "ops"}, "ops")
	root.Schema = "https://json-schema.org/draft/2020-12/schema"
	root.Title = change.SchemaID
	root.Description = "A change set: ordered operations that change content, review decisions and context assets, each guarded by the revision its sender read."
	root.Defs = runSchemas()
	root.Defs["ref"] = ref
	root.Defs["block"] = block
	root.Defs["path"] = &jsonschema.Schema{Type: "array", Items: pathStep(),
		Description: "a walk into a plural or select: a run index, then the form or case, repeated"}
	normalize(root)
	return root, nil
}

// refine adds what the Go types cannot say: choices of exactly one field, and
// enumerations.
func refine(kind change.Kind, m *jsonschema.Schema) {
	exactlyOneContent := []*jsonschema.Schema{{Required: []string{"text"}}, {Required: []string{"runs"}}}
	selection := func(s *jsonschema.Schema) {
		s.OneOf = []*jsonschema.Schema{{Required: []string{"find"}}, {Required: []string{"start", "end"}}, {Required: []string{"range"}}}
	}
	switch kind {
	case change.KindSetContent:
		m.OneOf = exactlyOneContent
	case change.KindReplaceText:
		edits := m.Properties["edits"]
		edits.MinItems = new(1)
		selection(edits.Items)
	case change.KindMark:
		selection(m.Properties["range"])
	case change.KindAnnotate:
		if a := m.Properties["anchor"]; a != nil {
			a.Properties["kind"].Enum = []any{"block", "run", "range", "form"}
		}
	case change.KindInsertBlock:
		m.Properties["editions"].AdditionalProperties.OneOf = exactlyOneContent
	case change.KindDeleteBlock:
		m.Properties["if_match"].AdditionalProperties.Pattern = patternRevision
	case change.KindDecide:
		m.Properties["outcome"].Enum = []any{string(change.OutcomeEstablish), string(change.OutcomeReject), string(change.OutcomeWithdraw), string(change.OutcomeAdvise)}
		m.Properties["score"].Minimum = new(0.0)
		m.Properties["score"].Maximum = new(100.0)
	case change.KindTerm:
		m.Properties["action"].Enum = []any{"upsert", "delete"}
	case change.KindMemory:
		m.Properties["action"].Enum = []any{"add", "delete"}
	}
}

// normalize makes the schema say what the decoder accepts: no value may be
// null, so a type list naming null is reduced to its other type.
func normalize(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if len(s.Types) > 0 {
		types := slices.DeleteFunc(slices.Clone(s.Types), func(t string) bool { return t == "null" })
		if len(types) == 1 {
			s.Type, s.Types = types[0], nil
		} else {
			s.Types = types
		}
	}
	for _, c := range s.Properties {
		normalize(c)
	}
	for _, c := range s.Defs {
		normalize(c)
	}
	for _, list := range [][]*jsonschema.Schema{s.OneOf, s.AnyOf, s.AllOf, s.PrefixItems} {
		for _, c := range list {
			normalize(c)
		}
	}
	normalize(s.Items)
	normalize(s.AdditionalProperties)
	normalize(s.Not)
}
