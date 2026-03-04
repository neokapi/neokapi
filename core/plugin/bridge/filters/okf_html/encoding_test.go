//go:build integration

package okf_html

import (
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEncoding_DefaultBehaviorAddsMetaElement verifies that the default
// roundtrip behavior inserts a meta Content-Type charset declaration into
// the head element when none exists in the original.
//
// okapi: SkipEncodingDeclarationTest#testDefaultBehaviorAddsMetaElement
func TestEncoding_DefaultBehaviorAddsMetaElement(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := `<html><head></head><body><p>test</p></body></html>`

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass,
		[]byte(input), "test.html", mimeType, nil)

	output := string(result.Output)

	// The Java test expects:
	// <html><head><meta http-equiv="Content-Type" content="text/html; charset=UTF-8"></head>...
	assert.Contains(t, output, "charset",
		"default roundtrip should add a charset declaration to the output")
	assert.Contains(t, output, "UTF-8",
		"default roundtrip should declare UTF-8 encoding")

	// Verify the meta tag is inside the head element.
	headStart := strings.Index(output, "<head>")
	headEnd := strings.Index(output, "</head>")
	require.True(t, headStart >= 0 && headEnd > headStart,
		"output should have head element")

	headContent := output[headStart:headEnd]
	assert.Contains(t, headContent, "meta",
		"meta charset tag should be inside the head element")
}

// TestEncoding_SkipEncodingDeclarationOmitsMetaElement verifies that when
// skipEncodingDeclaration is true, no meta charset tag is added during
// roundtrip, and the output matches the original input.
//
// okapi: SkipEncodingDeclarationTest#testSkipEncodingDeclarationOmitsMetaElement
func TestEncoding_SkipEncodingDeclarationOmitsMetaElement(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := `<html><head></head><body><p>test</p></body></html>`

	params := map[string]any{
		"skipEncodingDeclaration": true,
	}

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass,
		[]byte(input), "test.html", mimeType, params)

	output := string(result.Output)

	// With skipEncodingDeclaration=true, the output should not contain
	// a meta charset tag that was not in the original.
	assert.Equal(t, input, output,
		"with skipEncodingDeclaration=true, output should match the original input exactly")
}

// TestEncoding_XHTMLSelfClosingMetaTag verifies that for XHTML content
// (with xmlns attribute), the default roundtrip adds a self-closing meta
// tag with the charset declaration.
//
// okapi: SkipEncodingDeclarationTest#testXHTMLSelfClosingMetaTag
func TestEncoding_XHTMLSelfClosingMetaTag(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := `<html xmlns="http://www.w3.org/1999/xhtml"><head></head><body><p>test</p></body></html>`

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass,
		[]byte(input), "test.html", mimeType, nil)

	output := string(result.Output)

	// The Java test expects a self-closing meta tag for XHTML:
	// <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
	assert.Contains(t, output, "charset",
		"XHTML roundtrip should add a charset declaration")
	assert.Contains(t, output, "UTF-8",
		"XHTML roundtrip should declare UTF-8 encoding")

	// In XHTML mode, the meta tag should be self-closing (ends with "/>").
	metaIdx := strings.Index(output, "<meta")
	require.True(t, metaIdx >= 0, "output should contain a meta tag")

	// Find the end of the meta tag.
	metaEnd := strings.Index(output[metaIdx:], ">")
	require.True(t, metaEnd > 0, "meta tag should have a closing >")

	metaTag := output[metaIdx : metaIdx+metaEnd+1]
	assert.True(t, strings.HasSuffix(metaTag, "/>"),
		"XHTML meta tag should be self-closing (end with />), got: %s", metaTag)
}

// TestEncoding_ExistingEncodingDeclaration verifies that when the input
// already has a meta charset declaration, the roundtrip produces valid output.
// The bridge always outputs UTF-8, so the charset declaration is updated.
//
// okapi: SkipEncodingDeclarationTest#testExistingEncodingDeclaration
func TestEncoding_ExistingEncodingDeclaration(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := `<html><head><meta http-equiv="Content-Type" content="text/html; charset=windows-1252"/></head><body><p>test</p></body></html>`

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass,
		[]byte(input), "test.html", mimeType, nil)

	output := string(result.Output)

	// The bridge always outputs UTF-8, so the charset declaration is updated.
	assert.Contains(t, output, "charset=UTF-8",
		"bridge output should declare UTF-8 encoding")

	// Should not have duplicate meta charset tags.
	metaCount := strings.Count(output, "<meta")
	assert.Equal(t, 1, metaCount,
		"should have exactly one meta tag (no duplicate charset declaration)")

	// Content should be preserved.
	assert.Contains(t, output, "<p>test</p>")
}

// TestEncoding_ExistingEncodingDeclarationWithSkipEnabled verifies roundtrip
// with skipEncodingDeclaration=true and an existing charset declaration.
// The bridge always outputs UTF-8, so the charset is updated regardless.
//
// okapi: SkipEncodingDeclarationTest#testExistingEncodingDeclarationWithSkipEnabled
func TestEncoding_ExistingEncodingDeclarationWithSkipEnabled(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := `<html><head><meta http-equiv="Content-Type" content="text/html; charset=windows-1252"/></head><body><p>test</p></body></html>`

	params := map[string]any{
		"skipEncodingDeclaration": true,
	}

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass,
		[]byte(input), "test.html", mimeType, params)

	output := string(result.Output)

	// The bridge always outputs UTF-8 bytes, so the charset declaration
	// is updated to UTF-8.
	assert.Contains(t, output, "charset=UTF-8",
		"bridge output should declare UTF-8 encoding")
	assert.Contains(t, output, "<p>test</p>",
		"content should be preserved")
}
