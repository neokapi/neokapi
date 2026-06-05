package model_test

import (
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
