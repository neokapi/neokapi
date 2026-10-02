package model_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVocabularyLoadDefaults(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	err := reg.LoadDefaults()
	require.NoError(t, err)

	// common-formatting types
	info := reg.Lookup("fmt:bold")
	require.NotNil(t, info)
	assert.Equal(t, "formatting", info.Category)
	assert.Equal(t, "Bold", info.Label)
	assert.Equal(t, "<b>", info.HTML.Open)
	assert.Equal(t, "</b>", info.HTML.Close)
	assert.Equal(t, "[B]", info.Display.Open)
	assert.Equal(t, "[/B]", info.Display.Close)
	assert.True(t, info.Constraints.Deletable)
	assert.True(t, info.Constraints.Cloneable)
	assert.True(t, info.Constraints.Reorderable)

	// rich-html extension types
	info = reg.Lookup("fmt:strikethrough")
	require.NotNil(t, info)
	assert.Equal(t, "formatting", info.Category)
	assert.Equal(t, "<s>", info.HTML.Open)

	// code-tokens extension types
	info = reg.Lookup("code:variable")
	require.NotNil(t, info)
	assert.Equal(t, "code", info.Category)
	assert.False(t, info.Constraints.Deletable)

	// media types from common-formatting
	info = reg.Lookup("media:image")
	require.NotNil(t, info)
	assert.Equal(t, "<img/>", info.HTML.Placeholder)

	// struct types
	info = reg.Lookup("struct:break")
	require.NotNil(t, info)
	assert.Equal(t, "\n", info.Equiv)
	assert.False(t, info.Constraints.Deletable)
}

func TestVocabularyLookupUnknown(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.LoadDefaults())

	assert.Nil(t, reg.Lookup("unknown:type"))
}

func TestVocabularyLookupOrFallback(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.LoadDefaults())

	// Known type returns type info
	info := reg.LookupOrFallback("fmt:bold")
	require.NotNil(t, info)
	assert.Equal(t, "Bold", info.Label)

	// Unknown type returns fallback
	info = reg.LookupOrFallback("custom:unknown")
	require.NotNil(t, info)
	assert.True(t, info.Constraints.Deletable)
}

func TestVocabularyCategories(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.LoadDefaults())

	cats := reg.Categories()
	assert.Contains(t, cats, "formatting")
	assert.Contains(t, cats, "linking")
	assert.Contains(t, cats, "media")
	assert.Contains(t, cats, "structure")
	assert.Contains(t, cats, "code")
}

func TestVocabularyTypesInCategory(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.LoadDefaults())

	formatting := reg.TypesInCategory("formatting")
	assert.Contains(t, formatting, "fmt:bold")
	assert.Contains(t, formatting, "fmt:italic")
	assert.Contains(t, formatting, "fmt:underline")
	assert.Contains(t, formatting, "fmt:code")
	assert.Contains(t, formatting, "fmt:strikethrough")

	code := reg.TypesInCategory("code")
	assert.Contains(t, code, "code:variable")
	assert.Contains(t, code, "code:placeholder")
}

func TestVocabularyIsEntityType(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.LoadDefaults())

	assert.True(t, reg.IsEntityType("entity:person"))
	assert.True(t, reg.IsEntityType("entity:organization"))
	assert.False(t, reg.IsEntityType("fmt:bold"))
	assert.False(t, reg.IsEntityType("code:variable"))
}

func TestVocabularyHTMLRendering(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.LoadDefaults())

	// Known paired type
	assert.Equal(t, "<b>", reg.HTMLOpen("fmt:bold"))
	assert.Equal(t, "</b>", reg.HTMLClose("fmt:bold"))

	// Known placeholder type
	assert.Equal(t, "<br/>", reg.HTMLPlaceholder("struct:break"))

	// Unknown type uses fallback
	assert.Equal(t, `<span data-type="custom:foo">`, reg.HTMLOpen("custom:foo"))
	assert.Equal(t, `</span>`, reg.HTMLClose("custom:foo"))
	assert.Equal(t, `<span data-type="custom:foo"/>`, reg.HTMLPlaceholder("custom:foo"))
}

func TestVocabularyAllTypes(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.LoadDefaults())

	types := reg.AllTypes()
	assert.Greater(t, len(types), 10, "expected at least 10 types, got %d", len(types))
	// Should be sorted
	for i := 1; i < len(types); i++ {
		assert.Less(t, types[i-1], types[i], "types not sorted: %s >= %s", types[i-1], types[i])
	}
}

func TestVocabularyLoadInvalid(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	err := reg.Load([]byte("not json"))
	require.Error(t, err)
}

func TestVocabularyChipLabels(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.LoadDefaults())

	info := reg.Lookup("fmt:bold")
	require.NotNil(t, info)
	assert.Equal(t, "B>", info.ChipLabel.Open)
	assert.Equal(t, "/B", info.ChipLabel.Close)

	info = reg.Lookup("struct:break")
	require.NotNil(t, info)
	assert.Equal(t, "br", info.ChipLabel.Placeholder)
}

func TestVocabularyColors(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.LoadDefaults())

	info := reg.Lookup("fmt:bold")
	require.NotNil(t, info)
	assert.Contains(t, info.Color.Bg, "rgba")
	assert.Contains(t, info.Color.Border, "rgba")
	assert.Contains(t, info.Color.Text, "rgb")
}

func TestVocabularyCategoryIndexReflectsOverrides(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.LoadDefaults())
	for _, category := range reg.Categories() {
		names := reg.TypesInCategory(category)
		assert.Equal(t, slices.Compact(slices.Clone(names)), names,
			"inherited types must appear once in %s", category)
	}

	require.NoError(t, reg.Load(vocabularyPack(t, "technical", "rich-html", map[string]any{
		"fmt:code": map[string]any{"category": "technical", "label": "Literal code"},
	})))
	assert.NotContains(t, reg.TypesInCategory("formatting"), "fmt:code")
	assert.Equal(t, []string{"fmt:code"}, reg.TypesInCategory("technical"))
	assert.Equal(t, "Literal code", reg.Lookup("fmt:code").Label)
}

func TestVocabularyRejectsInvalidPacksAtomically(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{name: "missing identity", data: `{"types":{}}`, want: "name must be non-empty"},
		{name: "null pack", data: `null`, want: "name must be non-empty"},
		{name: "missing parent", data: `{"name":"custom","extends":"absent"}`, want: "is not loaded"},
		{name: "self parent", data: `{"name":"custom","extends":"custom"}`, want: "is not loaded"},
		{name: "duplicate identity", data: `{"name":"common-formatting"}`, want: "already loaded"},
		{name: "unrelated override", data: `{"name":"custom","types":{"fmt:bold":{"category":"other"}}}`,
			want: "conflicts with unrelated vocabulary"},
		{name: "sibling override", data: `{
			"name":"custom", "extends":"common-formatting", "types":{"fmt:strikethrough":{"category":"other"}}
		}`,
			want: "conflicts with unrelated vocabulary"},
		{name: "null definition", data: `{"name":"custom","types":{"custom:span":null}}`, want: "must be an object"},
		{name: "empty type", data: `{"name":"custom","types":{"":{"category":"other"}}}`, want: "type name"},
		{name: "padded type", data: `{"name":"custom","types":{" custom:span ":{"category":"other"}}}`, want: "type name"},
		{name: "missing category", data: `{"name":"custom","types":{"custom:span":{}}}`, want: "requires a category"},
		{name: "misspelled field", data: `{"name":"custom","types":{"custom:span":{"category":"other","constraint":{}}}}`,
			want: "unknown field"},
		{name: "trailing value", data: `{"name":"custom"} {}`, want: "one JSON document"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := model.NewVocabularyRegistry()
			require.NoError(t, reg.LoadDefaults())
			beforeTypes := reg.AllTypes()
			beforeCategories := reg.Categories()
			beforeBold := *reg.Lookup("fmt:bold")
			beforeFallback := *reg.Fallback()
			require.ErrorContains(t, reg.Load([]byte(tc.data)), tc.want)
			assert.Equal(t, beforeTypes, reg.AllTypes())
			assert.Equal(t, beforeCategories, reg.Categories())
			assert.Equal(t, beforeBold, *reg.Lookup("fmt:bold"))
			assert.Equal(t, beforeFallback, *reg.Fallback())
		})
	}
}

func TestVocabularyInvalidPackDoesNotPublishPartialState(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.LoadDefaults())
	beforeFallback := *reg.Fallback()
	invalid := []byte(`{
		"name":"custom", "extends":"rich-html", "entity_prefix":"custom:",
		"fallback":{"label":"different"},
		"types":{"a:valid":{"category":"custom"}, "z:invalid":null}
	}`)
	require.Error(t, reg.Load(invalid))
	assert.Nil(t, reg.Lookup("a:valid"))
	assert.Equal(t, beforeFallback, *reg.Fallback())
	assert.True(t, reg.IsEntityType("entity:person"))
	assert.False(t, reg.IsEntityType("custom:person"))

	// A failed registration does not reserve its name or satisfy a dependent pack.
	require.ErrorContains(t, reg.Load(vocabularyPack(t, "dependent", "custom", map[string]any{})), "is not loaded")
	require.NoError(t, reg.Load(vocabularyPack(t, "custom", "rich-html", map[string]any{
		"custom:span": map[string]any{"category": "custom"},
	})))
	require.NoError(t, reg.Load(vocabularyPack(t, "dependent", "custom", map[string]any{})))
}

func TestVocabularyOverrideRemovesEmptyCategory(t *testing.T) {
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.Load(vocabularyPack(t, "base", "", map[string]any{
		"custom:span": map[string]any{"category": "old"},
	})))
	require.NoError(t, reg.Load(vocabularyPack(t, "child", "base", map[string]any{
		"custom:span": map[string]any{"category": "new"},
	})))
	assert.Equal(t, []string{"new"}, reg.Categories())
	assert.Empty(t, reg.TypesInCategory("old"))
}

func TestDefaultVocabularyRejectsRegistration(t *testing.T) {
	reg := model.DefaultVocabulary()
	data := vocabularyPack(t, "custom", "rich-html", map[string]any{
		"custom:span": map[string]any{"category": "custom"},
	})
	require.ErrorContains(t, reg.Load(data), "read-only")
	assert.Nil(t, reg.Lookup("custom:span"))

	custom := model.NewVocabularyRegistry()
	require.NoError(t, custom.LoadDefaults())
	require.NoError(t, custom.Load(data))
	assert.NotNil(t, custom.Lookup("custom:span"))
}

func vocabularyPack(t *testing.T, name, parent string, types map[string]any) []byte {
	t.Helper()
	pack := map[string]any{"name": name, "types": types}
	if parent != "" {
		pack["extends"] = parent
	}
	data, err := json.Marshal(pack)
	require.NoError(t, err)
	return data
}
