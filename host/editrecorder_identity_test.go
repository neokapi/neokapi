package host

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/registry"
)

func markdownOf(paragraphs ...string) []byte {
	return []byte("# Guide\n\n" + strings.Join(paragraphs, "\n\n") + "\n")
}

func docxOf(t *testing.T, paragraphs ...string) []byte {
	t.Helper()
	var body strings.Builder
	for _, p := range paragraphs {
		body.WriteString(`<w:p><w:r><w:t>` + p + `</w:t></w:r></w:p>`)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(w, content)
		require.NoError(t, err)
	}
	add("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?>`+
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`+
		`<Default Extension="xml" ContentType="application/xml"/>`+
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>`+
		`</Types>`)
	add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>`+
		`</Relationships>`)
	add("word/document.xml", `<?xml version="1.0" encoding="UTF-8"?>`+
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`+
		body.String()+`</w:body></w:document>`)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// readBytes reads a document with the registered reader for a format.
func readBytes(t *testing.T, a *App, format, uri string, data []byte) []*model.Block {
	t.Helper()
	r, err := a.FormatReg.NewReader(registry.FormatID(format))
	require.NoError(t, err)
	require.NoError(t, r.Open(t.Context(), &model.RawDocument{
		URI: uri, SourceLocale: "en", FormatID: format, Reader: io.NopCloser(bytes.NewReader(data)),
	}))
	t.Cleanup(func() { _ = r.Close() })
	var out []*model.Block
	for res := range r.Read(t.Context()) {
		require.NoError(t, res.Error)
		if res.Part == nil || res.Part.Type != model.PartBlock {
			continue
		}
		if b, ok := res.Part.Resource.(*model.Block); ok && b.Translatable {
			out = append(out, b)
		}
	}
	return out
}

func blockWithText(t *testing.T, blocks []*model.Block, text string) *model.Block {
	t.Helper()
	for _, b := range blocks {
		if b.SourceText() == text {
			return b
		}
	}
	require.FailNow(t, "no block reads "+text)
	return nil
}

// TestRecordedIdentityReattachesHistoryAfterASiblingInsertion: a read keeps
// the keys the format reports, and a positional name moves when a paragraph
// is inserted above it. The identity evidence every record carries is what a
// later pass re-attaches history with: reconciling the whole new read against
// the block history's priors finds each recorded paragraph under the key it
// was recorded with, among siblings whose positions all moved, and reports the
// inserted paragraph as new.
func TestRecordedIdentityReattachesHistoryAfterASiblingInsertion(t *testing.T) {
	const edited = "The edited paragraph."
	tests := []struct {
		name, format, doc string
		before, after     func(t *testing.T) []byte
	}{
		{
			name: "markdown", format: "markdown", doc: "docs/guide.md",
			before: func(*testing.T) []byte { return markdownOf("First paragraph.", edited) },
			after:  func(*testing.T) []byte { return markdownOf("Inserted above.", "First paragraph.", edited) },
		},
		{
			name: "docx", format: "openxml", doc: "docs/guide.docx",
			before: func(t *testing.T) []byte { return docxOf(t, "First paragraph.", edited) },
			after:  func(t *testing.T) []byte { return docxOf(t, "Inserted above.", "First paragraph.", edited) },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, root, rec := recorderProject(t)
			a.FormatReg = appWithFormats().FormatReg
			ctx := t.Context()
			read := readBytes(t, a, tc.format, tc.doc, tc.before(t))
			first, target := blockWithText(t, read, "First paragraph."), blockWithText(t, read, edited)
			en, _ := model.ParseEditionKey("en")
			// The record names every paragraph of the document: the one an
			// agent rewrote, and the one it first wrote.
			_, err := rec.Record(ctx, change.Record{
				Actor: change.Actor{Kind: change.ActorAgent, Name: "claude"}, Origin: "apply",
				Docs: []change.DocResult{{Doc: tc.doc, Home: "file", Written: true}},
				Transitions: []change.Transition{
					{
						Ref:       change.Ref{Doc: tc.doc, Block: first.ID},
						Role:      change.RoleAuthoritative,
						After:     first.Source,
						BeforeRev: model.AbsentRevision,
						AfterRev:  model.RunsRevision(en, first.Source),
						Block:     first,
					},
					{
						Ref:       change.Ref{Doc: tc.doc, Block: target.ID},
						Role:      change.RoleAuthoritative,
						Before:    textRuns("The old paragraph."),
						After:     target.Source,
						BeforeRev: model.RunsRevision(en, textRuns("The old paragraph.")),
						AfterRev:  model.RunsRevision(en, target.Source),
						Block:     target,
					},
				},
			})
			require.NoError(t, err)

			after := readBytes(t, a, tc.format, tc.doc, tc.after(t))
			inserted := blockWithText(t, after, "Inserted above.")
			require.NotEqual(t, target.ID, blockWithText(t, after, edited).ID, "the format names the paragraph by its position")
			require.NotEqual(t, first.ID, blockWithText(t, after, "First paragraph.").ID)

			docs, err := a.DocumentIndex(ctx, root)
			require.NoError(t, err)
			key := docs.Key(tc.doc)
			db, err := a.ProjectDB(ctx, root)
			require.NoError(t, err)
			priors, err := db.History().Priors(ctx, key)
			require.NoError(t, err)
			require.Len(t, priors, 2)
			results := reconcile.Blocks(key, after, priors)
			require.Len(t, results, len(after))
			got := map[string]reconcile.Result{}
			for i, b := range after {
				got[b.SourceText()] = results[i]
			}
			assert.Equal(t, target.ID, got[edited].Key, "the rewritten paragraph re-attaches to the key it was recorded against")
			assert.NotEqual(t, reconcile.New, got[edited].Kind)
			assert.Equal(t, first.ID, got["First paragraph."].Key, "and so does its sibling")
			assert.NotEqual(t, reconcile.New, got["First paragraph."].Kind)
			assert.Equal(t, reconcile.New, got[inserted.SourceText()].Kind, "the inserted paragraph is new")
			assert.NotContains(t, []string{first.ID, target.ID}, got[inserted.SourceText()].Key,
				"the inserted paragraph takes neither recorded key")
		})
	}
}
