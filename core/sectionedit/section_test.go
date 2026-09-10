package sectionedit

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSectionPlansBindNativeBlocksAcrossFormats(t *testing.T) {
	docx := docxTestArchive(t, docxTestDocument(
		docxTestHeading("Share", "PreviewSection")+
			`<w:p><w:r><w:t>First old paragraph.</w:t></w:r></w:p>`+
			`<w:p><w:r><w:t>Second old paragraph.</w:t></w:r></w:p>`+
			docxTestHeading("Keep", "PreviewSection")+
			`<w:p><w:r><w:t>Keep unchanged.</w:t></w:r></w:p>`,
	))
	cases := map[string][]byte{
		"markdown": []byte("## Share\n\nFirst old paragraph.\n\nSecond old paragraph.\n\n## Keep\n\nKeep unchanged.\n"),
		"html": []byte(`<main id="guide"><h2>Share</h2><p>First old paragraph.</p><p>Second old paragraph.</p>` +
			`<h2>Keep</h2><p>Keep unchanged.</p></main><footer><p>Outside footer.</p></footer>`),
		"docx": docx,
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			original := bytes.Clone(source)
			doc, err := Inspect(t.Context(), name, source)
			require.NoError(t, err)
			require.Len(t, doc.Sections, 2)
			section := doc.Sections[0]
			assert.Equal(t, section.Range.Heading.ID, section.ID)
			require.Len(t, section.Range.Body, 2)
			for _, ref := range section.Range.Body {
				require.NotEmpty(t, ref.ID)
				require.NotEmpty(t, ref.ContentHash)
				require.NotNil(t, ref.Source)
			}
			require.NotNil(t, section.Range.EndBefore)
			assert.Equal(t, doc.Sections[1].ID, section.Range.EndBefore.ID)
			edit := Edit{
				ID: section.ID, Snapshot: doc.Snapshot,
				Text: "A **clear** introduction.\n\n### Share the snapshot\n\nA new step.\n\nAnother step.\n",
			}
			prepared, err := Prepare(t.Context(), name, source, edit)
			require.NoError(t, err)
			assert.Equal(t, original, source, "planning does not mutate source")
			require.Equal(t, section.Range, *prepared.Plan.Range)
			written, err := format.ApplyOffsetPatches(source, prepared.Plan)
			require.NoError(t, err)
			assert.Equal(t, prepared.Data, written, "preview is the actual writer output")
			after, err := Inspect(t.Context(), name, written)
			require.NoError(t, err)
			require.Len(t, after.Sections, 3, "one operation inserts a nested heading and several blocks")
			assert.Equal(t, "Share", after.Sections[0].Title)
			assert.Equal(t, doc.Sections[1].Content, after.Sections[2].Content)
			_, err = Prepare(t.Context(), name, written, edit)
			require.ErrorIs(t, err, ErrStale, "stale snapshot cannot target shifted IDs")
			edit.Snapshot, edit.ID = after.Snapshot, after.Sections[0].ID
			edit.Text = "A revised introduction.\n\nA revised next step.\n"
			second, err := Prepare(t.Context(), name, written, edit)
			require.NoError(t, err)
			assert.Contains(t, second.After.Content, "revised next step")
			if name == "docx" {
				assertDOCXOtherPartsIdentical(t, source, second.Data)
			}
		})
	}
}

func TestSectionGuardRejectsAmbiguousOrUnsupportedRequests(t *testing.T) {
	source := []byte("## Repeat\n\nFirst.\n\n## Repeat\n\nSecond.\n")
	doc, err := Inspect(t.Context(), "markdown", source)
	require.NoError(t, err)
	require.NotEqual(t, doc.Sections[0].ID, doc.Sections[1].ID)
	for _, fragment := range []string{"", "## Replaces the heading", "<script>bad()</script>", "![image](x.png)", "```go\nrun()", "[shared]: https://example.test"} {
		_, err := Prepare(t.Context(), "markdown", source, Edit{ID: doc.Sections[0].ID, Snapshot: doc.Snapshot, Text: fragment})
		require.Error(t, err)
	}
	_, err = Prepare(t.Context(), "markdown", source, Edit{ID: "Repeat", Snapshot: doc.Snapshot, Text: "New body"})
	require.ErrorContains(t, err, "unknown section id")
	_, err = Prepare(t.Context(), "markdown", source, Edit{ID: doc.Sections[0].ID, Text: "New body"})
	require.ErrorContains(t, err, "require id and snapshot")
}
