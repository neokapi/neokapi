package main

import (
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// renderSchema renders a schema whose root is named Root, with extra names by
// location, and returns the declarations.
func renderSchema(t *testing.T, schema string, names map[string]schemaName) (string, error) {
	t.Helper()
	root, err := decodeOrdered([]byte(schema))
	require.NoError(t, err)
	all := map[string]schemaName{"": {name: "Root"}}
	maps.Copy(all, names)
	s := newSchemaRenderer(root, &goDocs{types: map[string]string{}, fields: map[string]string{}, consts: map[string][]goConst{}},
		all, []emitType{{"RunPos", reflect.TypeFor[model.RunPos](), ""}})
	return s.emit()
}

func TestSchemaRendering(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		names  map[string]schemaName
		want   []string
	}{
		{
			name: "an exactly-one choice requires one field and refuses the others",
			schema: `{"type":"object","properties":{"text":{"type":"string"},"runs":{"type":"array","items":{"type":"string"}},"path":{"type":"string"}},
				"additionalProperties":false,"oneOf":[{"required":["text"]},{"required":["runs"]}]}`,
			want: []string{
				"export type Root = {",
				"  text?: string;",
				"  path?: string;",
				"} & (\n  | { text: string; runs?: never }\n  | { runs: string[]; text?: never }\n);",
			},
		},
		{
			name: "const, enum and required fields",
			schema: `{"type":"object","properties":{"op":{"type":"string","const":"mark"},"kind":{"type":"string","enum":["a","b"]},"n":{"type":"integer"}},
				"required":["op","kind"],"additionalProperties":{"not":{}}}`,
			want: []string{"export interface Root {", `  op: "mark";`, `  kind: "a" | "b";`, "  n?: number;"},
		},
		{
			name: "a map keyed by a plural form",
			schema: `{"type":"object","properties":{"forms":{"type":"object","additionalProperties":{"type":"string"},
				"propertyNames":{"type":"string","anyOf":[{"enum":["one","other"]},{"pattern":"^=[0-9]+(\\.[0-9]+)?$"}]}}},"additionalProperties":false}`,
			want: []string{"  forms?: Partial<Record<\"one\" | \"other\" | `=${number}`, string>>;"},
		},
		{
			name:   "a revision pattern stays a string and is documented",
			schema: `{"type":"object","properties":{"rev":{"type":"string","pattern":"^r:[0-9a-f]{16}$"}},"additionalProperties":false}`,
			want:   []string{"  /** @pattern ^r:[0-9a-f]{16}$ */\n  rev?: string;"},
		},
		{
			name: "a named location becomes its own declaration, and a node of its shape reuses it",
			schema: `{"type":"object","properties":{"a":{"type":"object","properties":{"x":{"type":"string"}},"additionalProperties":false},
				"b":{"type":"object","properties":{"x":{"type":"string","description":"same shape"}},"additionalProperties":false}},"additionalProperties":false}`,
			names: map[string]schemaName{"a": {name: "Named"}},
			want:  []string{"  a?: Named;", "  b?: Named;", "export interface Named {\n  x?: string;\n}"},
		},
		{
			name:   "a node shaped like a content-model type is imported",
			schema: `{"type":"object","properties":{"at":{"type":"object","properties":{"run":{"type":"integer"},"offset":{"type":"integer"}},"required":["run"],"additionalProperties":false}},"additionalProperties":false}`,
			want:   []string{"  at?: RunPos;"},
		},
		{
			name:   "a definition is referenced by its name",
			schema: `{"type":"object","properties":{"r":{"$ref":"#/$defs/ref"}},"additionalProperties":false,"$defs":{"ref":{"type":"string"}}}`,
			names:  map[string]schemaName{"$ref": {name: "Ref"}},
			want:   []string{"  r?: Ref;", "export type Ref = string;"},
		},
		{
			name:   "a value of any JSON",
			schema: `{"type":"object","properties":{"v":{"description":"any JSON value"}},"additionalProperties":false}`,
			want:   []string{"  /** any JSON value */\n  v?: unknown;"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderSchema(t, tt.schema, tt.names)
			require.NoError(t, err)
			for _, w := range tt.want {
				assert.Contains(t, got, w)
			}
		})
	}
}

func TestSchemaRenderingRefuses(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		names  map[string]schemaName
		want   string
	}{
		{
			name:   "a keyword it does not render",
			schema: `{"type":"object","properties":{"s":{"type":"string","maxLength":3}},"additionalProperties":false}`,
			want:   `keyword "maxLength" is not rendered`,
		},
		{
			name:   "an open object with properties",
			schema: `{"type":"object","properties":{"s":{"type":"string"}}}`,
			want:   "is not closed",
		},
		{
			name:   "a name for a location the schema lacks",
			schema: `{"type":"object","properties":{"s":{"type":"string"}},"additionalProperties":false}`,
			names:  map[string]schemaName{"gone": {name: "Gone"}},
			want:   `names "Gone" at "gone"`,
		},
		{
			name: "two names for one shape",
			schema: `{"type":"object","properties":{"a":{"type":"object","properties":{"x":{"type":"string"}},"additionalProperties":false},
				"b":{"type":"object","properties":{"x":{"type":"string"}},"additionalProperties":false}},"additionalProperties":false}`,
			names: map[string]schemaName{"a": {name: "A"}, "b": {name: "B"}},
			want:  "have one shape",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := renderSchema(t, tt.schema, tt.names)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestPatternType(t *testing.T) {
	tests := map[string]string{
		`^=[0-9]+(\.[0-9]+)?$`:    "`=${number}`",
		`^=[0-9]+$`:               "`=${number}`",
		`^r:[0-9a-f]{16}$`:        "string",
		`^sha256:[0-9a-f]{64}$`:   "string",
		`^(absent|\*|r:[0-9a-f])`: "string",
		``:                        "string",
	}
	for pattern, want := range tests {
		assert.Equal(t, want, patternType(pattern), pattern)
	}
}

// Every operation the change set may carry is rendered with its op, and the
// service's replies with the fields the Go structs marshal.
func TestEmitChange(t *testing.T) {
	out, err := emitChange()
	require.NoError(t, err)
	for _, k := range change.Kinds() {
		assert.Contains(t, out, `op: "`+string(k)+`";`)
	}
	assert.NotContains(t, out, `"`+string(change.KindProvenance)+`"`, "provenance is in-process only")
	for _, c := range change.Codes() {
		assert.Contains(t, out, "  "+string(c)+": ", "every code has its transport mappings")
	}
	assert.Contains(t, out, "export interface ResultRef {\n  doc: string;\n  block?: string;\n  edition?: string;\n}")
	assert.Contains(t, out, "  record: string | null;", "a nil pointer without omitempty is null")
	assert.Contains(t, out, "  at?: ResultRef;", "a pointer with omitempty is optional")
	assert.Contains(t, out, "  ref: ChangeRef;", "a read's reference is what an operation takes")
	assert.Contains(t, out, `import type { RunPos } from "./content.gen.ts";`)
}

// A type that marshals itself has no generic rendering.
func TestGoTSRefusesSelfMarshalingTypes(t *testing.T) {
	g := &goTS{names: map[reflect.Type]string{}, external: map[string]bool{}, imported: map[string]bool{}}
	_, err := g.tsOf(reflect.TypeFor[change.Ref]())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshals itself")

	ts, err := g.tsOf(reflect.TypeFor[[]model.EditionKey]())
	require.NoError(t, err)
	assert.Equal(t, "string[]", ts, "a text marshaler is a string")
}

func TestJSDoc(t *testing.T) {
	assert.Equal(t, "/** short */\n", jsDoc("short", ""))
	assert.Empty(t, jsDoc("  ", ""))

	got := jsDoc("Content is an edition's content in one of its two forms: Text, the\nplaceholder form a read shows, or Runs. Exactly\none is set.\n\n- `a`: first\n- `b`: second item whose text runs on well past the width of one line of a doc", "")
	assert.Equal(t, strings.Join([]string{
		"/**",
		" * Content is an edition's content in one of its two forms: Text, the",
		" * placeholder form a read shows, or Runs. Exactly one is set.",
		" *",
		" * - `a`: first",
		" * - `b`: second item whose text runs on well past the width of one line of a",
		" *   doc",
		" */",
		"",
	}, "\n"), got)
	assert.Contains(t, jsDoc("a */ b\nc", ""), `a *\/ b`)
}

func TestConstDoc(t *testing.T) {
	assert.Equal(t, "every operation landed.", constDoc(goConst{name: "SetApplied", doc: "SetApplied: every operation landed."}))
	assert.Equal(t, "applies the change set. It is the default.", constDoc(goConst{name: "ModeApply", doc: "ModeApply applies the change set.\nIt is the default."}))
	assert.Equal(t, "unrelated text", constDoc(goConst{name: "X", doc: "unrelated text"}))
}
