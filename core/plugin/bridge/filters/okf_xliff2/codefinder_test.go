//go:build integration

package okf_xliff2

import (
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// codeFinderParams returns filter params that enable the code finder with
// patterns matching escaped HTML tags (matching the okf_xliff2@codefinder.fprm config).
func codeFinderParams() map[string]any {
	return map[string]any{
		"useCodeFinder": true,
		"codeFinderRules": map[string]any{
			"rules": []string{
				`</?[a-zA-Z][a-zA-Z0-9]*[^>]*>`,
			},
		},
	}
}

// okapi: XLIFF2CodeFinderRoundTripTest#testCodeFinderCreatesInlineCodes
func TestCodeFinder_CreatesInlineCodes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "okf_xliff2/codefinder-subfilter-test.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, codeFinderParams())

	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "code finder should extract translatable blocks")

	// With code finder enabled and rules matching HTML-like patterns,
	// the escaped HTML tags in the source text should be recognized as inline codes.
	var foundInlineCodes bool
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag == nil {
			continue
		}
		for _, s := range frag.Spans {
			if s.SpanType == model.SpanPlaceholder || s.SpanType == model.SpanOpening || s.SpanType == model.SpanClosing {
				foundInlineCodes = true
				break
			}
		}
		if foundInlineCodes {
			break
		}
	}
	assert.True(t, foundInlineCodes, "code finder should create inline codes from matched patterns")
}

// okapi: XLIFF2CodeFinderRoundTripTest#testCodeFinderWithEscapedHtmlTags
func TestCodeFinder_WithEscapedHtmlTags(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>&lt;p&gt;Some text&lt;/p&gt;</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, codeFinderParams())

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Some text", "should contain the text content")
}

// okapi: XLIFF2CodeFinderRoundTripTest#testFullMergePreservesEscapedText
func TestCodeFinder_FullMergePreservesEscapedText(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "okf_xliff2/codefinder-subfilter-test.xlf")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, codeFinderParams())
	require.NotEmpty(t, result.Output)

	output := string(result.Output)
	assert.Contains(t, output, "Welcome", "roundtrip output should preserve text content")
	assert.Contains(t, output, "messages", "roundtrip output should preserve text content")
}

// okapi: XLIFF2CodeFinderRoundTripTest#testFullRoundTripPreservesEscapedText
func TestCodeFinder_FullRoundTripPreservesEscapedText(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "okf_xliff2/codefinder-subfilter-test.xlf")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, codeFinderParams())
}

// okapi: XLIFF2CodeFinderRoundTripTest#testSubfilterCodeFinderOnly
func TestCodeFinder_SubfilterCodeFinderOnly(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "okf_xliff2/codefinder/en-fr.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from codefinder test file")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World: How are you?")
	assert.Contains(t, texts, "xliff2 is cool!")

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}
