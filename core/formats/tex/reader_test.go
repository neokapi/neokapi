package tex_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gokapi/gokapi/core/formats/tex"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: read a string and return all parts
func readString(t *testing.T, input string) []*model.Part {
	t.Helper()
	ctx := context.Background()
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
func TestMathMode(t *testing.T) {
	// Inline math $...$ should be excluded from text
	parts := readString(t, `\begin{document}
$E = mc^2$
\end{document}`)

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

// okapi: TEXFilterTest#testMathMode (display math $$)
func TestMathMode_DisplayDollar(t *testing.T) {
	parts := readString(t, `\begin{document}
$$a^2 + b^2 = c^2$$
\end{document}`)

	blocks := testutil.FilterBlocks(parts)
	for _, b := range blocks {
		assert.NotContains(t, b.SourceText(), "a^2")
	}
}

// okapi: TEXFilterTest#testMathMode (display math \[...\])
func TestMathMode_DisplayBracket(t *testing.T) {
	parts := readString(t, `\begin{document}
\[x = \frac{-b}{2a}\]
\end{document}`)

	blocks := testutil.FilterBlocks(parts)
	for _, b := range blocks {
		assert.NotContains(t, b.SourceText(), "frac")
	}
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

// okapi: TEXFilterTest#testRussianCyrillic
func TestRussianCyrillic(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
Привет мир!
\end{document}`)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Привет мир!")
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

// okapi: TEXFilterTest#testRunawayBraces
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

// okapi: TEXFilterTest#testCommandClassification
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

// okapi: TEXFilterTest#testCommandClassification (inline-text)
func TestCommandClassification_InlineText(t *testing.T) {
	// \textbf, \emph arguments contain translatable text
	blocks := readBlocks(t, `\begin{document}
This is \textbf{bold text} here.
\end{document}`)

	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "bold text")
}

// okapi: TEXFilterTest#testCommandClassification (paragraph-text)
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

// okapi: TEXFilterTest#testTableEnvironment
func TestTableEnvironment(t *testing.T) {
	// tabular environment content should be parsed
	blocks := readBlocks(t, `\begin{document}
\begin{tabular}{|c|c|}
Cell one & Cell two
\end{tabular}
\end{document}`)
	require.NotEmpty(t, blocks)
	// The content within the tabular environment should be extracted
	allText := ""
	for _, b := range blocks {
		allText += b.SourceText()
	}
	assert.Contains(t, allText, "Cell one")
}

// okapi: TEXFilterTest#testEquationEnvironment
func TestEquationEnvironment(t *testing.T) {
	parts := readString(t, `\begin{document}
\begin{equation}
a^2 + b^2 = c^2
\end{equation}
\end{document}`)

	blocks := testutil.FilterBlocks(parts)
	for _, b := range blocks {
		assert.NotContains(t, b.SourceText(), "a^2",
			"equation environment should not be translatable")
	}
}

// okapi: TEXFilterTest#testNestedCommands
func TestNestedCommands(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
This is \textbf{\emph{bold italic}} text.
\end{document}`)

	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "bold italic")
}

// okapi: TEXFilterTest#testScripts
func TestScripts(t *testing.T) {
	// Subscript/superscript in math mode — non-translatable
	parts := readString(t, `\begin{document}
$x^2$ and $x_i$
\end{document}`)
	blocks := testutil.FilterBlocks(parts)
	for _, b := range blocks {
		assert.NotContains(t, b.SourceText(), "x^2")
		assert.NotContains(t, b.SourceText(), "x_i")
	}
}

// okapi: TEXFilterTest#testFootnotes
func TestFootnotes(t *testing.T) {
	blocks := readBlocks(t, `\begin{document}
Main text\footnote{This is the footnote content.} continues.
\end{document}`)

	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "This is the footnote content.")
}

// okapi: TEXFilterTest#testSectionVariants
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
	assert.Contains(t, texts, "Subsubsection")
	assert.Contains(t, texts, "Chapter")
}

// --- Additional tests ---

func TestNameAndMimeType(t *testing.T) {
	reader := tex.NewReader()
	assert.Equal(t, "tex", reader.Name())
	assert.Equal(t, "TeX/LaTeX", reader.DisplayName())

	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/x-tex")
	assert.Contains(t, sig.Extensions, ".tex")
}

func TestEmptyInput(t *testing.T) {
	parts := readString(t, "")
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

func TestNilDocument(t *testing.T) {
	reader := tex.NewReader()
	err := reader.Open(context.Background(), nil)
	assert.Error(t, err)
}

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
	assert.NoError(t, cfg.Validate())

	// Unknown parameter should error
	err := cfg.ApplyMap(map[string]any{"unknownParam": "value"})
	assert.Error(t, err)
}

func TestCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create large input
	var sb strings.Builder
	sb.WriteString(`\begin{document}` + "\n")
	for i := 0; i < 1000; i++ {
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
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
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
		}()
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

	// The tilde should be converted to a space in text
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Word one")
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
	blocks := readBlocks(t, `\documentclass{article}
\title{Test}
\date{January 2024}
\begin{document}
\end{document}`)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "January 2024")
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
	ctx := context.Background()

	reader := tex.NewReader()
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

// okapi: RoundTripTEXIT#testSimple
func TestRoundTrip_Simple(t *testing.T) {
	input := `\begin{document}
Hello world.
\end{document}`

	output := roundtrip(t, input)
	assert.Contains(t, output, "Hello world")
}

// okapi: RoundTripTEXIT#testSections
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

// okapi: RoundTripTEXIT#testMath
func TestRoundTrip_Math(t *testing.T) {
	input := `\begin{document}
\begin{equation}
a^2 + b^2 = c^2
\end{equation}
\end{document}`

	output := roundtrip(t, input)
	assert.Contains(t, output, "equation")
	assert.Contains(t, output, "a^2 + b^2 = c^2")
}

// okapi: RoundTripTEXIT#testComment
func TestRoundTrip_Comment(t *testing.T) {
	input := `\begin{document}
% This is a comment
Hello.
\end{document}`

	output := roundtrip(t, input)
	assert.Contains(t, output, "comment")
	assert.Contains(t, output, "Hello")
}

// okapi: RoundTripTEXIT#testHeader
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

// okapi: RoundTripTEXIT#testFiles
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

			ctx := context.Background()
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

// okapi: RoundTripTEXIT#testTargetLocale
func TestRoundTrip_TargetLocale(t *testing.T) {
	ctx := context.Background()
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

// okapi: RoundTripTEXIT#testDoubleExtraction
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
	assert.NoError(t, writer.Close())
}

func TestWriter_EmptyChannel(t *testing.T) {
	ctx := context.Background()
	writer := tex.NewWriter()
	var buf bytes.Buffer
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := make(chan *model.Part)
	close(ch)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
}
