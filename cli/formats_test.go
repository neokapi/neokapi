package cli

import (
	"testing"

	"github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/registry"
	coreschema "github.com/neokapi/neokapi/core/schema"
	"github.com/stretchr/testify/assert"
)

func TestFilterFormatsEmpty(t *testing.T) {
	infos := []registry.FormatInfo{
		{Name: "html", MimeTypes: []string{"text/html"}, Extensions: []string{".html"}},
		{Name: "csv", MimeTypes: []string{"text/csv"}, Extensions: []string{".csv"}},
	}

	result := filterFormats(infos, "application/pdf", "")
	assert.Empty(t, result)
}

func TestFilterFormatsByMime(t *testing.T) {
	infos := []registry.FormatInfo{
		{Name: "html", MimeTypes: []string{"text/html"}, Extensions: []string{".html"}},
		{Name: "csv", MimeTypes: []string{"text/csv"}, Extensions: []string{".csv"}},
		{Name: "xml", MimeTypes: []string{"application/xml"}, Extensions: []string{".xml"}},
	}

	result := filterFormats(infos, "text/html", "")
	assert.Len(t, result, 1)
	assert.Equal(t, registry.FormatID("html"), result[0].Name)
}

func TestFilterFormatsByExt(t *testing.T) {
	infos := []registry.FormatInfo{
		{Name: "html", MimeTypes: []string{"text/html"}, Extensions: []string{".html", ".htm"}},
		{Name: "csv", Extensions: []string{".csv"}},
	}

	result := filterFormats(infos, "", ".htm")
	assert.Len(t, result, 1)
	assert.Equal(t, registry.FormatID("html"), result[0].Name)
}

func TestFilterFormatsCombined(t *testing.T) {
	infos := []registry.FormatInfo{
		{Name: "html", MimeTypes: []string{"text/html"}, Extensions: []string{".html"}},
		{Name: "okapi-html", MimeTypes: []string{"text/html"}, Extensions: []string{".html", ".htm"}},
		{Name: "csv", MimeTypes: []string{"text/csv"}, Extensions: []string{".csv"}},
	}

	result := filterFormats(infos, "text/html", ".htm")
	assert.Len(t, result, 1)
	assert.Equal(t, registry.FormatID("okapi-html"), result[0].Name)
}

func TestContainsLower(t *testing.T) {
	assert.True(t, containsLower([]string{"text/html", "TEXT/XML"}, "text/html"))
	assert.True(t, containsLower([]string{"TEXT/HTML"}, "text/html"))
	assert.False(t, containsLower([]string{"text/html"}, "application/json"))
	assert.False(t, containsLower(nil, "text/html"))
}

func TestToFormatInfoParam_Boolean(t *testing.T) {
	prop := schema.PropertySchema{
		PropertySchema: coreschema.PropertySchema{
			Type:        "boolean",
			Description: "Enable extraction",
			Default:     true,
		},
	}
	p := toFormatInfoParam("extractAll", prop)
	assert.Equal(t, "extractAll", p.Name)
	assert.Equal(t, "boolean", p.Type)
	assert.Equal(t, true, p.Default)
	assert.Equal(t, "Enable extraction", p.Description)
}

func TestToFormatInfoParam_ObjectWithOkapiFormat(t *testing.T) {
	prop := schema.PropertySchema{
		PropertySchema: coreschema.PropertySchema{
			Type:        "object",
			Description: "Inline code detection",
		},
		OkapiFormat: "inlineCodeFinder",
	}
	p := toFormatInfoParam("codeFinderRules", prop)
	assert.Equal(t, "codeFinderRules", p.Name)
	assert.Equal(t, "inlineCodeFinder", p.Type)
	assert.Equal(t, "Inline code detection", p.Description)
}
