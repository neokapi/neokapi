//go:build integration

package php

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.php.PHPContentFilter"
const mimeType = "text/x-php"

// readPHP parses a PHP content snippet with custom filter params and returns the parts.
func readPHP(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.phpcnt", mimeType, filterParams)
}

// readPHPDefault parses a PHP content snippet with default (nil) params.
func readPHPDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readPHP(t, snippet, nil)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// snippetRoundtrip roundtrips a PHP content snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.phpcnt", mimeType, filterParams)
	return string(result.Output)
}

// snippetRoundtripDefault roundtrips with default (nil) params.
func snippetRoundtripDefault(t *testing.T, snippet string) string {
	t.Helper()
	return snippetRoundtrip(t, snippet, nil)
}

// findBlockContaining finds a block whose source text contains the given substring.
func findBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

// countSpans counts the total number of inline-code runs in a block's
// first source segment.
func countSpans(b *model.Block) int {
	return bridgetest.CountInlineCodes(b.SourceRuns())
}

// codeData returns a slice of inline-code run Data strings (in run order)
// across PcOpen / PcClose / Ph runs. Plain text runs are skipped.
func codeData(runs []model.Run) []string {
	var out []string
	for _, r := range runs {
		switch {
		case r.PcOpen != nil:
			out = append(out, r.PcOpen.Data)
		case r.PcClose != nil:
			out = append(out, r.PcClose.Data)
		case r.Ph != nil:
			out = append(out, r.Ph.Data)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Tests translated from PHPContentFilterTest.java
// ---------------------------------------------------------------------------

// okapi: PHPContentFilterTest#testDefaultInfo
func TestSnippet_DefaultInfo(t *testing.T) {
	// Verify the filter is available and produces parts from minimal input.
	parts := readPHPDefault(t, "$a='text';")
	require.NotEmpty(t, parts, "filter should produce parts")
}

// okapi: PHPContentFilterTest#testEntityReferences
func TestSnippet_EntityReferences(t *testing.T) {
	snippet := "$a='&aacute;&#xC1;&#225;&#x00c1;';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// Entity references resolve: &aacute;=\u00e1 &#xC1;=\u00c1 &#225;=\u00e1 &#x00c1;=\u00c1
	assert.Equal(t, "\u00e1\u00c1\u00e1\u00c1", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testReferencesLooklike
func TestSnippet_ReferencesLooklike(t *testing.T) {
	snippet := "$a='& &; &#; &aacute';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// Look-alike references that aren't valid entities pass through literally.
	assert.Equal(t, "& &; &#; &aacute", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testConcatSQStrings
func TestSnippet_ConcatSQStrings(t *testing.T) {
	snippet := "$a='t1' \r. 't2';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	// Concatenation merges into one TU; the concat operator becomes an inline code.
	// SourceText() returns plain text with inline code markers stripped.
	assert.Equal(t, "t1t2", tu.SourceText())

	codes := codeData(tu.SourceRuns())
	require.Equal(t, 1, len(codes), "should have 1 inline-code run for concat operator")
	assert.Equal(t, "' \r. '", codes[0])

	assert.Equal(t, "x-singlequoted", tu.Type)
}

// okapi: PHPContentFilterTest#testConcatDQStringsWithCodesAndVariable
func TestSnippet_ConcatDQStringsWithCodesAndVariable(t *testing.T) {
	snippet := "$a=\"t1<b>\".$_CONFIG[\"site\"].\"</b>t2\";"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	// SourceText() returns plain text only; <b>, variable, and </b> are inline codes.
	assert.Equal(t, "t1t2", tu.SourceText())

	codes := codeData(tu.SourceRuns())
	require.Equal(t, 3, len(codes), "should have 3 inline codes: <b>, variable, </b>")
	assert.Equal(t, "<b>", codes[0])
	assert.Equal(t, "\".$_CONFIG[\"site\"].\"", codes[1])
	assert.Equal(t, "</b>", codes[2])
}

// okapi: PHPContentFilterTest#testCommaCaseWithConcat
func TestSnippet_CommaCaseWithConcat(t *testing.T) {
	snippet := "$a=test('t1', 't2 '.\"and t3\");"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2)

	assert.Equal(t, "t1", blocks[0].SourceText())

	tu2 := blocks[1]
	// The quote-switch concat operator becomes an inline code.
	assert.Equal(t, "t2 and t3", tu2.SourceText())
	assert.Equal(t, "x-mixed", tu2.Type)

	codes := codeData(tu2.SourceRuns())
	require.Equal(t, 1, len(codes), "should have 1 inline-code run for quote-switch concat")
	assert.Equal(t, "'.\"", codes[0])
}

// okapi: PHPContentFilterTest#testConcatWithVariable
func TestSnippet_ConcatWithVariable(t *testing.T) {
	snippet := "$a='t1' \r.$b.' t2';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	// The concat+variable expression becomes an inline code.
	assert.Equal(t, "t1 t2", tu.SourceText())

	codes := codeData(tu.SourceRuns())
	require.Equal(t, 1, len(codes), "should have 1 inline-code run for concat+variable")
	assert.Equal(t, "' \r.$b.'", codes[0])
}

// okapi: PHPContentFilterTest#testConcatMultipleStrings
func TestSnippet_ConcatMultipleStrings(t *testing.T) {
	snippet := "$a='t1' \r.$b.' t2' . $c.\" t3 \""
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	// Multiple concat operators become inline codes; SourceText() is plain text only.
	assert.Equal(t, "t1 t2 t3 ", tu.SourceText())
	assert.Equal(t, "x-mixed", tu.Type)

	codes := codeData(tu.SourceRuns())
	require.Equal(t, 2, len(codes), "should have 2 inline-code runs for multiple concat operators")
	assert.Equal(t, "' \r.$b.'", codes[0])
	assert.Equal(t, "' . $c.\"", codes[1])
}

// okapi: PHPContentFilterTest#testConcatWithEndings
func TestSnippet_ConcatWithEndings(t *testing.T) {
	snippet := "$a= $z.'t1' \r.$b.' t2' . $c.\" t3 \".$d;"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	// Leading $z and trailing $d are stripped; concat operators become inline codes.
	assert.Equal(t, "t1 t2 t3 ", tu.SourceText())

	codes := codeData(tu.SourceRuns())
	require.Equal(t, 2, len(codes), "should have 2 inline-code runs for concat operators")
	assert.Equal(t, "' \r.$b.'", codes[0])
	assert.Equal(t, "' . $c.\"", codes[1])
}

// okapi: PHPContentFilterTest#testConcatSGAndDQStrings
func TestSnippet_ConcatSGAndDQStrings(t *testing.T) {
	snippet := "$a='t1' . \"t2\";"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	// The quote-switch concat operator becomes an inline code.
	assert.Equal(t, "t1t2", tu.SourceText())
	assert.Equal(t, "x-mixed", tu.Type)

	codes := codeData(tu.SourceRuns())
	require.Equal(t, 1, len(codes), "should have 1 inline-code run for quote-switch concat")
	assert.Equal(t, "' . \"", codes[0])
}

// okapi: PHPContentFilterTest#testEntryWithCodes
func TestSnippet_EntryWithCodes(t *testing.T) {
	snippet := "$a='{$abc}=text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	// {$abc} becomes an inline code; SourceText() returns plain text only.
	assert.Equal(t, "=text", tu.SourceText())

	codes := codeData(tu.SourceRuns())
	require.Equal(t, 1, len(codes), "should have 1 inline-code run for {$abc}")
	assert.Equal(t, "{$abc}", codes[0])
}

// okapi: PHPContentFilterTest#testSimpleHTMLCodes
func TestSnippet_SimpleHTMLCodes(t *testing.T) {
	snippet := "$a='t<a>t</a>t<a attr=\"val\"/>t';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	// HTML tags become inline codes; SourceText() returns plain text only.
	assert.Equal(t, "tttt", tu.SourceText())

	codes := codeData(tu.SourceRuns())
	require.Equal(t, 3, len(codes), "should have 3 inline-code runs for <a>, </a>, <a attr/>")
	assert.Equal(t, "<a>", codes[0])
	assert.Equal(t, "</a>", codes[1])
	assert.Equal(t, "<a attr=\"val\"/>", codes[2])
}

// okapi: PHPContentFilterTest#testParitalStartingHTMLCodes
func TestSnippet_PartialStartingHTMLCodes(t *testing.T) {
	snippet := "$a='c attr=\"val\"> text <br/>';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	// <br/> becomes an inline code; the partial opening tag text is plain text.
	assert.Equal(t, "c attr=\"val\"> text ", tu.SourceText())

	codes := codeData(tu.SourceRuns())
	require.Equal(t, 1, len(codes), "should have 1 inline-code run for <br/>")
	assert.Equal(t, "<br/>", codes[0])
}

// okapi: PHPContentFilterTest#testParitalClosingHTMLCodes
func TestSnippet_PartialClosingHTMLCodes(t *testing.T) {
	snippet := "$a='<br/> text <a href=\"...';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	// <br/> becomes an inline code; partial closing tag text remains as plain text.
	assert.Equal(t, " text <a href=\"...", tu.SourceText())

	codes := codeData(tu.SourceRuns())
	require.Equal(t, 1, len(codes), "should have 1 inline-code run for <br/>")
	assert.Equal(t, "<br/>", codes[0])
}

// okapi: PHPContentFilterTest#testSpecialHTMLCodes
func TestSnippet_SpecialHTMLCodes(t *testing.T) {
	snippet := "$a='<!DOCTYPE...> t <?pi attr=\"val\"?> t';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	// DOCTYPE and PI become inline codes; SourceText() returns plain text only.
	assert.Equal(t, " t  t", tu.SourceText())

	codes := codeData(tu.SourceRuns())
	require.Equal(t, 2, len(codes), "should have 2 inline-code runs for DOCTYPE and PI")
	assert.Equal(t, "<!DOCTYPE...>", codes[0])
	assert.Equal(t, "<?pi attr=\"val\"?>", codes[1])
}

// okapi: PHPContentFilterTest#testEscapeCodes
func TestSnippet_EscapeCodes(t *testing.T) {
	snippet := "$a='\\n t \\r t \\n\\r t \\v t \\a';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	// Escape sequences become inline codes; SourceText() returns plain text only.
	assert.Equal(t, " t  t  t  t ", tu.SourceText())

	codes := codeData(tu.SourceRuns())
	require.Equal(t, 6, len(codes), "should have 6 inline-code runs for escape codes")
	assert.Equal(t, "\\n", codes[0])
	assert.Equal(t, "\\r", codes[1])
	assert.Equal(t, "\\n", codes[2])
	assert.Equal(t, "\\r", codes[3])
	assert.Equal(t, "\\v", codes[4])
	assert.Equal(t, "\\a", codes[5])
}

// okapi: PHPContentFilterTest#testLinefeedCodes
func TestSnippet_LinefeedCodes(t *testing.T) {
	snippet := "$a='\\n\\n';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	// No text content, only escape codes => no TU extracted.
	assert.Empty(t, blocks, "linefeed-only string should not produce a translatable block")
}

// okapi: PHPContentFilterTest#testVariableCodes
func TestSnippet_VariableCodes(t *testing.T) {
	snippet := "$a=\"t [var1] t {var2} t {$var3} t\";"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	// Variable references become inline codes; SourceText() returns plain text only.
	assert.Equal(t, "t  t  t  t", tu.SourceText())

	codes := codeData(tu.SourceRuns())
	require.Equal(t, 3, len(codes), "should have 3 inline-code runs for [var1], {var2}, {$var3}")
	assert.Equal(t, "[var1]", codes[0])
	assert.Equal(t, "{var2}", codes[1])
	assert.Equal(t, "{$var3}", codes[2])
}

// okapi: PHPContentFilterTest#testCommentsSingleLine
func TestSnippet_CommentsSingleLine(t *testing.T) {
	snippet := "// $a='abc';\n$b=\"def\";"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// Single-line comment is skipped; only $b is extracted.
	assert.Equal(t, "def", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testCommentsMultiline
func TestSnippet_CommentsMultiline(t *testing.T) {
	snippet := "/* $a='abc';\nstuff // etc. * / \n */$b=\"def\";"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// Multi-line comment is skipped; only $b is extracted.
	assert.Equal(t, "def", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testEmptyComment
func TestSnippet_EmptyComment(t *testing.T) {
	snippet := "/**/$a='abc';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "abc", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testCommentsWithApos
func TestSnippet_CommentsWithApos(t *testing.T) {
	snippet := "/** Felix's Favorites */\n$cnt['glob']['type'] = 'Felix\\'s Favorites';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Felix\\'s Favorites", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testSkipDirective
func TestSnippet_SkipDirective(t *testing.T) {
	snippet := "//_skip\n $a='skip';\n$b='text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// The _skip directive causes the next string to be skipped.
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testSkipDirectiveOnConcat
func TestSnippet_SkipDirectiveOnConcat(t *testing.T) {
	snippet := "//_skip\n $a='skip' . $x . 'skip';\n$b='text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testTextInBSkipDirective
func TestSnippet_TextInBSkipDirective(t *testing.T) {
	snippet := "//_bskip\n $a='skip';\n//_text\n$b='text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testESkipDirective
func TestSnippet_ESkipDirective(t *testing.T) {
	snippet := "//_bskip\n $a='skip';\n//_eskip\n$b='text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testDirectiveInMultilineComment
func TestSnippet_DirectiveInMultilineComment(t *testing.T) {
	snippet := "/*_skip*/ $a='skip'; $b='text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testBTextDirective
func TestSnippet_BTextDirective(t *testing.T) {
	snippet := "/*_bskip*/ $a='skip'; /*_btext*/ $b='text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testETextDirective
func TestSnippet_ETextDirective(t *testing.T) {
	snippet := "/*_bskip*/ $a='skip'; /*_btext*/ $b='textB'; /*_etext*/\n" +
		"$c='skip'; /*_eskip*/ $d='textD'"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "textB", blocks[0].SourceText())
	assert.Equal(t, "textD", blocks[1].SourceText())
}

// okapi: PHPContentFilterTest#testSkipOutsideDirective
func TestSnippet_SkipOutsideDirective(t *testing.T) {
	snippet := "$a='skip'; /*_btext*/ $b='textB';"
	params := map[string]any{
		"useDirectives":            true,
		"extractOutsideDirectives": false,
	}
	parts := readPHP(t, snippet, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "textB", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testDisabledDirectives
func TestSnippet_DisabledDirectives(t *testing.T) {
	snippet := "/*_skip*/ $a='textA'; $b='textB';"
	params := map[string]any{
		"useDirectives":            false,
		"extractOutsideDirectives": false,
	}
	parts := readPHP(t, snippet, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "textA", blocks[0].SourceText())
	assert.Equal(t, "textB", blocks[1].SourceText())
}

// okapi: PHPContentFilterTest#testDirectiveScope
func TestSnippet_DirectiveScope(t *testing.T) {
	snippet := "/*_skip*/ $a['key1']='skip'; $a['key2']='text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testSingleQuotedString
func TestSnippet_SingleQuotedString(t *testing.T) {
	snippet := "$a='\\\\text\\'';\n$b='\\'\"text\"';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2)

	// First TU: the bridge treats \t as an escape code inline, leaving \ext\' as text.
	assert.Equal(t, "\\ext\\'", blocks[0].SourceText())
	codes := codeData(blocks[0].SourceRuns())
	require.Equal(t, 1, len(codes), "should have 1 inline-code run for \\t escape")
	assert.Equal(t, "\\t", codes[0])

	// Second TU: \'\"text\" — no inline codes.
	assert.Equal(t, "\\'\"text\"", blocks[1].SourceText())
}

// okapi: PHPContentFilterTest#testDoubleQuotedString
func TestSnippet_DoubleQuotedString(t *testing.T) {
	snippet := "$a=\"text\\\"\";\n$b=\"'text\\\"\";"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2)
	// Second TU: 'text\"
	assert.Equal(t, "'text\\\"", blocks[1].SourceText())
}

// okapi: PHPContentFilterTest#testHeredocString
func TestSnippet_HeredocString(t *testing.T) {
	snippet := "$a=<<<EOT\ntext\nEOT \n\nEOT;"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	assert.Equal(t, "text\nEOT \n", tu.SourceText())
	assert.Equal(t, "x-heredoc", tu.Type)
}

// okapi: PHPContentFilterTest#testQuotedHeredocString
func TestSnippet_QuotedHeredocString(t *testing.T) {
	snippet := "$a=<<<\"EOT\"\ntext\nEOT \n\nEOT;"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	assert.Equal(t, "text\nEOT \n", tu.SourceText())
	assert.Equal(t, "x-heredoc", tu.Type)
}

// okapi: PHPContentFilterTest#testQuotedNowdocString
func TestSnippet_QuotedNowdocString(t *testing.T) {
	snippet := "$a=<<<'EOT'\ntext\nEOT \n\nEOT;"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	tu := blocks[0]
	assert.Equal(t, "text\nEOT \n", tu.SourceText())
	assert.Equal(t, "x-nowdoc", tu.Type)
}

// okapi: PHPContentFilterTest#testSemiColumnHeredocString
func TestSnippet_SemiColumnHeredocString(t *testing.T) {
	snippet := "$a=<<<EOT\ntext\nEOT \n;\nEOT;"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text\nEOT \n;", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testMultipleLinesHeredocString
func TestSnippet_MultipleLinesHeredocString(t *testing.T) {
	snippet := "$a=<<<EOT\ntext\nEOT \n EOT \n\nEOT;\n"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text\nEOT \n EOT \n", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testEmptyHeredocStringAndOutput
func TestSnippet_EmptyHeredocStringAndOutput(t *testing.T) {
	snippet := "$a=<<<EOT\n\nEOT;"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	// Empty heredoc produces no TU.
	assert.Empty(t, blocks, "empty heredoc should not produce a translatable block")
}

// okapi: PHPContentFilterTest#testWhiteHeredocStringAndOutput
func TestSnippet_WhiteHeredocStringAndOutput(t *testing.T) {
	snippet := "$a=<<<EOT\n  \t  \nEOT;"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	// Whitespace-only heredoc produces no TU.
	assert.Empty(t, blocks, "whitespace-only heredoc should not produce a translatable block")
}

// okapi: PHPContentFilterTest#testSQIndex
func TestSnippet_SQIndex(t *testing.T) {
	snippet := "$a['skip']; $arr2[  'skip' ] = 'text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// Array keys in single-quoted brackets are skipped; only the value is extracted.
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testnoStringIndex
func TestSnippet_NoStringIndex(t *testing.T) {
	snippet := "$a[2] = 'text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testDQIndex
func TestSnippet_DQIndex(t *testing.T) {
	snippet := "$a[\"skip\"]; $arr2[  \"skip\" ] = 'text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testHeredocIndex
func TestSnippet_HeredocIndex(t *testing.T) {
	snippet := "$a[ <<<key\nskip\nkey\n] = 'text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testQuotedHeredocIndex
func TestSnippet_QuotedHeredocIndex(t *testing.T) {
	snippet := "$a[ <<<\"key\"\nskip\nkey\n] = 'text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testNowdocIndex
func TestSnippet_NowdocIndex(t *testing.T) {
	snippet := "$a[ <<<'key'\nskip\nkey\n] = 'text';"
	parts := readPHPDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testFilteringOfHtmlLikeTags
func TestSnippet_FilteringOfHtmlLikeTags(t *testing.T) {
	tests := []struct {
		name     string
		snippet  string
		wantText string
	}{
		{
			name:     "greater-than not tag",
			snippet:  "'Some value, which is not tag > 15\u00b0.'",
			wantText: "Some value, which is not tag > 15\u00b0.",
		},
		{
			name:     "less-than not tag",
			snippet:  "'Some value, which is not tag < 15\u00b0.'",
			wantText: "Some value, which is not tag < 15\u00b0.",
		},
		{
			name:     "opening tag-like",
			snippet:  "'<Some value, which is tag > 15\u00b0.'",
			wantText: " 15\u00b0.",
		},
		{
			name:     "closing tag-like",
			snippet:  "'</Some value, which is tag > 15\u00b0.'",
			wantText: " 15\u00b0.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := readPHPDefault(t, tc.snippet)
			blocks := bridgetest.TranslatableBlocks(parts)
			require.NotEmpty(t, blocks, "should produce at least one translatable block")
			assert.Equal(t, tc.wantText, model.RunsPlainText(blocks[0].SourceRuns()))
		})
	}
}
