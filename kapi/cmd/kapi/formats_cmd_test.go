package main

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/loader"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/stretchr/testify/assert"
)

func TestFilterFormatsEmpty(t *testing.T) {
	infos := []registry.FormatInfo{
		{Name: "html", MimeTypes: []string{"text/html"}, Extensions: []string{".html"}},
		{Name: "csv", MimeTypes: []string{"text/csv"}, Extensions: []string{".csv"}},
	}

	// No match.
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
	assert.Equal(t, "html", result[0].Name)
}

func TestFilterFormatsByExt(t *testing.T) {
	infos := []registry.FormatInfo{
		{Name: "html", MimeTypes: []string{"text/html"}, Extensions: []string{".html", ".htm"}},
		{Name: "csv", Extensions: []string{".csv"}},
	}

	result := filterFormats(infos, "", ".htm")
	assert.Len(t, result, 1)
	assert.Equal(t, "html", result[0].Name)
}

func TestFilterFormatsCombined(t *testing.T) {
	infos := []registry.FormatInfo{
		{Name: "html", MimeTypes: []string{"text/html"}, Extensions: []string{".html"}},
		{Name: "okapi-html", MimeTypes: []string{"text/html"}, Extensions: []string{".html", ".htm"}},
		{Name: "csv", MimeTypes: []string{"text/csv"}, Extensions: []string{".csv"}},
	}

	// Both MIME and extension filter: only okapi-html has .htm + text/html.
	result := filterFormats(infos, "text/html", ".htm")
	assert.Len(t, result, 1)
	assert.Equal(t, "okapi-html", result[0].Name)
}

func TestContainsLower(t *testing.T) {
	assert.True(t, containsLower([]string{"text/html", "TEXT/XML"}, "text/html"))
	assert.True(t, containsLower([]string{"TEXT/HTML"}, "text/html"))
	assert.False(t, containsLower([]string{"text/html"}, "application/json"))
	assert.False(t, containsLower(nil, "text/html"))
}

func TestToFormatInfoParam_Boolean(t *testing.T) {
	prop := loader.PropertySchema{
		Type:        "boolean",
		Description: "Enable extraction",
		Default:     true,
	}
	p := toFormatInfoParam("extractAll", prop)
	assert.Equal(t, "extractAll", p.Name)
	assert.Equal(t, "boolean", p.Type)
	assert.Equal(t, true, p.Default)
	assert.Equal(t, "Enable extraction", p.Description)
}

func TestToFormatInfoParam_String(t *testing.T) {
	prop := loader.PropertySchema{
		Type:        "string",
		Description: "Pattern to match",
		Default:     ".*",
	}
	p := toFormatInfoParam("pattern", prop)
	assert.Equal(t, "pattern", p.Name)
	assert.Equal(t, "string", p.Type)
	assert.Equal(t, ".*", p.Default)
}

func TestToFormatInfoParam_Integer(t *testing.T) {
	prop := loader.PropertySchema{
		Type:        "integer",
		Description: "Max depth",
		Default:     float64(10), // JSON numbers come as float64
	}
	p := toFormatInfoParam("maxDepth", prop)
	assert.Equal(t, "maxDepth", p.Name)
	assert.Equal(t, "integer", p.Type)
	assert.Equal(t, float64(10), p.Default)
}

func TestToFormatInfoParam_ObjectWithOkapiFormat(t *testing.T) {
	prop := loader.PropertySchema{
		Type:        "object",
		Description: "Inline code detection",
		OkapiFormat: "inlineCodeFinder",
	}
	p := toFormatInfoParam("codeFinderRules", prop)
	assert.Equal(t, "codeFinderRules", p.Name)
	assert.Equal(t, "inlineCodeFinder", p.Type) // OkapiFormat overrides Type
	assert.Equal(t, "Inline code detection", p.Description)
}

func TestToFormatInfoParam_DeprecatedParam(t *testing.T) {
	prop := loader.PropertySchema{
		Type:       "boolean",
		Deprecated: true,
	}
	p := toFormatInfoParam("oldOption", prop)
	assert.Equal(t, "oldOption", p.Name)
	assert.Equal(t, "boolean", p.Type)
	assert.Nil(t, p.Default)
}

func TestToFormatInfoParam_EmptyDefault(t *testing.T) {
	prop := loader.PropertySchema{
		Type:    "string",
		Default: "",
	}
	p := toFormatInfoParam("emptyStr", prop)
	assert.Equal(t, "emptyStr", p.Name)
	assert.Equal(t, "", p.Default)
}

func TestToFormatInfoParam_NilDefault(t *testing.T) {
	prop := loader.PropertySchema{
		Type:    "string",
		Default: nil,
	}
	p := toFormatInfoParam("noDefault", prop)
	assert.Equal(t, "noDefault", p.Name)
	assert.Nil(t, p.Default)
}
