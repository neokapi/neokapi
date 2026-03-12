//go:build integration

package html

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullFile_AllExternalFiles reads all external HTML/HTM test files from
// the testdata directory and verifies extraction completes without errors.
//
// okapi: HtmlFullFileTest#testAllExternalFiles
func TestFullFile_AllExternalFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	tdDir := bridgetest.TestdataDir(t)
	htmlDir := filepath.Join(tdDir, "okf_html")

	// Collect all .html and .htm files.
	htmlFiles, err := filepath.Glob(filepath.Join(htmlDir, "*.html"))
	require.NoError(t, err)
	htmFiles, err := filepath.Glob(filepath.Join(htmlDir, "*.htm"))
	require.NoError(t, err)

	allFiles := append(htmlFiles, htmFiles...)
	if len(allFiles) == 0 {
		t.Skip("no HTML test files found in testdata")
	}

	for _, f := range allFiles {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			// The Java test simply iterates through all events without
			// asserting content -- just verifying no errors/panics.
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass, f, mimeType, nil)
			require.NotEmpty(t, parts, "reading %s should produce at least one part", name)
		})
	}
}

// TestFullFile_Nonwellformed reads non-wellformed HTML content and verifies
// the filter handles it gracefully without crashing.
//
// okapi: HtmlFullFileTest#testNonwellformed
func TestFullFile_Nonwellformed(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_html/nonwellformed.specialtest")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	// The main assertion is that parsing completes without error.
	require.NotEmpty(t, parts, "non-wellformed HTML should still produce parts")
}

// TestFullFile_EncodingShouldBeFound reads an HTML file with a
// meta charset="windows-1252" declaration and verifies the detected
// encoding is windows-1252.
//
// okapi: HtmlFullFileTest#testEncodingShouldBeFound
func TestFullFile_EncodingShouldBeFound(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_html/withEncoding.html")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part should be a Layer")

	// The HTML file declares charset=windows-1252 in a meta tag.
	// The Java test asserts: assertEquals("windows-1252", htmlFilter.getEncoding())
	assert.Equal(t, "windows-1252", layer.Encoding,
		"layer encoding should reflect the meta charset declaration")
}

// TestFullFile_EncodingShouldBeFound2 reads an XHTML file with a
// meta charset="UTF-8" declaration and verifies UTF-8 is detected.
//
// okapi: HtmlFullFileTest#testEncodingShouldBeFound2
func TestFullFile_EncodingShouldBeFound2(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_html/W3CHTMHLTest1.html")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part should be a Layer")

	assert.Equal(t, "UTF-8", layer.Encoding,
		"layer encoding should be UTF-8")
}

// TestFullFile_OkapiIntro reads the okapi_intro_test.html file and verifies
// that the first extracted text unit is "Okapi Framework" and the last is
// the non-breaking space character.
//
// okapi: HtmlFullFileTest#testOkapiIntro
func TestFullFile_OkapiIntro(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_html/okapi_intro_test.html")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable text from okapi_intro_test.html")

	// Java asserts: firstText == "Okapi Framework"
	firstText := blocks[0].SourceText()
	assert.Equal(t, "Okapi Framework", firstText,
		"first translatable text unit should be 'Okapi Framework'")

	// Java asserts: lastText == "\u00A0" (non-breaking space from &nbsp;)
	lastText := blocks[len(blocks)-1].SourceText()
	assert.Equal(t, "\u00A0", lastText,
		"last translatable text unit should be a non-breaking space")
}

// TestFullFile_SkippedScriptAndStyleElements reads an HTML file with
// script and style elements and verifies their content is NOT extracted
// as translatable text. The first translatable text should be "First Text".
//
// okapi: HtmlFullFileTest#testSkippedScriptandStyleElements
func TestFullFile_SkippedScriptAndStyleElements(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_html/testStyleScriptStylesheet.html")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract at least one translatable block")

	// Java asserts: firstText == "First Text"
	firstText := blocks[0].SourceText()
	assert.Equal(t, "First Text", firstText,
		"first translatable text should be 'First Text' (script/style content should be skipped)")

	// Verify no script or style content leaked into translatable blocks.
	texts := bridgetest.BlockTexts(blocks)
	for _, text := range texts {
		assert.NotContains(t, text, "h1 {color:red}",
			"style content should not appear in translatable blocks")
	}
}

// TestFullFile_OpenTwiceWithString reads the same HTML string content twice
// to verify the filter can be re-opened and produces consistent results.
//
// okapi: HtmlFullFileTest#testOpenTwiceWithString
func TestFullFile_OpenTwiceWithString(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := "<b>bolded html</b>"

	// First read.
	parts1 := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)
	blocks1 := bridgetest.FilterBlocks(parts1)

	// Second read -- same content.
	parts2 := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)
	blocks2 := bridgetest.FilterBlocks(parts2)

	require.NotEmpty(t, blocks1, "first read should produce blocks")
	require.Equal(t, len(blocks1), len(blocks2),
		"reading the same content twice should produce the same number of blocks")

	texts1 := bridgetest.BlockTexts(blocks1)
	texts2 := bridgetest.BlockTexts(blocks2)
	assert.Equal(t, texts1, texts2,
		"reading the same content twice should produce the same block texts")
}

// TestFullFile_OpenTwiceWithURI reads the same testdata file twice to verify
// file-based re-reading produces consistent results.
//
// okapi: HtmlFullFileTest#testOpenTwiceWithURI
func TestFullFile_OpenTwiceWithURI(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_html/okapi_intro_test.html")

	// First read.
	parts1 := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks1 := bridgetest.FilterBlocks(parts1)

	// Second read -- same file.
	parts2 := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks2 := bridgetest.FilterBlocks(parts2)

	require.NotEmpty(t, blocks1, "first read should produce blocks")
	require.Equal(t, len(blocks1), len(blocks2),
		"reading the same file twice should produce the same number of blocks")

	texts1 := bridgetest.BlockTexts(blocks1)
	texts2 := bridgetest.BlockTexts(blocks2)
	assert.Equal(t, texts1, texts2,
		"reading the same file twice should produce the same block texts")
}

// TestFullFile_OpenTwiceWithStream verifies that the filter can process the
// same content via byte-based reads. In Java this test expects a
// NullPointerException on the second open (because the InputStream is
// exhausted), but in Go the bridge reads content into bytes first, so
// re-reading the same content works. We verify both reads succeed and
// produce identical results.
//
// okapi: HtmlFullFileTest#testOpenTwiceWithStream
func TestFullFile_OpenTwiceWithStream(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_html/okapi_intro_test.html")

	// First read.
	parts1 := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks1 := bridgetest.FilterBlocks(parts1)

	// Second read -- the Go bridge reads content eagerly, so unlike the
	// Java InputStream-based test, this should work fine.
	parts2 := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks2 := bridgetest.FilterBlocks(parts2)

	require.NotEmpty(t, blocks1, "first read should produce blocks")
	require.Equal(t, len(blocks1), len(blocks2),
		"reading the same file twice should produce the same number of blocks")

	texts1 := bridgetest.BlockTexts(blocks1)
	texts2 := bridgetest.BlockTexts(blocks2)
	assert.Equal(t, texts1, texts2,
		"reading the same file twice should produce the same block texts")
}
