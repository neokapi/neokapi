package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
)

// stubTranslator resolves a fixed (scope, source) → target map. Everything
// else returns the source unchanged. Lets tests assert exact scope paths
// without wiring up a full gotext catalog.
type stubTranslator struct {
	entries map[string]string // key = scope + "\x00" + source
}

func newStub(entries ...[3]string) *stubTranslator {
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e[0]+"\x00"+e[1]] = e[2]
	}
	return &stubTranslator{entries: m}
}

func (s *stubTranslator) T(scope Scope, source string) string {
	if v, ok := s.entries[string(scope)+"\x00"+source]; ok {
		return v
	}
	return source
}

func (s *stubTranslator) Locale() model.LocaleID { return "fr-FR" }

func TestLocalizeComponentSchema_TranslatesToolMetaAndProperties(t *testing.T) {
	tr := newStub(
		[3]string{"tools.translate.displayName", "AI Translate", "Traduction IA"},
		[3]string{"tools.translate.description", "Translate content", "Traduire le contenu"},
		[3]string{"tools.translate.title", "AI Translate Tool", "Outil de Traduction IA"},
		[3]string{"tools.translate.properties.target-lang.title", "Target language", "Langue cible"},
		[3]string{"tools.translate.properties.target-lang.description", "BCP-47 locale", "Locale BCP-47"},
		[3]string{"tools.translate.groups.advanced.label", "Advanced", "Avancé"},
	)

	s := &schema.ComponentSchema{
		Title:       "AI Translate Tool",
		Description: "Translate content",
		Type:        "object",
		ToolMeta: &schema.ToolMeta{
			ID:          "translate",
			DisplayName: "AI Translate",
			Description: "Translate content",
		},
		Groups: []schema.ParameterGroup{
			{ID: "advanced", Label: "Advanced"},
		},
		Properties: map[string]schema.PropertySchema{
			"target-lang": {
				Type:        "string",
				Title:       "Target language",
				Description: "BCP-47 locale",
			},
		},
	}

	out := LocalizeComponentSchema(s, tr)

	assert.Equal(t, "Traduction IA", out.ToolMeta.DisplayName)
	assert.Equal(t, "Traduire le contenu", out.ToolMeta.Description)
	assert.Equal(t, "Outil de Traduction IA", out.Title,
		"ComponentSchema.Title uses the /title leaf (independent of /displayName)")
	assert.Equal(t, "Traduire le contenu", out.Description)
	assert.Equal(t, "Avancé", out.Groups[0].Label)
	assert.Equal(t, "Langue cible", out.Properties["target-lang"].Title)
	assert.Equal(t, "Locale BCP-47", out.Properties["target-lang"].Description)
}

func TestLocalizeComponentSchema_DoesNotMutateInput(t *testing.T) {
	tr := newStub(
		[3]string{"tools.translate.displayName", "AI Translate", "Traduction IA"},
	)
	s := &schema.ComponentSchema{
		ToolMeta: &schema.ToolMeta{ID: "translate", DisplayName: "AI Translate"},
	}
	LocalizeComponentSchema(s, tr)
	assert.Equal(t, "AI Translate", s.ToolMeta.DisplayName,
		"input schema must not be mutated")
}

func TestLocalizeComponentSchema_PreservesNonTranslatableFields(t *testing.T) {
	tr := newStub(
		[3]string{"tools.foo.properties.count.title", "Count", "Nombre"},
	)
	five := 5.0
	s := &schema.ComponentSchema{
		ToolMeta: &schema.ToolMeta{ID: "foo"},
		Properties: map[string]schema.PropertySchema{
			"count": {
				Type:    "integer",
				Title:   "Count",
				Default: 3,
				Min:     &five,
				Max:     &five,
			},
		},
	}
	out := LocalizeComponentSchema(s, tr)

	p := out.Properties["count"]
	assert.Equal(t, "Nombre", p.Title)
	assert.Equal(t, "integer", p.Type, "Type must stay verbatim")
	assert.Equal(t, 3, p.Default, "Default must stay verbatim")
	assert.Equal(t, &five, p.Min, "Min must stay verbatim")
	assert.Equal(t, &five, p.Max, "Max must stay verbatim")
}

func TestLocalizeComponentSchema_TranslatesOptionLabelsAndEnumDescriptions(t *testing.T) {
	tr := newStub(
		[3]string{"tools.foo.properties.mode.options.fast.label", "Fast", "Rapide"},
		[3]string{"tools.foo.properties.mode.options.slow.label", "Slow", "Lent"},
		[3]string{"tools.foo.properties.mode.enumDescriptions.fast", "Prioritise throughput", "Priorité au débit"},
	)
	s := &schema.ComponentSchema{
		ToolMeta: &schema.ToolMeta{ID: "foo"},
		Properties: map[string]schema.PropertySchema{
			"mode": {
				Type: "string",
				Options: []schema.OptionItem{
					{Value: "fast", Label: "Fast"},
					{Value: "slow", Label: "Slow"},
				},
				EnumDescriptions: map[string]string{
					"fast": "Prioritise throughput",
				},
			},
		},
	}
	out := LocalizeComponentSchema(s, tr)

	assert.Equal(t, "Rapide", out.Properties["mode"].Options[0].Label)
	assert.Equal(t, "Lent", out.Properties["mode"].Options[1].Label)
	assert.Equal(t, "Priorité au débit",
		out.Properties["mode"].EnumDescriptions["fast"])
}

func TestLocalizeComponentSchema_RecursesIntoNestedPropertiesAndArrayItems(t *testing.T) {
	tr := newStub(
		[3]string{"tools.foo.properties.outer.properties.inner.title", "Inner", "Intérieur"},
		[3]string{"tools.foo.properties.list.items.title", "Item", "Élément"},
	)
	s := &schema.ComponentSchema{
		ToolMeta: &schema.ToolMeta{ID: "foo"},
		Properties: map[string]schema.PropertySchema{
			"outer": {
				Type: "object",
				Properties: map[string]schema.PropertySchema{
					"inner": {Type: "string", Title: "Inner"},
				},
			},
			"list": {
				Type:  "array",
				Items: &schema.PropertySchema{Type: "string", Title: "Item"},
			},
		},
	}
	out := LocalizeComponentSchema(s, tr)
	assert.Equal(t, "Intérieur", out.Properties["outer"].Properties["inner"].Title)
	assert.Equal(t, "Élément", out.Properties["list"].Items.Title)
}

func TestLocalizeComponentSchema_NilSchemaReturnsNil(t *testing.T) {
	assert.Nil(t, LocalizeComponentSchema(nil, NoopTranslator{}))
}
