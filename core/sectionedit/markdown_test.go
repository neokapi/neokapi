package sectionedit

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func applyAdapterPatch(t *testing.T, source []byte, patches []Patch) []byte {
	t.Helper()
	require.Len(t, patches, 1)
	patch := patches[0]
	require.Empty(t, patch.Entry)
	require.Equal(t, string(source[patch.Start:patch.End]), patch.Before)
	result := append([]byte{}, source[:patch.Start]...)
	result = append(result, patch.Replacement...)
	result = append(result, source[patch.End:]...)
	return result
}

func TestMarkdownSectionPatchPreservesHeadingAndOutsideBytes(t *testing.T) {
	source := []byte("Preamble\r\n\r\n# Parent *title*\r\n\r\nOld text.\r\n\r\n## Nested\r\n\r\nOld child.\r\n\r\n# Next\r\n\r\nTail  \r\n")
	original := bytes.Clone(source)
	sections, err := inspectMarkdown(source)
	require.NoError(t, err)
	require.Len(t, sections, 3)
	assert.Contains(t, sections[0].Content, "## Nested\r\n")
	assert.NotContains(t, sections[1].Content, "# Next")
	patches, err := planMarkdown(source, 0, "New **emphasis** and [link](https://example.test).\n\n- One\n- Two\n\n## Replacement\n\n```go\nrun()\n```\n")
	require.NoError(t, err)
	assert.Equal(t, original, source, "planning must not mutate its input")
	result := applyAdapterPatch(t, source, patches)
	assert.True(t, bytes.HasPrefix(result, []byte("Preamble\r\n\r\n# Parent *title*\r\n")))
	assert.True(t, bytes.HasSuffix(result, []byte("# Next\r\n\r\nTail  \r\n")))
	after, err := inspectMarkdown(result)
	require.NoError(t, err)
	require.Len(t, after, 3)
	assert.Equal(t, "Replacement", after[1].Title)
	assert.Contains(t, after[0].Content, "```go\r\nrun()\r\n```")
}

func TestMarkdownSetextAndDuplicateTitlesUseSourceIndexes(t *testing.T) {
	source := []byte("Intro\n\nSame\n====\n\nFirst\n\nSame\n----\n\nNested\n\nSame\n====\n\nLast\n")
	sections, err := inspectMarkdown(source)
	require.NoError(t, err)
	require.Len(t, sections, 3)
	assert.Equal(t, []int{1, 2, 1}, []int{sections[0].Level, sections[1].Level, sections[2].Level})
	patches, err := planMarkdown(source, 1, "Changed.")
	require.NoError(t, err)
	result := applyAdapterPatch(t, source, patches)
	assert.Equal(t, "Intro\n\nSame\n====\n\nFirst\n\nSame\n----\n\nChanged.\n\nSame\n====\n\nLast\n", string(result))
}

func TestMarkdownIgnoresNonDocumentHeadings(t *testing.T) {
	source := []byte("# Real\n\n```md\n# In fence\n```\n\n    # Indented\n\n> # Quoted\n\n- # Listed\n\n## Child\n\nText.\n")
	sections, err := inspectMarkdown(source)
	require.NoError(t, err)
	require.Len(t, sections, 2)
	assert.Equal(t, "Real", sections[0].Title)
	assert.Equal(t, "Child", sections[1].Title)
	assert.Contains(t, sections[0].Content, "# In fence")
}

func TestMarkdownEmptyFinalHeadingAndInvalidInput(t *testing.T) {
	sections, err := inspectMarkdown([]byte("##"))
	require.NoError(t, err)
	require.Len(t, sections, 1)
	patches, err := planMarkdown([]byte("##"), 0, "New body.")
	require.NoError(t, err)
	assert.Equal(t, "##\n\nNew body.\n\n", string(applyAdapterPatch(t, []byte("##"), patches)))
	sections, err = inspectMarkdown([]byte("No headings here."))
	require.NoError(t, err)
	assert.Empty(t, sections)
	_, err = planMarkdown([]byte("No headings here."), 0, "New")
	require.Error(t, err)
	_, err = inspectMarkdown([]byte{0xff})
	require.Error(t, err)
	_, err = inspectMarkdown([]byte("# Title\n\x00"))
	require.Error(t, err)
}

func TestMarkdownSetextTitleBeginningWithHashIsNotATX(t *testing.T) {
	source := []byte("#Literal title\ncontinued line\n===\n\nOld.\n")
	sections, err := inspectMarkdown(source)
	require.NoError(t, err)
	require.Len(t, sections, 1)
	assert.Equal(t, "#Literal title continued line", sections[0].Title)
	patches, err := planMarkdown(source, 0, "New.")
	require.NoError(t, err)
	assert.Equal(t, "#Literal title\ncontinued line\n===\n\nNew.\n\n", string(applyAdapterPatch(t, source, patches)))
}

func TestMarkdownBOMKeepsFirstHeadingAndAbsolutePatchOffsets(t *testing.T) {
	for _, heading := range []string{"# Café\r\n", "Café\r\n===\r\n"} {
		source := []byte("\ufeff" + heading + "\r\nOld.\r\n\r\n# Next\r\n\r\nTail.\r\n")
		sections, err := inspectMarkdown(source)
		require.NoError(t, err)
		require.Len(t, sections, 2)
		assert.Equal(t, "Café", sections[0].Title)
		assert.Equal(t, 3, sections[0].headingStart)
		patches, err := planMarkdown(source, 0, "New.")
		require.NoError(t, err)
		result := applyAdapterPatch(t, source, patches)
		assert.Equal(t, "\ufeff"+heading+"\r\nNew.\r\n\r\n# Next\r\n\r\nTail.\r\n", string(result))
	}
}

func TestMarkdownCannotDeleteReferenceDefinitionsUsedOutsideSection(t *testing.T) {
	for _, prefix := range []string{"", "\ufeff"} {
		source := []byte(prefix + "# First\n\nRead [help][shared].\n\n# Last\n\nOld.\n\n[shared]: https://example.test\n")
		_, err := planMarkdown(source, 1, "New body.")
		require.ErrorContains(t, err, "reference definitions")
		// A reference definition elsewhere does not prohibit editing this body.
		_, err = planMarkdown(source, 0, "New introduction.")
		require.NoError(t, err)
	}
}

func TestMarkdownCodeThatLooksLikeReferenceDefinitionRemainsEditable(t *testing.T) {
	source := []byte("# Examples\n\n```md\n[shared]: https://example.test\n```\n\n    [another]: https://example.test\n")
	_, err := planMarkdown(source, 0, "New examples.")
	require.NoError(t, err)
}
