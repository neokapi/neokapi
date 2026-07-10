package brandscan

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssembleCorpusOrderingAndTagging(t *testing.T) {
	sources := []Source{
		{Kind: SourceRepo, Label: "README.md", Text: "Repo readme voice."},
		{Kind: SourceFile, Label: "voice.docx", Text: "Uploaded style guide."},
		{Kind: SourceURL, Label: "https://acme.com", Text: "Website copy."},
		{Kind: SourcePaste, Label: "Founder notes", Text: "Pasted founder notes."},
	}
	corpus := AssembleCorpus(sources, 0)

	// Deterministic order: paste, url, file, repo.
	require.Len(t, corpus.Sources, 4)
	assert.Equal(t, SourcePaste, corpus.Sources[0].Kind)
	assert.Equal(t, SourceURL, corpus.Sources[1].Kind)
	assert.Equal(t, SourceFile, corpus.Sources[2].Kind)
	assert.Equal(t, SourceRepo, corpus.Sources[3].Kind)

	// Every source is tagged with its label and kind.
	assert.Contains(t, corpus.Text, "=== source: Founder notes (paste) ===")
	assert.Contains(t, corpus.Text, "=== source: https://acme.com (url) ===")
	assert.Contains(t, corpus.Text, "=== source: voice.docx (file) ===")
	assert.Contains(t, corpus.Text, "=== source: README.md (repo) ===")

	// Text follows its own tag.
	pasteIdx := strings.Index(corpus.Text, "=== source: Founder notes (paste) ===")
	urlIdx := strings.Index(corpus.Text, "=== source: https://acme.com (url) ===")
	notesIdx := strings.Index(corpus.Text, "Pasted founder notes.")
	require.GreaterOrEqual(t, pasteIdx, 0)
	assert.Greater(t, notesIdx, pasteIdx)
	assert.Greater(t, urlIdx, notesIdx)

	assert.False(t, corpus.Truncated)
	assert.Equal(t, utf8.RuneCountInString(corpus.Text), corpus.RuneCount)
}

func TestAssembleCorpusOrderIsInputOrderIndependent(t *testing.T) {
	a := []Source{
		{Kind: SourceURL, Label: "b.example", Text: "B text."},
		{Kind: SourceURL, Label: "a.example", Text: "A text."},
		{Kind: SourcePaste, Label: "notes", Text: "Notes."},
	}
	b := []Source{a[2], a[0], a[1]}
	assert.Equal(t, AssembleCorpus(a, 0).Text, AssembleCorpus(b, 0).Text)
}

func TestAssembleCorpusDropsEmptySources(t *testing.T) {
	corpus := AssembleCorpus([]Source{
		{Kind: SourcePaste, Label: "empty", Text: "   \n\t"},
		{Kind: SourceURL, Label: "https://acme.com", Text: "Real content."},
	}, 0)
	require.Len(t, corpus.Sources, 1)
	assert.Equal(t, "https://acme.com", corpus.Sources[0].Label)
	assert.NotContains(t, corpus.Text, "empty")
}

func TestAssembleCorpusEmptyLabelFallsBackToKind(t *testing.T) {
	corpus := AssembleCorpus([]Source{{Kind: SourcePaste, Text: "Some pasted text."}}, 0)
	assert.Contains(t, corpus.Text, "=== source: paste (paste) ===")
}

func TestAssembleCorpusTruncatesOnRuneBoundary(t *testing.T) {
	// Multibyte text: each 'æ', 'ø', 'å' is 2 bytes but 1 rune.
	long := strings.Repeat("æøå ", 100) // 400 runes
	sources := []Source{{Kind: SourcePaste, Label: "nb", Text: long}}

	header := "=== source: nb (paste) ===" // 26 runes
	maxRunes := utf8.RuneCountInString(header) + 1 + 50

	corpus := AssembleCorpus(sources, maxRunes)
	assert.True(t, corpus.Truncated)
	assert.True(t, utf8.ValidString(corpus.Text), "truncation must never split a rune")
	assert.Equal(t, maxRunes, corpus.RuneCount)
	assert.Equal(t, utf8.RuneCountInString(corpus.Text), corpus.RuneCount)
	require.Len(t, corpus.Sources, 1)
	assert.Equal(t, 50, corpus.Sources[0].Runes)
	assert.True(t, strings.HasPrefix(corpus.Text, header+"\n"))
}

func TestAssembleCorpusDropsSourcesPastBudget(t *testing.T) {
	sources := []Source{
		{Kind: SourcePaste, Label: "first", Text: strings.Repeat("x", 100)},
		{Kind: SourceURL, Label: "second", Text: "Should be dropped."},
	}
	// Budget covers the first source's header + some text only.
	corpus := AssembleCorpus(sources, 60)
	assert.True(t, corpus.Truncated)
	require.Len(t, corpus.Sources, 1)
	assert.Equal(t, "first", corpus.Sources[0].Label)
	assert.NotContains(t, corpus.Text, "second")
	assert.NotContains(t, corpus.Text, "Should be dropped.")
	assert.LessOrEqual(t, corpus.RuneCount, 60)
}

func TestAssembleCorpusNoCapWhenMaxRunesZeroOrNegative(t *testing.T) {
	sources := []Source{{Kind: SourcePaste, Label: "notes", Text: strings.Repeat("y", 5000)}}
	for _, maxRunes := range []int{0, -1} {
		corpus := AssembleCorpus(sources, maxRunes)
		assert.False(t, corpus.Truncated)
		assert.Contains(t, corpus.Text, strings.Repeat("y", 5000))
	}
}

func TestAssembleCorpusEmptyInput(t *testing.T) {
	corpus := AssembleCorpus(nil, 100)
	assert.Empty(t, corpus.Text)
	assert.Zero(t, corpus.RuneCount)
	assert.False(t, corpus.Truncated)
	assert.Empty(t, corpus.Sources)
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		s    string
		max  int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 3, "hel"},
		{"hello", 0, ""},
		{"hello", -1, ""},
		{"æøå", 2, "æø"},
		{"日本語テキスト", 3, "日本語"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.s+"/"+string(rune('0'+max(tt.max, 0))), func(t *testing.T) {
			got := truncateRunes(tt.s, tt.max)
			assert.Equal(t, tt.want, got)
			assert.True(t, utf8.ValidString(got))
		})
	}
}
