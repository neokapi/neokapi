package gen

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	neokapiconfig "github.com/neokapi/neokapi/core/config"
	fschema "github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/i18n"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	coreschema "github.com/neokapi/neokapi/core/schema"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The generator writes a key and the localizer asks for one; the two derive
// their key from the same value, and there is exactly one function that does
// it. Before there were two, and four xliff2 version labels were written under
// `options..label` and asked for under `options.0.label` — pending in every
// locale, with nothing to say so.
func TestGeneratedKeysAnswerEveryScopeTheLocalizerAsks(t *testing.T) {
	doc := realDocument(t)
	raw := marshalToMap(t, doc)

	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)
	aitools.RegisterAll(toolReg)

	var missing []string
	var asked int
	for _, info := range toolReg.ListWithSchemas() {
		s := toolReg.Schema(info.Name)
		if s == nil {
			continue
		}
		tr := &recordingTranslator{doc: raw, domain: "tools", id: string(info.Name)}
		i18n.LocalizeComponentSchema(s, tr)
		asked += tr.asked
		missing = append(missing, tr.missing...)
	}

	formatReg := registry.NewFormatRegistry()
	schemaReg := fschema.NewSchemaRegistry()
	formats.RegisterAll(formatReg, formats.RegisterOptions{
		SchemaReg: schemaReg,
		ConfigReg: neokapiconfig.NewRegistry(),
	})
	for _, info := range formatReg.FormatInfos() {
		s, ok := schemaReg.GetSchema(string(info.Name))
		if !ok {
			continue
		}
		cs := &coreschema.ComponentSchema{
			ToolMeta:    &coreschema.ToolMeta{ID: string(info.Name)},
			Title:       s.Title,
			Description: s.Description,
			Groups:      s.Groups,
			Properties:  coreProperties(s.Properties),
		}
		tr := &recordingTranslator{doc: raw, domain: "formats", id: string(info.Name)}
		i18n.LocalizeComponentSchema(cs, tr)
		asked += tr.asked
		missing = append(missing, tr.missing...)
	}

	sort.Strings(missing)
	assert.Empty(t, missing, "the generated document carries no entry for these scopes")
	assert.Positive(t, asked)
}

// The four format ids whose schema the document had nothing for, named so a
// regression says which one came back.
func TestGeneratedDocumentCarriesFormatParameterText(t *testing.T) {
	raw := marshalToMap(t, realDocument(t))
	for _, id := range []string{"epub", "messageformat", "odf", "tsv", "csv", "xliff2"} {
		t.Run(id, func(t *testing.T) {
			entry, ok := raw["formats"].(map[string]any)[id].(map[string]any)
			require.True(t, ok, "no entry for format %s", id)
			props, _ := entry["properties"].(map[string]any)
			assert.NotEmpty(t, props, "format %s carries no parameter text", id)
		})
	}
}

func TestOptionKey(t *testing.T) {
	tests := []struct {
		name  string
		value any
		index int
		want  string
	}{
		{"a string names itself", "2.0", 3, "2.0"},
		{"an empty string falls back to the index", "", 0, "0"},
		{"an int", 7, 2, "7"},
		{"a whole float, the shape a JSON round trip leaves", float64(7), 2, "7"},
		{"a fractional float keeps its value", 1.5, 2, "1.5"},
		{"an int64", int64(11), 2, "11"},
		{"a named string, the shape a provider selector carries", providerID("anthropic"), 2, "anthropic"},
		{"an empty named string falls back to the index", providerID(""), 9, "9"},
		{"true", true, 4, "true"},
		{"false", false, 4, "false"},
		{"nil falls back to the index", nil, 5, "5"},
		{"a struct falls back to the index", struct{ A int }{1}, 6, "6"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, i18n.OptionKey(tc.value, tc.index))
		})
	}
}

// providerID stands in for aiprovider.ProviderID and every other named string
// an option value carries.
type providerID string

func realDocument(t *testing.T) *Document {
	t.Helper()
	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)
	aitools.RegisterAll(toolReg)
	formatReg := registry.NewFormatRegistry()
	schemaReg := fschema.NewSchemaRegistry()
	formats.RegisterAll(formatReg, formats.RegisterOptions{
		SchemaReg: schemaReg,
		ConfigReg: neokapiconfig.NewRegistry(),
	})
	return buildDocument(toolReg, formatReg, schemaReg)
}

func marshalToMap(t *testing.T, doc *Document) map[string]any {
	t.Helper()
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

func coreProperties(in map[string]fschema.PropertySchema) map[string]coreschema.PropertySchema {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]coreschema.PropertySchema, len(in))
	for name, p := range in {
		core := p.PropertySchema
		core.Properties = coreProperties(p.Properties)
		out[name] = core
	}
	return out
}

// recordingTranslator answers every lookup with the source text and records
// which scopes the generated document has no entry for. The scope the localizer
// derives starts at `tools.<id>` whatever the domain, so a format's lookups are
// redirected the same way the reference generator redirects them.
type recordingTranslator struct {
	doc     map[string]any
	domain  string
	id      string
	asked   int
	missing []string
}

func (r *recordingTranslator) Locale() model.LocaleID { return model.LocaleID("qps") }

func (r *recordingTranslator) T(scope i18n.Scope, source string) string {
	if source == "" {
		return source
	}
	r.asked++
	path := strings.Split(string(scope), ".")
	if len(path) > 0 && path[0] == "tools" {
		path[0] = r.domain
	}
	// A schema's own title is the entry's display name in the document.
	if len(path) == 3 && path[2] == "title" {
		path[2] = "displayName"
	}
	if !hasScopedKey(r.doc, path) {
		r.missing = append(r.missing, r.domain+" "+r.id+": "+strings.Join(path, "."))
	}
	return source
}

// hasScopedKey walks the document by key path, joining segments until one names
// a key: a property is keyed by its schema path, which may itself carry a dot.
func hasScopedKey(doc map[string]any, path []string) bool {
	var cur any = doc
	for i := 0; i < len(path); {
		m, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		matched := false
		for j := len(path); j > i; j-- {
			if v, ok := m[strings.Join(path[i:j], ".")]; ok {
				cur, i, matched = v, j, true
				break
			}
		}
		if !matched {
			return false
		}
	}
	s, ok := cur.(string)
	return ok && s != ""
}
