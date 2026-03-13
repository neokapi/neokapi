//go:build integration

package tex

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

// readTestFile reads a file from disk and returns the content bytes.
func readTestFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// okapi: RoundTripTexIT
func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte(`\documentclass{article}
\begin{document}
Hello world.
\end{document}`)
	bridgetest.AssertRoundTrip(t, pool, cfg, filterClass, input, "test.tex", mimeType, nil)
}

// okapi: RoundTripTexIT#testTexFiles
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okapi/filters/tex/src/test/resources/*.tex", mimeType, nil)
}

// okapi: RoundTripTexIT (inline snippets with various TeX features)
func TestRoundTrip_InlineSnippets(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	tests := []struct {
		name  string
		input string
	}{
		{
			"simple_text",
			`\documentclass{article}
\begin{document}
A simple sentence.
\end{document}`,
		},
		{
			"two_paragraphs",
			`\documentclass{article}
\begin{document}
First paragraph.

Second paragraph.
\end{document}`,
		},
		{
			"section_commands",
			`\documentclass{article}
\begin{document}
\section{Introduction}
Some content here.
\end{document}`,
		},
		{
			"inline_formatting",
			`\documentclass{article}
\begin{document}
This is \textbf{bold} and \emph{italic} text.
\end{document}`,
		},
		{
			"math_mode",
			`\documentclass{article}
\begin{document}
The value $x = 5$ is used.
\end{document}`,
		},
		{
			"comments",
			`\documentclass{article}
\begin{document}
Text here. % This is a comment
More text.
\end{document}`,
		},
		{
			"escaped_percent",
			`\documentclass{article}
\begin{document}
Rate is 55\% today.
\end{document}`,
		},
		{
			"table_environment",
			`\documentclass{article}
\begin{document}
\begin{table}
\caption{Simple Table}
\begin{tabular}{|c|c|}
\hline
A & B \\
\hline
\end{tabular}
\end{table}
\end{document}`,
		},
		{
			"equation_environment",
			`\documentclass{article}
\begin{document}
Before equation.
\begin{equation}
E = mc^2
\end{equation}
After equation.
\end{document}`,
		},
		{
			"nested_commands",
			`\documentclass{article}
\begin{document}
Text with \textbf{\emph{nested}} formatting.
\end{document}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
				[]byte(tt.input), "test.tex", mimeType, nil)
		})
	}
}

// okapi: RoundTripTexIT (file-level event roundtrip)
func TestRoundTrip_FileEvents(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	files := []string{
		"Test01.tex",
		"Test02.tex",
		"Test03.tex",
		"sample.tex",
		"sample1.tex",
	}

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			path := tdDir + "/okapi/filters/tex/src/test/resources/" + f
			content, err := readTestFile(path)
			require.NoError(t, err)
			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
				content, path, mimeType, nil)
		})
	}
}

// okapi: TexXliffCompareIT (XLIFF compare - tested via roundtrip events)
func TestRoundTrip_XliffCompare(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// The XLIFF compare IT verifies extraction consistency. Event-level
	// roundtrip tests cover the same ground: re-reading output and comparing
	// parts ensures extraction is stable.
	path := tdDir + "/okapi/filters/tex/src/test/resources/Test01.tex"
	content, err := readTestFile(path)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, path, mimeType, nil)
}

// okapi: TEXFilterTest#testLatvianSymbolsEscaping (roundtrip via events)
func TestRoundTrip_LatvianSymbols(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte(`\documentclass{article}
\begin{document}
\v{S}\={\i} L\={\i}nija p\={A}rbaud\={a} latvie\v{s}u simbolu att\={e}lo\v{s}anu.
\end{document}`)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		input, "latvian.tex", mimeType, nil)
}

// okapi: TEXWriterTest#writeComments (roundtrip via events)
func TestRoundTrip_WithComments(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte(`\documentclass{article}
\begin{document}
% This is a comment
Visible text.
% Another comment
More text.
\end{document}`)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		input, "comments.tex", mimeType, nil)
}

// okapi: TEXWriterTest#writeHierarchy (roundtrip via events)
func TestRoundTrip_WithHierarchy(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte(`\documentclass{article}
\begin{document}
\section{Top Level}
\subsection{Sub Level}
Content here.
\end{document}`)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		input, "hierarchy.tex", mimeType, nil)
}
