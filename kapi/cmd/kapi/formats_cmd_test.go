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

func TestPrintParameter_Boolean(t *testing.T) {
	prop := loader.PropertySchema{
		Type:        "boolean",
		Description: "Enable extraction",
		Default:     true,
	}
	// Just verify it doesn't panic
	printParameter("extractAll", prop)
}

func TestPrintParameter_String(t *testing.T) {
	prop := loader.PropertySchema{
		Type:        "string",
		Description: "Pattern to match",
		Default:     ".*",
	}
	printParameter("pattern", prop)
}

func TestPrintParameter_Integer(t *testing.T) {
	prop := loader.PropertySchema{
		Type:        "integer",
		Description: "Max depth",
		Default:     float64(10), // JSON numbers come as float64
	}
	printParameter("maxDepth", prop)
}

func TestPrintParameter_ObjectWithOkapiFormat(t *testing.T) {
	prop := loader.PropertySchema{
		Type:        "object",
		Description: "Inline code detection",
		OkapiFormat: "inlineCodeFinder",
	}
	printParameter("codeFinderRules", prop)
}

func TestPrintParameter_DeprecatedParam(t *testing.T) {
	prop := loader.PropertySchema{
		Type:       "boolean",
		Deprecated: true,
	}
	printParameter("oldOption", prop)
}

func TestPrintParameter_EmptyDefault(t *testing.T) {
	prop := loader.PropertySchema{
		Type:    "string",
		Default: "",
	}
	printParameter("emptyStr", prop)
}

func TestPrintParameter_NilDefault(t *testing.T) {
	prop := loader.PropertySchema{
		Type:    "string",
		Default: nil,
	}
	printParameter("noDefault", prop)
}
