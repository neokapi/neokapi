package tex_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/tex"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: read a string and return all parts
func readString(t *testing.T, input string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := tex.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

// helper: read a string and return only blocks
func readBlocks(t *testing.T, input string) []*model.Block {
	t.Helper()
	return testutil.FilterBlocks(readString(t, input))
}

// helper: read a string with a custom config modifier and return all parts
func readStringWithConfig(t *testing.T, input string, configure func(*tex.Config)) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := tex.NewReader()
	configure(reader.TexConfig())
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

// disableNonTranslatableContent is the Okapi-faithful config modifier that
// keeps verbatim/lstlisting code and math as opaque Data/skeleton.
func disableNonTranslatableContent(c *tex.Config) {
	c.SetExtractNonTranslatableContent(false)
}

// nonTranslatableContentBlocks returns every Block part with Translatable=false
// (the surfaced code/formula content blocks).
func nonTranslatableContentBlocks(parts []*model.Part) []*model.Block {
	var content []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b := p.Resource.(*model.Block); !b.Translatable {
				content = append(content, b)
			}
		}
	}
	return content
}

// translatableBlocks returns every Block part with Translatable=true.
func translatableBlocks(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b := p.Resource.(*model.Block); b.Translatable {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks
}

// helper: return block source texts
func blockTexts(blocks []*model.Block) []string {
	return testutil.BlockTexts(blocks)
}

// ---- TEXFilterTest ----

// okapi: TEXFilterTest#testSimpleText
func TestSimpleText(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
Hello world.
\end{document}`)
	require.GreaterOrEqual(t, len(blocks), 1)
	assert.Contains(t, blocks[0].SourceText(), "Hello world")
}

// okapi: TEXFilterTest#testSimpleText (multi-paragraph)
func TestSimpleText_MultiParagraph(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
First paragraph.

Second paragraph.
\end{document}`)
	require.Len(t, blocks, 2)
	assert.Contains(t, blocks[0].SourceText(), "First paragraph")
	assert.Contains(t, blocks[1].SourceText(), "Second paragraph")
}

// okapi: TEXFilterTest#testMathMode
// Okapi-faithful config (surfacing off): inline math stays opaque Data.
func TestMathMode(t *testing.T) {
	// Inline math $...$ should be excluded from text
	parts := readStringWithConfig(t, `\begin{document}
$E = mc^2$
\end{document}`, disableNonTranslatableContent)

	blocks := testutil.FilterBlocks(parts)
	// The inline math should be emitted as Data, not Block
	for _, b := range blocks {
		assert.NotContains(t, b.SourceText(), "E = mc^2",
			"math mode content should not appear in translatable blocks")
	}

	// Verify Data parts contain the math
	var hasMathData bool
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Properties != nil && strings.Contains(data.Properties["content"], "$E = mc^2$") {
				hasMathData = true
			}
		}
	}
	assert.True(t, hasMathData, "math should be in a Data part")
}

// By default (ExtractNonTranslatableContent on), inline math $...$ is surfaced
// as a non-translatable RoleFormula content block — visible to ingestion,
// skipped by MT — with the `$` delimiters kept in skeleton, and the document
// still round-trips byte-exact.
func TestMathMode_InlineAsContent(t *testing.T) {
	input := "\\begin{document}\nEnergy is $E = mc^2$ here.\n\\end{document}"
	parts := readString(t, input)

	var translatable, content []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.Translatable {
				translatable = append(translatable, b)
			} else {
				content = append(content, b)
			}
		}
	}
	require.Len(t, content, 1, "inline math surfaces as one non-translatable content block")
	math := content[0]
	assert.False(t, math.Translatable)
	assert.Equal(t, model.RoleFormula, math.SemanticRole())
	assert.True(t, math.PreserveWhitespace)
	assert.Equal(t, "E = mc^2", math.SourceText(), "body excludes the $ delimiters")
	for _, b := range translatable {
		assert.NotContains(t, b.SourceText(), "E = mc^2",
			"math body never reaches a translatable block")
	}

	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input), "round-trip stays byte-exact")
}

// okapi: TEXFilterTest#testMathMode (display math $$)
// Okapi-faithful config (surfacing off): display $$ math stays opaque Data.
func TestMathMode_DisplayDollar(t *testing.T) {
	parts := readStringWithConfig(t, `\begin{document}
$$a^2 + b^2 = c^2$$
\end{document}`, disableNonTranslatableContent)

	blocks := testutil.FilterBlocks(parts)
	for _, b := range blocks {
		assert.NotContains(t, b.SourceText(), "a^2")
	}
}

// By default, display math $$...$$ surfaces as a RoleFormula content block and
// round-trips byte-exact.
func TestMathMode_DisplayDollarAsContent(t *testing.T) {
	input := "\\begin{document}\n$$a^2 + b^2 = c^2$$\n\\end{document}"
	parts := readString(t, input)

	content := nonTranslatableContentBlocks(parts)
	require.Len(t, content, 1)
	assert.Equal(t, model.RoleFormula, content[0].SemanticRole())
	assert.Equal(t, "a^2 + b^2 = c^2", content[0].SourceText())
	assert.True(t, content[0].PreserveWhitespace)
	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input), "round-trip stays byte-exact")
}

// okapi: TEXFilterTest#testMathMode (display math \[...\])
// Okapi-faithful config (surfacing off): display \[..\] math stays opaque Data.
func TestMathMode_DisplayBracket(t *testing.T) {
	parts := readStringWithConfig(t, `\begin{document}
\[x = \frac{-b}{2a}\]
\end{document}`, disableNonTranslatableContent)

	blocks := testutil.FilterBlocks(parts)
	for _, b := range blocks {
		assert.NotContains(t, b.SourceText(), "frac")
	}
}

// By default, display math \[...\] surfaces as a RoleFormula content block and
// round-trips byte-exact.
func TestMathMode_DisplayBracketAsContent(t *testing.T) {
	input := "\\begin{document}\n\\[x = \\frac{-b}{2a}\\]\n\\end{document}"
	parts := readString(t, input)

	content := nonTranslatableContentBlocks(parts)
	require.Len(t, content, 1)
	assert.Equal(t, model.RoleFormula, content[0].SemanticRole())
	assert.Equal(t, `x = \frac{-b}{2a}`, content[0].SourceText())
	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input), "round-trip stays byte-exact")
}

// okapi: TEXFilterTest#testComments
func TestComments(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
% This is a comment
Hello after comment.
\end{document}`)

	texts := blockTexts(blocks)
	for _, text := range texts {
		assert.NotContains(t, text, "This is a comment",
			"comment text should not be translatable")
	}
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Hello after comment")
}

// neokapi-only: minimal Cyrillic body smoke test. The upstream
// TEXFilterTest#testRussian behavior (preamble skipped, abstract body
// extracted) is covered by TestRussian below; this just guards basic
// UTF-8 Cyrillic passthrough in a body paragraph.
func TestRussianCyrillic(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
Привет мир!
\end{document}`)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Привет мир!")
}

// okapi: TEXFilterTest#testRussian
// Upstream feeds a full Russian preamble (\documentclass, several
// \usepackage[..]{..}, %comments, \hyphenation, \tableofcontents) and
// asserts the only translatable body text comes from inside the
// abstract environment. The native reader routes every preamble command
// and comment to Data (non-translatable) and emits the abstract body
// "Это вводный абзац в начале документа." as the sole block — matching
// upstream's contract that preamble structure is opaque and the abstract
// prose is translatable. (Upstream additionally emits a leading
// whitespace-only TU "  "; native trims surrounding whitespace into the
// skeleton instead, so only the meaningful prose surfaces as a block.)
func TestRussian(t *testing.T) {
	parts := readString(t, `\documentclass{article}

%Russian-specific packages
%--------------------------------------
\usepackage[T2A]{fontenc}
\usepackage[utf8]{inputenc}
\usepackage[russian]{babel}
%--------------------------------------

%Hyphenation rules
%--------------------------------------
\usepackage{hyphenat}
\hyphenation{ма-те-ма-ти-ка вос-ста-нав-ли-вать}
%--------------------------------------

\begin{document}

\tableofcontents

\begin{abstract}
  Это вводный абзац в начале документа.
\end{abstract}`)

	blocks := testutil.FilterBlocks(parts)
	texts := blockTexts(blocks)
	// The abstract prose is the only translatable text unit.
	assert.Equal(t, []string{"Это вводный абзац в начале документа."}, texts)

	// Preamble packages / hyphenation / comments stay non-translatable.
	for _, text := range texts {
		assert.NotContains(t, text, "usepackage")
		assert.NotContains(t, text, "hyphenat")
		assert.NotContains(t, text, "Russian-specific")
	}
}

// okapi: TEXFilterTest#testLineBreaks
func TestLineBreaks(t *testing.T) {
	// Single newline does NOT start new paragraph — text continues
	blocks := readBlocks(t, `\begin{document}
Line one
Line two
\end{document}`)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Line one")
	assert.Contains(t, text, "Line two")
}

// okapi: TEXFilterTest#testLineBreaks (double newline)
func TestLineBreaks_DoubleSplits(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
Line one

Line two
\end{document}`)
	require.Len(t, blocks, 2)
	assert.Contains(t, blocks[0].SourceText(), "Line one")
	assert.Contains(t, blocks[1].SourceText(), "Line two")
}

// neokapi-only: hardening check that an unclosed inline command
// (\textbf{ with no matching brace) does not crash the parser. The
// upstream runaway-brace contract is verified by TestRunawayCurly below
// against the exact upstream snippet.
func TestRunawayBraces(t *testing.T) {
	// Malformed braces should not crash the parser
	parts := readString(t, `\begin{document}
\textbf{unclosed bold
Some text after.
\end{document}`)
	// Should not panic, should produce some parts
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: TEXFilterTest#testRunawayCurly
// Upstream snippet: a \newcommand whose body is a \code macro followed
// by a \title. Upstream asserts the parser survives the runaway curly
// braces from the macro-with-argument form (\newcommand{\code}[1]{...})
// and still reaches the trailing \title. The native reader treats
// \newcommand as a no-text command (its \code argument → Data), the
// runaway "[1]{\small\texttt{#1}}" fragments fall through to body
// handling without crashing, and the trailing \title{...} surfaces as a
// translatable block — the same "survive the runaway, recover at the
// next real command" contract.
func TestRunawayCurly(t *testing.T) {
	parts := readString(t, `\begin{document}
\newcommand{\code}[1]{\small\texttt{#1}}
\title{Tilde's Machine Translation Systems for WMT 2017}`)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	// Parser recovers and extracts the trailing \title as a block.
	blocks := testutil.FilterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Tilde's Machine Translation Systems for WMT 2017")
}

// neokapi-only: focused classification probe for the no-text command
// family (\label, \ref, \cite). The exact upstream
// TEXFilterTest#testOneArgNoTextCommands behavior is verified by
// TestOneArgNoTextCommands below.
func TestCommandClassification_NoText(t *testing.T) {
	// \label, \ref, \cite arguments are non-translatable
	blocks := readBlocks(t, `\begin{document}
\label{sec:intro}
\ref{sec:intro}
\cite{knuth1984}
Some text.
\end{document}`)

	texts := blockTexts(blocks)
	for _, text := range texts {
		assert.NotContains(t, text, "sec:intro")
		assert.NotContains(t, text, "knuth1984")
	}
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[len(blocks)-1].SourceText(), "Some text")
}

// neokapi-only: focused classification probe for inline-text commands.
// The exact upstream TEXFilterTest#testOneArgInlineTextCommands behavior
// is verified by TestOneArgInlineTextCommands below.
func TestCommandClassification_InlineText(t *testing.T) {
	// \textbf, \emph arguments contain translatable text
	blocks := readBlocks(t, `\begin{document}
This is \textbf{bold text} here.
\end{document}`)

	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "bold text")
}

// neokapi-only: focused classification probe for paragraph-text
// commands. The exact upstream TEXFilterTest#testoneArgParaTextCommands
// behavior is verified by TestOneArgParaTextCommands below.
func TestCommandClassification_ParagraphText(t *testing.T) {
	// \section produces a separate text unit
	blocks := readBlocks(t, `\begin{document}
\section{My Section Title}
Paragraph text.
\end{document}`)

	require.GreaterOrEqual(t, len(blocks), 2)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "My Section Title")
	assert.Contains(t, texts, "Paragraph text.")
}

// okapi: TEXFilterTest#testOneArgNoTextCommands
// Upstream snippet is a header block of no-text commands
// (\documentclass[..]{..}, several \usepackage{..}, %comments, and the
// bare custom command \ijcnlpfinalcopy) with no \begin{document}. It
// asserts there are NO translatable text units — every token lands in a
// non-translatable document part. The native reader produces zero
// blocks: documentclass/usepackage are no-text commands, the comments
// route to Data, and the unknown \ijcnlpfinalcopy also becomes Data.
func TestOneArgNoTextCommands(t *testing.T) {
	parts := readString(t, "%\n"+"\n"+"\\documentclass[11pt, letterpaper]{article}\n"+
		"\\usepackage{ijcnlp2017}\n"+"\\usepackage{times}\n"+"\\usepackage{placeins}\n"+"\n"+
		"% Uncomment this line for the final submission:\n"+"\\ijcnlpfinalcopy")

	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks, "no-text command header yields no translatable units")
}

// okapi: TEXFilterTest#testOneArgInlineTextCommands
// Upstream snippet mixes inline-text commands (\textbf{article},
// \hbox{...}) with ordinary body prose inside \begin{document}. It
// asserts the preamble part is "\begin{document}\n%\n\n" and that the
// inline-command argument text ("article", "ijcnlp2017", the ordinary
// prose, "and more, weird text") all merges into the translatable body.
// The native reader emits the leading comment as Data and folds the
// \textbf / \hbox argument text into the body block alongside the
// surrounding prose.
func TestOneArgInlineTextCommands(t *testing.T) {
	parts := readString(t, "\\begin{document}\n%\n"+"\n"+"\\textbf{article}\n"+"\\hbox{ijcnlp2017}\n"+
		" And some ordinary text"+"\\hbox{and more, weird text} \n"+"{\\tt and one more different style command}\n")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)
	var all strings.Builder
	for _, b := range blocks {
		all.WriteString(b.SourceText())
		all.WriteString("\n")
	}
	merged := all.String()
	// Inline-command argument text is translatable (merged into body).
	assert.Contains(t, merged, "article")
	assert.Contains(t, merged, "ijcnlp2017")
	assert.Contains(t, merged, "And some ordinary text")
	assert.Contains(t, merged, "and more, weird text")
}

// okapi: TEXFilterTest#testoneArgParaTextCommands
// Upstream snippet places paragraph-text commands (\title, \section) and
// plain paragraphs (split by blank lines) inside \begin{document}. It
// asserts the \title argument "Installing \LaTeX" and \section argument
// "One [just text] more section" each become their own resource, and the
// surrounding paragraphs ("La la la some paragraph", "Some text",
// "Split with many newlines") are separate text units. The native reader
// extracts the \title and \section as typed blocks and splits the body
// prose into separate paragraph blocks on blank lines, matching the
// upstream segmentation. (The native \title block drops the trailing
// inline \LaTeX code into the placeholder stream, so its plain text is
// "Installing ".)
func TestOneArgParaTextCommands(t *testing.T) {
	parts := readString(t, "\\begin{document}\n%\n"+"\n"+"\\title{Installing \\LaTeX}\n"+"La la la some paragraph\n"+
		"\\section{One [just text] more section}\n"+"Some text \n"+"\n"+"Split with many newlines")

	blocks := testutil.FilterBlocks(parts)
	texts := blockTexts(blocks)

	// \title and \section each produce their own text unit.
	assert.Contains(t, texts, "Installing ")
	assert.Contains(t, texts, "One [just text] more section")
	// Body prose splits into separate paragraph units.
	assert.Contains(t, texts, "La la la some paragraph")
	assert.Contains(t, texts, "Some text")
	assert.Contains(t, texts, "Split with many newlines")
}

// okapi: TEXFilterTest#testHeaderCommands
func TestHeaderCommands(t *testing.T) {
	// Content between \documentclass and \begin{document} is mostly non-translatable
	parts := readString(t, `\documentclass{article}
\usepackage{amsmath}
\begin{document}
Body text.
\end{document}`)

	blocks := testutil.FilterBlocks(parts)
	texts := blockTexts(blocks)
	for _, text := range texts {
		assert.NotContains(t, text, "amsmath",
			"usepackage argument should not be translatable")
	}
}

// okapi: TEXFilterTest#testHeaderText
func TestHeaderText(t *testing.T) {
	// \title and \author in header ARE translatable
	blocks := readBlocks(t, `\documentclass{article}
\title{My Title}
\author{John Doe}
\begin{document}
\end{document}`)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "My Title")
	assert.Contains(t, texts, "John Doe")
}

// okapi: TEXFilterTest#testLatvianSymbols
func TestLatvianSymbols(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
Latvian: āčēģīķļņšūž
\end{document}`)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "āčēģīķļņšūž")
}

// neokapi-only: probes that tabular content is reachable. The upstream
// table contract (the whole \begin{table}...\end{table} span is opaque)
// is verified by TestTable / TestTable2 below.
func TestTableEnvironment(t *testing.T) {
	// tabular environment content should be parsed
	blocks := readBlocks(t, `\begin{document}
\begin{tabular}{|c|c|}
Cell one & Cell two
\end{tabular}
\end{document}`)
	require.NotEmpty(t, blocks)
	// The content within the tabular environment should be extracted
	var allTextSb273 strings.Builder
	for _, b := range blocks {
		allTextSb273.WriteString(b.SourceText())
	}
	assert.Contains(t, allTextSb273.String(), "Cell one")
}

// okapi: TEXFilterTest#testTable
// Upstream feeds a full float table (\begin{table}[] ... \centering ...
// \begin{tabular}{|lcc|} ... \multicolumn ... \caption ... \label ...
// \end{table}) and asserts exactly ONE document part holding the entire
// table span verbatim and ZERO text units. With the Okapi-faithful config
// (ExtractNonTranslatableContent off), the native reader captures the whole
// \begin{table}...\end{table} span as a single opaque Data part — matching
// upstream's "the whole table is data" contract. (With the flag on, the
// nested \caption is surfaced as a RoleCaption content block; see
// TestTableCaptionAsContent.)
func TestTable(t *testing.T) {
	snippet := "\\begin{table}[]\n" +
		"\\centering\n" +
		"\\begin{tabular}{|lcc|}\n" +
		"\\hline\n" +
		"\\multicolumn{1}{|c}{\\multirow{2}{*}{\\textbf{Corpus}}} & \\textbf{Sentences} & \\textbf{Sentences} \\\\\n" +
		"\\multicolumn{1}{|c}{} & \\textbf{before filtering} & \\textbf{after filtering} \\\\ \\hline\n" +
		"Parallel & \\multicolumn{1}{r}{378,869} & \\multicolumn{1}{r|}{325,332} \\\\\n" +
		"Monolingual & \\multicolumn{1}{r}{378,869} & \\multicolumn{1}{r|}{332,652} \\\\ \\hline\n" +
		"\\end{tabular}\n" +
		"\\caption{Statistics of the training corpora}\n" +
		"\\label{data-table}\n" +
		"\\end{table}"

	parts := readStringWithConfig(t, snippet, disableNonTranslatableContent)
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks, "table environment yields no translatable units")

	// Exactly one Data part holds the verbatim table span.
	var data []*model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			data = append(data, p.Resource.(*model.Data))
		}
	}
	require.Len(t, data, 1)
	assert.Equal(t, snippet, data[0].Properties["content"])
}

// okapi: TEXFilterTest#testTable2
// A second float-table variant (different column spec, math cells like
// 46.57$\pm$1.46, trailing newline). Upstream again asserts one
// document part for the whole span and no text units. With the
// Okapi-faithful config (surfacing off), the native reader captures the
// entire \begin{table}...\end{table} span as one opaque Data part; the
// trailing "\n" after \end{table} is folded into the inter-part skeleton,
// so no spurious block appears.
func TestTable2(t *testing.T) {
	snippet := "\\begin{table}[]\n" +
		"\\centering\n" +
		"\\begin{tabular}{|lrrr|}\n" +
		"\\hline\n" +
		"\\multicolumn{1}{|c}{\\textbf{System}} & \\multicolumn{1}{c}{\\textbf{BLEU}} & \\multicolumn{1}{c}{\\textbf{NIST}} & \\multicolumn{1}{c|}{\\textbf{ChrF2}} \\\\ \\hline\n" +
		"SMT & 46.57$\\pm$1.46 & 9.45$\\pm$0.18 & 0.7586 \\\\\n" +
		"NMT & 38.44$\\pm$1.62 & 8.63$\\pm$0.15 & 0.7065 \\\\ \\hline\n" +
		"\\end{tabular}\n" +
		"\\caption{Automatic evaluation results}\n" +
		"\\label{mt-eval-table}\n" +
		"\\end{table}\n"

	parts := readStringWithConfig(t, snippet, disableNonTranslatableContent)
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks, "table environment yields no translatable units")

	var data []*model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			data = append(data, p.Resource.(*model.Data))
		}
	}
	require.NotEmpty(t, data)
	// The whole table span (sans trailing newline, which lands in
	// skeleton) is captured verbatim in the first Data part.
	assert.Equal(t, strings.TrimRight(snippet, "\n"), data[0].Properties["content"])
}

// By default (ExtractNonTranslatableContent on), a nested \caption inside a
// table/figure float is surfaced as a non-translatable RoleCaption content
// block — visible to ingestion/LLM consumers, skipped by MT — while the rest
// of the float (tabular grid, \label, \centering, the \begin/\end tags, and
// the \caption{ / } delimiters themselves) stays skeleton, so the whole span
// round-trips byte-exact. No translatable unit is produced.
func TestTableCaptionAsContent(t *testing.T) {
	snippet := "\\begin{table}[]\n" +
		"\\centering\n" +
		"\\begin{tabular}{|lrrr|}\n" +
		"\\hline\n" +
		"SMT & 46.57 & 9.45 & 0.7586 \\\\ \\hline\n" +
		"\\end{tabular}\n" +
		"\\caption{Automatic evaluation results}\n" +
		"\\label{mt-eval-table}\n" +
		"\\end{table}"

	parts := readString(t, snippet)
	assert.Empty(t, translatableBlocks(parts),
		"table float yields no translatable units")

	content := nonTranslatableContentBlocks(parts)
	require.Len(t, content, 1, "the nested \\caption is surfaced as one content block")
	capt := content[0]
	assert.False(t, capt.Translatable)
	assert.Equal(t, model.RoleCaption, capt.SemanticRole())
	assert.Equal(t, "Automatic evaluation results", capt.SourceText(),
		"caption body is the verbatim text between \\caption{ and }")
	// The tabular grid / label never leak into any block.
	for _, b := range testutil.FilterBlocks(parts) {
		assert.NotContains(t, b.SourceText(), "tabular")
		assert.NotContains(t, b.SourceText(), "mt-eval-table")
	}

	assert.Equal(t, snippet, snippetRoundtripWithSkeleton(t, snippet),
		"round-trip stays byte-exact")
}

// A figure float with a caption behaves the same way: the caption surfaces as
// a RoleCaption content block and the span round-trips byte-exact. A
// caption containing inline markup is carried as a single verbatim run (no
// inline parse), matching the content-block contract.
func TestFigureCaptionAsContent(t *testing.T) {
	snippet := "\\begin{figure}\n" +
		"\\centering\n" +
		"\\includegraphics{plot.png}\n" +
		"\\caption{Influence of \\textbf{word order} on BLEU}\n" +
		"\\label{wo}\n" +
		"\\end{figure}"

	parts := readString(t, snippet)
	assert.Empty(t, translatableBlocks(parts))

	content := nonTranslatableContentBlocks(parts)
	require.Len(t, content, 1)
	assert.Equal(t, model.RoleCaption, content[0].SemanticRole())
	assert.Equal(t, "Influence of \\textbf{word order} on BLEU", content[0].SourceText())

	assert.Equal(t, snippet, snippetRoundtripWithSkeleton(t, snippet),
		"round-trip stays byte-exact")
}

// With surfacing disabled, table/figure floats stay opaque Data — no caption
// content block, the whole span in one Data part, exactly as before.
func TestFloatCaption_DisabledStaysData(t *testing.T) {
	snippet := "\\begin{figure}\n\\caption{A caption}\n\\end{figure}"

	parts := readStringWithConfig(t, snippet, disableNonTranslatableContent)
	assert.Empty(t, testutil.FilterBlocks(parts), "no blocks when surfacing is off")

	var data []*model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			data = append(data, p.Resource.(*model.Data))
		}
	}
	require.Len(t, data, 1)
	assert.Equal(t, snippet, data[0].Properties["content"])
}

// A float with no \caption stays fully opaque even with surfacing on — there
// is nothing to surface, so the whole span round-trips byte-exact via the
// opaque path and yields no block.
func TestFloatWithoutCaption_StaysOpaque(t *testing.T) {
	snippet := "\\begin{table}\n\\begin{tabular}{cc}a & b\\end{tabular}\n\\end{table}"

	parts := readString(t, snippet)
	assert.Empty(t, testutil.FilterBlocks(parts), "caption-less float yields no block")
	assert.Equal(t, snippet, snippetRoundtripWithSkeleton(t, snippet),
		"round-trip stays byte-exact")
}

// A commented-out \caption inside a float must NOT be surfaced.
func TestFloatCommentedCaptionNotSurfaced(t *testing.T) {
	snippet := "\\begin{figure}\n" +
		"% \\caption{Disabled caption}\n" +
		"\\caption{Real caption}\n" +
		"\\end{figure}"

	parts := readString(t, snippet)
	content := nonTranslatableContentBlocks(parts)
	require.Len(t, content, 1, "only the live \\caption is surfaced")
	assert.Equal(t, "Real caption", content[0].SourceText())
	assert.Equal(t, snippet, snippetRoundtripWithSkeleton(t, snippet))
}

// neokapi-only: probes that inline math $...$ scripts are excluded from
// translatable text. The upstream TEXFilterTest#testScript behavior
// (header script commands skipped, body prose extracted) is verified by
// TestScript below.
func TestScripts(t *testing.T) {
	// Subscript/superscript in math mode — non-translatable. Okapi-faithful
	// config (surfacing off) keeps the math opaque; only translatable blocks
	// are checked so the math content never reaches MT either way.
	parts := readStringWithConfig(t, `\begin{document}
$x^2$ and $x_i$
\end{document}`, disableNonTranslatableContent)
	blocks := testutil.FilterBlocks(parts)
	for _, b := range blocks {
		assert.NotContains(t, b.SourceText(), "x^2")
		assert.NotContains(t, b.SourceText(), "x_i")
	}
}

// okapi: TEXFilterTest#testScript
// Upstream snippet is a homework-template header (tab-indented
// \pagestyle, \lhead{\hmwkAuthorName}, \chead{...}, \rhead, \lfoot, each
// trailed by a "% ..." comment) followed by \begin{document}, two body
// paragraphs split by a blank line, and \end{document}. Upstream asserts
// the first body text unit is "\tJust text" (leading tab preserved).
// The native reader routes all the header style commands and their
// trailing comments to Data, then extracts "Just text" and "More text"
// as the two body blocks. (Native trims the leading tab into skeleton,
// so SourceText is "Just text"; the tab round-trips via the skeleton.)
func TestScript(t *testing.T) {
	snippet := "\t% Set up the header and footer\n" +
		"\t\\pagestyle{fancy}\n" +
		"\t\\lhead{\\hmwkAuthorName} % Top left header\n" +
		"\t\\chead{\\hmwkClass\\ (\\hmwkClassInstructor\\ \\hmwkClassTime): \\hmwkTitle} % Top center head\n" +
		"\t\\rhead{\\firstxmark} % Top right header\n" +
		"\t\\lfoot{\\lastxmark} % Bottom left footer\n" +
		"\t\\begin{document}\n" +
		"\tJust text\n\nMore text" +
		"\t\\end{document}"

	parts := readString(t, snippet)
	blocks := testutil.FilterBlocks(parts)
	texts := blockTexts(blocks)
	// Two body paragraphs are extracted; header style commands are not.
	assert.Equal(t, []string{"Just text", "More text"}, texts)
	for _, text := range texts {
		assert.NotContains(t, text, "hmwk")
		assert.NotContains(t, text, "pagestyle")
	}
}

// okapi: TEXFilterTest#testNested
// Upstream snippet defines a \newcommand whose body nests \nobreak and
// \extramarks{#1}{#1 continued...} commands, terminated by "}x". It
// asserts the parser survives the nested-command body (3 events: start,
// one document part, end) with no crash. The native reader treats
// \newcommand as a no-text command, routes \nobreak / \extramarks to
// Data, and survives the runaway "}x" tail without panicking — the same
// "nested macro body parses safely" contract. (Native models the parse
// with more granular Data/Block parts than upstream's single document
// part, but neither emits the macro-body identifiers as translatable
// text and both reach END cleanly.)
func TestNested(t *testing.T) {
	parts := readString(t, "\\newcommand{\\enterProblemHeader}[1]{\n"+
		"\\nobreak\\extramarks{#1}{#1 continued on next page\\ldots}\\nobreak\n"+
		"}x\n")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	// No translatable text leaks the macro-name identifiers.
	for _, b := range testutil.FilterBlocks(parts) {
		assert.NotContains(t, b.SourceText(), "enterProblemHeader")
		assert.NotContains(t, b.SourceText(), "extramarks")
	}
}

// neokapi-only: probes that \footnote argument text is reachable as
// translatable content. No dedicated upstream TEXFilterTest method
// exists for footnotes (the upstream filter treats \footnote as an
// inline-text command, exercised indirectly via testHierarchy); this
// guards the native inline-text classification for \footnote.
func TestFootnotes(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
Main text\footnote{This is the footnote content.} continues.
\end{document}`)

	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "This is the footnote content.")
}

// okapi-skip: TEXFilterTest#testJava8Split — pure JVM behavior assertion.
// The upstream test verifies java.lang.String.split("") edge cases
// (empty-lead-string removal in Java 8); it exercises no TEXFilter code
// path and has no analog in the Go reader, whose tokenizer does not use
// regex splitting.
func TestJava8Split_NotApplicable(t *testing.T) {
	t.Skip("Java String.split semantics; not applicable to the Go reader")
}

// okapi: TEXFilterTest#testEquation
// Upstream feeds a standalone \begin{equation} ... \end{equation} block
// (with a \textup subscript) and asserts a single document part holding
// the whole equation verbatim and no text units. The native reader
// registers equation as a non-translatable environment, so the entire
// \begin{equation}...\end{equation} span is one opaque Data part.
func TestEquation(t *testing.T) {
	snippet := "\\begin{equation}\n" +
		"  S_\\textup{IC} = S_{123}\n" +
		"\\end{equation}"

	// Okapi-faithful config (surfacing off): the whole equation span is one
	// opaque Data part with no blocks.
	parts := readStringWithConfig(t, snippet, disableNonTranslatableContent)
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks, "equation environment yields no translatable units")

	var data []*model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			data = append(data, p.Resource.(*model.Data))
		}
	}
	require.Len(t, data, 1)
	assert.Equal(t, snippet, data[0].Properties["content"])
}

// By default, an equation environment surfaces its body as a non-translatable
// RoleFormula content block — visible to ingestion, skipped by MT — while
// \begin{equation}/\end{equation} stay skeleton and the span round-trips
// byte-exact. No translatable unit is produced.
func TestEquationEnvironmentAsContent(t *testing.T) {
	snippet := "\\begin{equation}\n" +
		"  S_\\textup{IC} = S_{123}\n" +
		"\\end{equation}"

	parts := readString(t, snippet)
	assert.Empty(t, translatableBlocks(parts),
		"equation environment yields no translatable units")

	content := nonTranslatableContentBlocks(parts)
	require.Len(t, content, 1)
	math := content[0]
	assert.Equal(t, model.RoleFormula, math.SemanticRole())
	assert.True(t, math.PreserveWhitespace)
	assert.Equal(t, "\n  S_\\textup{IC} = S_{123}\n", math.SourceText(),
		"body is the verbatim span between begin/end tags")

	assert.Equal(t, snippet, snippetRoundtripWithSkeleton(t, snippet), "round-trip stays byte-exact")
}

// By default, a verbatim environment surfaces its body as a non-translatable
// RoleCode content block (code visible to ingestion, skipped by MT) with the
// \begin{verbatim}/\end{verbatim} delimiters kept in skeleton; round-trip is
// byte-exact. lstlisting behaves the same way.
func TestVerbatimEnvironmentAsContent(t *testing.T) {
	for _, env := range []string{"verbatim", "lstlisting"} {
		t.Run(env, func(t *testing.T) {
			snippet := "Before\n\n\\begin{" + env + "}\n" +
				"x = f(y)\nprint(x)\n" +
				"\\end{" + env + "}\n\nAfter"

			parts := readString(t, snippet)
			content := nonTranslatableContentBlocks(parts)
			require.Len(t, content, 1)
			code := content[0]
			assert.False(t, code.Translatable)
			assert.Equal(t, model.RoleCode, code.SemanticRole())
			assert.True(t, code.PreserveWhitespace)
			assert.Contains(t, code.SourceText(), "x = f(y)")
			assert.Contains(t, code.SourceText(), "print(x)")

			// The surrounding prose stays translatable.
			texts := blockTexts(translatableBlocks(parts))
			assert.Equal(t, []string{"Before", "After"}, texts)

			assert.Equal(t, snippet, snippetRoundtripWithSkeleton(t, snippet),
				"round-trip stays byte-exact")
		})
	}
}

// With surfacing disabled, verbatim/lstlisting stay opaque Data, exactly as
// before — no content block, the whole span in one Data part.
func TestVerbatimEnvironment_DisabledStaysData(t *testing.T) {
	snippet := "\\begin{verbatim}\nx = f(y)\n\\end{verbatim}"

	parts := readStringWithConfig(t, snippet, disableNonTranslatableContent)
	assert.Empty(t, testutil.FilterBlocks(parts), "no blocks when surfacing is off")

	var data []*model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			data = append(data, p.Resource.(*model.Data))
		}
	}
	require.Len(t, data, 1)
	assert.Equal(t, snippet, data[0].Properties["content"])
}

// okapi: TEXFilterTest#testHierarchy
// Upstream snippet leads with a \title{...} (containing a \c{n} cedilla
// accent and surrounding prose with \ref / \emph / \textbf / \footnote /
// \cite). It asserts the \title argument becomes the first text unit's
// translatable text. The native reader extracts \title as the first
// block; the no-text \ref / \cite arguments and \footnote/\emph code
// bytes never surface as bare identifiers in the prose blocks.
//
// Divergence (honest): upstream decodes \c{n} → "ņ" giving "Skadiņas";
// the native accentMap covers \v (caron) and \= (macron) but not \c
// (cedilla) by design (see accentMap doc in reader.go), so the native
// title text is "...Inguna Skadias". This test asserts the shared
// structural contract (title is first translatable block, prefix up to
// the cedilla matches, reference/citation keys excluded) without
// claiming the cedilla decode.
func TestHierarchy(t *testing.T) {
	snippet := "\\title{NMT or SMT: Case Study of a Narrow-domain English-Latvian Post-editing Project by Inguna Skadi\\c{n}as}\n" +
		"For inter-annotator agreement, we calculated free-marginal kappa in three different conditions (see Table \\ref{agreement-table}): \\emph{perfect \\textbf{match} analysis}" +
		"\\footnote{Free-marginal kappa is interpreted as: 0.01-0.20 = slight agreement \\cite{landis1977measurement}}."

	parts := readString(t, snippet)
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	// \title is the first translatable text unit.
	first := blocks[0]
	assert.Equal(t, "title", first.Type)
	assert.Contains(t, first.SourceText(),
		"NMT or SMT: Case Study of a Narrow-domain English-Latvian Post-editing Project by Inguna Skadi")

	// No-text reference/citation keys never leak into translatable text.
	for _, text := range blockTexts(blocks) {
		assert.NotContains(t, text, "agreement-table")
		assert.NotContains(t, text, "landis1977measurement")
	}
}

// okapi: TEXFilterTest#testSplitTUonNewlines
// Upstream snippet "\\begin{document}\nFirst text\n\nSecond text\nThird
// text" asserts that a blank line splits text units while a single
// newline does not: "First text" is one unit and "Second text\nThird
// text" is the next. The native reader applies the same rule —
// \begin{document} → Data, then two body blocks "First text" and
// "Second text\nThird text".
func TestSplitTUonNewlines(t *testing.T) {
	parts := readString(t, "\\begin{document}\nFirst text\n"+"\n"+"Second text\nThird text")
	blocks := testutil.FilterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Equal(t, []string{"First text", "Second text\nThird text"}, texts)
}

// okapi: TEXFilterTest#testSplitTUonNewlines2
// A longer multi-paragraph snippet exercising blank-line splitting in
// the presence of inline commands (\textit, \textbf), escaped percents
// (9\%), %-comment lines, a \subsection, and \ref / \footnote commands.
// Upstream asserts blank lines split each prose paragraph into its own
// text unit, the \subsection argument "Inter-annotator Agreement" is its
// own unit, and the %-comment lines are not translatable. The native
// reader reproduces this segmentation: each blank-line-separated prose
// block is its own unit, escaped \% decodes to "%", comment lines route
// to Data, and \subsection{...} surfaces as a typed block.
//
// Divergence (honest): the native reader splits the final paragraph
// around the embedded \ref{...} (upstream keeps it as one unit) and does
// not decode the \=u macron when it appears inside a \textit inline
// command (accent decoding runs only on top-level body text). This test
// asserts the shared, robust observable behavior — paragraph count,
// percent-escape decoding, comment exclusion, subsection extraction —
// not those two divergent details.
func TestSplitTUonNewlines2(t *testing.T) {
	snippet := "\\begin{document}\nThe as \\textit{\\textbf{2008.} gada} text.\n" +
		"\n" +
		"Latvian also has a relatively free word order. In, it has a rather system (9\\% of errors), while impact.\n" +
		"\n" +
		"Errors for NMT (15\\%) than for SMT outputs (10\\%).\n" +
		"\n" +
		"%Finally, errors mostly capitalized politeness.\n" +
		"\n" +
		"\\subsection{Inter-annotator Agreement} \\label{agreement}\n" +
		"Although , 200 segments two annotators presents summary .\n"

	parts := readString(t, snippet)
	blocks := testutil.FilterBlocks(parts)
	texts := blockTexts(blocks)

	// Escaped percent decodes to a literal "%" in the body.
	assert.Contains(t, texts, "Latvian also has a relatively free word order. In, it has a rather system (9% of errors), while impact.")
	assert.Contains(t, texts, "Errors for NMT (15%) than for SMT outputs (10%).")
	// \subsection produces its own translatable unit.
	assert.Contains(t, texts, "Inter-annotator Agreement")
	// %-comment lines and \label keys are excluded from translatable text.
	for _, text := range texts {
		assert.NotContains(t, text, "Finally, errors")
		assert.NotContains(t, text, "agreement")
	}
}

// okapi: TEXFilterTest#testLatvianSymbolsEscaping
// Upstream snippet has \begin{document}, a %-comment, then three prose
// paragraphs of pre-composed Latvian Unicode letters (āčēģīķļņšūž etc.)
// separated by blank lines. Upstream's assertions round-trip these
// through TEXEncoder.toNative (encoding back to \={a}\v{c}... escapes);
// the extraction-side contract this verifies is that the three Unicode
// paragraphs are extracted verbatim as three separate translatable text
// units, with the comment excluded. The native reader emits the comment
// as Data and the three Latvian paragraphs as three blocks unchanged.
func TestLatvianSymbolsEscaping(t *testing.T) {
	snippet := "\\begin{document}\n%Šis ir koments - ar to nekas nav jādara\n" +
		"āčēģīķļņšūž\n\n" +
		"ĀČĒĢĪĶĻŅŠŪŽ\n\n" +
		"Šī Līnija pĀrbaudā latviešu simbolu attēlošanu."

	parts := readString(t, snippet)
	blocks := testutil.FilterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Equal(t, []string{
		"āčēģīķļņšūž",
		"ĀČĒĢĪĶĻŅŠŪŽ",
		"Šī Līnija pĀrbaudā latviešu simbolu attēlošanu.",
	}, texts)
}

// readTexFile reads a testdata .tex file through the native reader and
// returns the parts.
func readTexFile(t *testing.T, file string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	f, err := os.Open(file)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	reader := tex.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromReader(f, file, model.LocaleEnglish)))
	t.Cleanup(func() { reader.Close() })
	return testutil.CollectParts(t, reader.Read(ctx))
}

// okapi: TEXFilterTest#testDemoFile
// Upstream reads Test01.tex (the upstream fixture, copied verbatim to
// testdata/) and asserts the translatable text units include "Installing"
// (from \title{Installing \LaTeX}), "Jason, Gross" (from \author), and
// "translated" (from a body \title{translated}). The native reader
// extracts the \title and \author header commands as typed blocks plus
// the body \title — verifying the same header-and-body text-unit
// contract. (Native \title text is "Installing " with the trailing
// inline \LaTeX modeled as a placeholder code.)
func TestDemoFile(t *testing.T) {
	parts := readTexFile(t, "testdata/Test01.tex")
	texts := blockTexts(testutil.FilterBlocks(parts))

	assert.Contains(t, texts, "Installing ")
	assert.Contains(t, texts, "Jason, Gross")
	assert.Contains(t, texts, "translated")
}

// okapi: TEXFilterTest#testDemoFileWin
// Identical assertions to testDemoFile but against Test03.tex, the
// Windows-line-ending counterpart fixture. In the pinned upstream tree
// Test01.tex and Test03.tex are byte-identical (both LF), so the native
// reader yields the same header/body text units; this guards that file
// loading is line-ending agnostic for this fixture.
func TestDemoFileWin(t *testing.T) {
	parts := readTexFile(t, "testdata/Test03.tex")
	texts := blockTexts(testutil.FilterBlocks(parts))

	assert.Contains(t, texts, "Installing ")
	assert.Contains(t, texts, "Jason, Gross")
	assert.Contains(t, texts, "translated")
}

// okapi: TEXFilterTest#testDemoFile2
// Upstream reads Test02.tex (Russian fixture, copied verbatim to
// testdata/) and asserts body text units "Предисловие" (a \section
// argument) and "Кириллические символы также могут быть использованы в
// математическом режиме." (a body paragraph). The native reader extracts
// the \section headings and Cyrillic body paragraphs as translatable
// blocks while routing the preamble packages, \tableofcontents, the
// abstract wrapper, and the \begin{equation} block to non-translatable
// Data — matching the upstream extraction.
func TestDemoFile2(t *testing.T) {
	parts := readTexFile(t, "testdata/Test02.tex")
	texts := blockTexts(testutil.FilterBlocks(parts))

	assert.Contains(t, texts, "Предисловие")
	assert.Contains(t, texts, "Кириллические символы также могут быть использованы в математическом режиме.")

	// Preamble package commands stay non-translatable.
	for _, text := range texts {
		assert.NotContains(t, text, "usepackage")
		assert.NotContains(t, text, "tableofcontents")
	}
}

// TestSectionVariants verifies which `\section`-family commands
// produce translatable text units. Mirrors Okapi TEXFilter's
// `oneArgParaText` list — `\section`, `\subsection`, `\chapter` are
// included; `\subsubsection`, `\paragraph`, `\subparagraph`, `\part`
// are intentionally excluded (treated as unknown commands so their
// `{...}` arguments stay non-translatable). Keeps native byte-equal
// with the okapi reference for the upstream `sample.tex` fixture
// where mid-paragraph `\subsubsection{Typefaces and Sizes:}` must NOT
// emit "Typefaces and Sizes:" as translatable text.
func TestSectionVariants(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
\section{Section}
\subsection{Subsection}
\subsubsection{Subsubsection}
\chapter{Chapter}
\end{document}`)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Section")
	assert.Contains(t, texts, "Subsection")
	assert.Contains(t, texts, "Chapter")
	assert.NotContains(t, texts, "Subsubsection",
		"\\subsubsection content must not be translatable per okapi parity")
}

// --- Additional tests ---

// okapi: TEXFilterTest#testDefaultInfo
// Upstream asserts the filter exposes non-null parameters, a non-null
// name, and a non-empty configuration list. The native reader's
// equivalent surface is Name/DisplayName/Signature plus a non-nil
// Config; this verifies the reader advertises its identity and the
// tex MIME type / .tex extension (the native analog of the okapi
// FilterConfiguration list).
func TestNameAndMimeType(t *testing.T) {
	reader := tex.NewReader()
	assert.Equal(t, "tex", reader.Name())
	assert.Equal(t, "TeX/LaTeX", reader.DisplayName())

	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/x-tex")
	assert.Contains(t, sig.Extensions, ".tex")

	require.NotNil(t, reader.Config())
}

func TestEmptyInput(t *testing.T) {
	parts := readString(t, "")
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

func TestNilDocument(t *testing.T) {
	reader := tex.NewReader()
	err := reader.Open(t.Context(), nil)
	require.Error(t, err)
}

// okapi: TEXFilterTest#testStartDocument
// Upstream's FilterTestDriver.testStartDocument verifies the filter
// opens a document and emits a well-formed START_DOCUMENT event. The
// native equivalent is the leading PartLayerStart carrying a Layer with
// the tex format/MIME/locale set — this asserts the document opens and
// the start-of-document resource is correctly populated.
func TestLayerStructure(t *testing.T) {
	parts := readString(t, `\begin{document}
Hello
\end{document}`)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "tex", layer.Format)
	assert.Equal(t, "application/x-tex", layer.MimeType)
	assert.Equal(t, model.LocaleEnglish, layer.Locale)
}

func TestBlockIDsUnique(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
\section{First}
Paragraph one.

Paragraph two.
\end{document}`)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID)
		assert.False(t, ids[b.ID], "block IDs should be unique: %s", b.ID)
		ids[b.ID] = true
	}
}

func TestConfig(t *testing.T) {
	reader := tex.NewReader()
	cfg := reader.Config()
	assert.Equal(t, "tex", cfg.FormatName())
	require.NoError(t, cfg.Validate())

	// Unknown parameter should error
	err := cfg.ApplyMap(map[string]any{"unknownParam": "value"})
	require.Error(t, err)
}

func TestCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Create large input
	var sb strings.Builder
	sb.WriteString(`\begin{document}` + "\n")
	for range 1000 {
		sb.WriteString("Paragraph text for cancellation testing.\n\n")
	}
	sb.WriteString(`\end{document}`)

	reader := tex.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sb.String(), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)
	count := 0
	for range ch {
		count++
		if count >= 5 {
			cancel()
			break
		}
	}
	for range ch {
	}
	assert.GreaterOrEqual(t, count, 5)
}

func TestSynchronization(t *testing.T) {
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			ctx := t.Context()
			reader := tex.NewReader()
			err := reader.Open(ctx, testutil.RawDocFromString(
				`\begin{document}`+"\n"+"Hello\n\nWorld\n"+`\end{document}`,
				model.LocaleEnglish))
			if err != nil {
				t.Errorf("Open failed: %v", err)
				return
			}
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()
			blocks := testutil.FilterBlocks(parts)
			if len(blocks) < 2 {
				t.Errorf("expected at least 2 blocks, got %d", len(blocks))
			}
		})
	}
	wg.Wait()
}

func TestEscapedSpecialChars(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
Price is \$10 and 5\% off.
\end{document}`)

	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "$10")
	assert.Contains(t, text, "5% off")
}

func TestTildeNonBreakingSpace(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
Word~one is here.
\end{document}`)

	// The tilde should be modeled as an inline placeholder so it
	// round-trips verbatim and survives pseudo-translation. okapi's
	// TEXFilter routes TILDE tokens to addDocumentPart, preserving the
	// literal `~` byte. The translatable text stream excludes the
	// tilde, so Block.SourceText (TextRun-only) is "Wordone is here.".
	require.NotEmpty(t, blocks)
	runs := blocks[0].SourceRuns()
	var hasTilde bool
	for _, r := range runs {
		if r.Ph != nil && r.Ph.Data == "~" {
			hasTilde = true
			break
		}
	}
	assert.True(t, hasTilde, "expected an inline ~ placeholder, got runs=%+v", runs)
}

func TestMultipleComments(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
% First comment
Text between comments.
% Second comment
More text.
\end{document}`)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text between comments.")
	assert.Contains(t, texts, "More text.")
	for _, text := range texts {
		assert.NotContains(t, text, "First comment")
		assert.NotContains(t, text, "Second comment")
	}
}

func TestDateInHeader(t *testing.T) {
	// \date in the preamble is non-translatable (matches Okapi
	// TEXFilter — date strings are usually programmatically
	// formatted and not meaningful translation targets). \title
	// remains translatable.
	blocks := readBlocks(t, `\documentclass{article}
\title{Test}
\date{January 2024}
\begin{document}
\end{document}`)

	texts := blockTexts(blocks)
	for _, text := range texts {
		assert.NotContains(t, text, "January 2024",
			"\\date in preamble should not be translatable")
	}
	assert.Contains(t, texts, "Test")
}

func TestSectionWithOptionalArg(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
\section[Short]{Long Section Title}
Body text.
\end{document}`)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Long Section Title")
}

func TestDocumentWithoutPreamble(t *testing.T) {
	// No \documentclass — entire content is body
	blocks := readBlocks(t, `\begin{document}
Simple text.
\end{document}`)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Simple text")
}

func TestPlainTextOnly(t *testing.T) {
	// No TeX commands at all
	blocks := readBlocks(t, "Just plain text.")
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Just plain text.", blocks[0].SourceText())
}

func TestStarredSection(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
\section*{Unnumbered Section}
Text content.
\end{document}`)

	texts := blockTexts(blocks)
	// section* is a starred command variant
	// Check that the content is extracted one way or another
	found := false
	for _, text := range texts {
		if strings.Contains(text, "Unnumbered Section") || strings.Contains(text, "Text content") {
			found = true
		}
	}
	assert.True(t, found, "should extract some content from starred section")
}

// ---- Roundtrip tests ----

func roundtrip(t *testing.T, input string) string {
	t.Helper()
	return roundtripWithConfig(t, input, func(*tex.Config) {})
}

// roundtripWithConfig reads then writes (non-skeleton) with a custom config.
func roundtripWithConfig(t *testing.T, input string, configure func(*tex.Config)) string {
	t.Helper()
	ctx := t.Context()

	reader := tex.NewReader()
	configure(reader.TexConfig())
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := tex.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	err = writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, err)
	writer.Close()

	return buf.String()
}

// neokapi-only: native reader→writer roundtrip smoke (no okapi unit
// @Test analog; RoundTripTEXIT is an integration class absent from the
// pinned surefire suite). Byte-exact writer reproduction is verified by
// the TEXWriterTest mappings (TestWriteComments/BadTable/Hierarchy).
func TestRoundTrip_Simple(t *testing.T) {
	input := `\begin{document}
Hello world.
\end{document}`

	output := roundtrip(t, input)
	assert.Contains(t, output, "Hello world")
}

// neokapi-only: native roundtrip preserves sections + paragraphs.
func TestRoundTrip_Sections(t *testing.T) {
	input := `\begin{document}
\section{Introduction}
First paragraph.

Second paragraph.
\end{document}`

	output := roundtrip(t, input)
	assert.Contains(t, output, "Introduction")
	assert.Contains(t, output, "First paragraph")
	assert.Contains(t, output, "Second paragraph")
}

// neokapi-only: native roundtrip preserves an opaque equation block.
// Uses the Okapi-faithful config (surfacing off) so the non-skeleton writer
// reconstructs the equation environment from its opaque Data part. The
// byte-exact skeleton round-trip with surfacing ON is covered by
// TestEquationEnvironmentAsContent.
func TestRoundTrip_Math(t *testing.T) {
	input := `\begin{document}
\begin{equation}
a^2 + b^2 = c^2
\end{equation}
\end{document}`

	output := roundtripWithConfig(t, input, disableNonTranslatableContent)
	assert.Contains(t, output, "equation")
	assert.Contains(t, output, "a^2 + b^2 = c^2")
}

// neokapi-only: native roundtrip preserves comments.
func TestRoundTrip_Comment(t *testing.T) {
	input := `\begin{document}
% This is a comment
Hello.
\end{document}`

	output := roundtrip(t, input)
	assert.Contains(t, output, "comment")
	assert.Contains(t, output, "Hello")
}

// neokapi-only: native roundtrip preserves preamble \title + body.
func TestRoundTrip_Header(t *testing.T) {
	input := `\documentclass{article}
\title{My Title}
\begin{document}
Body.
\end{document}`

	output := roundtrip(t, input)
	assert.Contains(t, output, "My Title")
	assert.Contains(t, output, "Body")
}

// okapi: RoundTripTexIT#texFiles — native extract→write over the .tex fixture corpus, asserting extracted block content survives; Okapi's texFiles does extract→merge→compare-events over a .tex corpus.
// okapi-skip: RoundTripTexIT#texSerializedFiles — Okapi serialized-skeleton variant; native uses its own skeleton store, not Okapi's serialized event/skeleton format.
func TestRoundTrip_Files(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"simple", "testdata/simple.tex"},
		{"math", "testdata/math.tex"},
		{"complex", "testdata/complex.tex"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original, err := os.ReadFile(tt.file)
			require.NoError(t, err)

			ctx := t.Context()
			f, err := os.Open(tt.file)
			require.NoError(t, err)

			reader := tex.NewReader()
			err = reader.Open(ctx, testutil.RawDocFromReader(f, tt.file, model.LocaleEnglish))
			require.NoError(t, err)
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()

			// Verify we got some blocks
			blocks := testutil.FilterBlocks(parts)
			require.NotEmpty(t, blocks, "file %s should produce blocks", tt.file)

			// Write and verify output contains key content
			var buf bytes.Buffer
			writer := tex.NewWriter()
			err = writer.SetOutputWriter(&buf)
			require.NoError(t, err)
			writer.SetLocale(model.LocaleEnglish)
			err = writer.Write(ctx, testutil.PartsToChannel(parts))
			require.NoError(t, err)
			writer.Close()

			output := buf.String()
			require.NotEmpty(t, output)

			// Simple content preservation check
			_ = original
			for _, block := range blocks {
				text := block.SourceText()
				if len(text) > 5 {
					assert.Contains(t, output, text,
						"output should contain block text: %s", text)
				}
			}
		})
	}
}

// neokapi-only: native writer injects French targets into the output.
func TestRoundTrip_TargetLocale(t *testing.T) {
	ctx := t.Context()
	input := `\begin{document}
\section{Introduction}
Hello world.
\end{document}`

	reader := tex.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set French targets
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			switch block.SourceText() {
			case "Introduction":
				block.SetTargetText(model.LocaleFrench, "Présentation")
			case "Hello world.":
				block.SetTargetText(model.LocaleFrench, "Bonjour le monde.")
			}
		}
	}

	var buf bytes.Buffer
	writer := tex.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)
	err = writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Présentation")
	assert.Contains(t, output, "Bonjour le monde")
}

// okapi: TexXliffCompareIT#texXliffCompareFiles — native double-extraction (roundtrip of a roundtrip) verifies extracted content is stable; Okapi's texXliffCompareFiles extracts to XLIFF and compares against a gold XLIFF corpus.
// neokapi note: native double-extraction (roundtrip of a roundtrip) is
// stable for section + paragraph content.
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	input := `\begin{document}
\section{Heading}
First paragraph.

Second paragraph.
\end{document}`

	// First roundtrip
	output1 := roundtrip(t, input)
	require.NotEmpty(t, output1)

	// Second roundtrip on the output
	output2 := roundtrip(t, output1)
	require.NotEmpty(t, output2)

	// Both outputs should contain the same content
	assert.Contains(t, output2, "Heading")
	assert.Contains(t, output2, "First paragraph")
	assert.Contains(t, output2, "Second paragraph")
}

// ---- Writer tests ----

func TestWriter_NameAndClose(t *testing.T) {
	writer := tex.NewWriter()
	assert.Equal(t, "tex", writer.Name())
	require.NoError(t, writer.Close())
}

func TestWriter_EmptyChannel(t *testing.T) {
	ctx := t.Context()
	writer := tex.NewWriter()
	var buf bytes.Buffer
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := make(chan *model.Part)
	close(ch)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
}

// skeletonRoundtrip reads input, then writes it back through the
// SkeletonStore path with no translation set. This is the native
// equivalent of Okapi's IFilterWriter byte-exact reproduction: the
// reader streams the non-translatable bytes to the skeleton store and
// the writer splices the (untranslated) block source back in, yielding
// the original document verbatim.
func skeletonRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := tex.NewReader()
	writer := tex.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// okapi: TEXWriterTest#writeComments
// Upstream parses a snippet of leading comments, blank lines, and body
// prose, then writes every event back through the filter writer and
// asserts the output equals the input byte-for-byte. The native
// SkeletonStore path is the equivalent byte-exact writer: comments and
// blank lines route to skeleton text, body prose round-trips via block
// refs, so the reconstructed document matches the input exactly.
func TestWriteComments(t *testing.T) {
	snippet := "%\\title{Tilde's ijcnlp 2017 submission}\n" + "\n" + "% File ijcnlp2017.tex\n" +
		"%3\nLAlala Kautkads saturs\n\n\n" + "% nokomentēju, jo nav publicēts\nVel saturs\n" + "% nokomentēju, jo nav publicēts"

	assert.Equal(t, snippet, skeletonRoundtrip(t, snippet))
}

// okapi: TEXWriterTest#writeBadTable
// Upstream feeds display math ($$..$$), a \subsection with a trailing
// \label, a long body paragraph, a full \begin{table}...\end{table}
// float, and a block of commented-out figure lines, then asserts the
// writer reproduces the input verbatim. The native SkeletonStore writer
// reproduces the same bytes: the table and math route to opaque skeleton
// text, the subsection and paragraph round-trip via block refs.
func TestWriteBadTable(t *testing.T) {
	snippet := "$$3*4^6$$\n\\subsection{MT System Evaluation} \\label{mt_eval}\n" +
		"SMT and NMT systems were evaluated on a held-out set of 1000 randomly selected sentence pairs. The automatic evaluation results are given in Table~\\ref{mt-eval-table}. The results show that the SMT system achieves better results than the NMT system.\n" +
		"\\begin{table}[]\n" +
		"\\centering\n" +
		"\\begin{tabular}{|lrrr|}\n" +
		"\\hline\n" +
		"\\multicolumn{1}{|c}{\\textbf{System}} & \\multicolumn{1}{c}{\\textbf{BLEU}} & \\multicolumn{1}{c}{\\textbf{NIST}} & \\multicolumn{1}{c|}{\\textbf{ChrF2}} \\\\ \\hline\n" +
		"SMT & 46.57$\\pm$1.46 & 9.45$\\pm$0.18 & 0.7586 \\\\\n" +
		"NMT & 38.44$\\pm$1.62 & 8.63$\\pm$0.15 & 0.7065 \\\\ \\hline\n" +
		"\\end{tabular}\n" +
		"\\caption{Automatic evaluation results}\n" +
		"\\label{mt-eval-table}\n" +
		"\\end{table}\n" +
		"\n" +
		"% attēls nav labs, varbūt mest ārā?\n" +
		"%\\begin{figure*}[]\n" +
		"%\\begin{center}\n" +
		"%\\includegraphics[width=450px]{wOrder-fixed.jpg}\n" +
		"%\\end{center}\n" +
		"%\\caption{\\label{WO}Influence of word order on BLEU score}\n" +
		"%\\end{figure*}"

	assert.Equal(t, snippet, skeletonRoundtrip(t, snippet))
}

// okapi: TEXWriterTest#writeHierarchy
// Upstream feeds a full document hierarchy (\documentclass, several
// concatenated \usepackage, a \newcommand, \title, \begin{document},
// \maketitle, a long body paragraph with nested inline commands, and
// \end{document}), then asserts byte-exact writer output. The native
// SkeletonStore writer reproduces the input verbatim: header commands
// and \maketitle route to skeleton text while \title and the body
// paragraph round-trip via block refs.
func TestWriteHierarchy(t *testing.T) {
	snippet := "\\documentclass[11pt,letterpaper]{article}\n" +
		"\\usepackage{ijcnlp2017}\\usepackage{times}\\usepackage{latexsym}\\usepackage{multirow}\\usepackage{graphicx}\\usepackage{color,soul}\\usepackage{todonotes}\\usepackage{placeins}\n" +
		"\n" +
		"\n" +
		"\\newcommand\\BibTeX{B{\\sc ib}\\TeX}\n" +
		"\\title{NMT or SMT: Case Study of a Narrow-domain English-Latvian Post-editing Project by Inguna Skadiņas}\n" +
		"\\begin{document}\n" +
		"\n" +
		"\\maketitle\n" +
		"\n" +
		"\n" +
		"For inter-annotator agreement, we calculated free-marginal kappa in three different conditions (see Table \\ref{agreement-table}): \\emph{perfect \\textbf{match} analysis} (i.e., by taking the precise positions and (sub)categories of errors into account), error count analysis. The results show that when taking positions into account, there is just slight agreement between the annotators.\n" +
		"\n" +
		"\n" +
		"\\end{document}"

	assert.Equal(t, snippet, skeletonRoundtrip(t, snippet))
}
