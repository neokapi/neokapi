package tex_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/tex"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := tex.NewReader()
	writer := tex.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

func TestSkeletonStore_ByteExact_SimpleParagraph(t *testing.T) {
	input := "Hello world"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple paragraph roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_SectionAndParagraph(t *testing.T) {
	input := `\section{Introduction}

This is the first paragraph.`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "section + paragraph roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_DocumentStructure(t *testing.T) {
	input := `\documentclass{article}
\begin{document}
Hello world
\end{document}`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "full document structure roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MathAndComments(t *testing.T) {
	input := `% This is a comment
Hello world
$E = mc^2$
More text`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "math and comments roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_NonTranslatableEnv(t *testing.T) {
	input := `Text before
\begin{equation}
x^2 + y^2 = z^2
\end{equation}
Text after`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "non-translatable environment roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_TitleAuthorDate(t *testing.T) {
	input := `\documentclass{article}
\title{My Title}
\author{John Doe}
\begin{document}
Hello`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "title/author in preamble roundtrip should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := `\section{Introduction}

Hello world`
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := tex.NewReader()
	writer := tex.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Introduction":
				b.SetTargetText(locale, "Pr\u00e9sentation")
			case "Hello world":
				b.SetTargetText(locale, "Bonjour le monde")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := `\section{Pr` + "\u00e9" + `sentation}

Bonjour le monde`
	assert.Equal(t, expected, buf.String())
}
