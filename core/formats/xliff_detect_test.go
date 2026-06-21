package formats_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The .xlf / .xliff extension is claimed by BOTH the XLIFF 1.2 ("xliff") and
// XLIFF 2.x ("xliff2") readers at equal priority. DetectByExtension breaks the
// tie alphabetically and always returns the 1.2 reader, which cannot parse a
// 2.x <unit>/<segment> document and yields zero blocks. The lab/preview WASM
// paths therefore must use content-aware DetectFile, which sniffs among the
// candidates. These tests pin that behavior so a bilingual XLIFF 2.x file reads
// to the same blocks as the source-only one, each carrying source AND target
// runs (AD-002).
func TestDetectXLIFFVersionByContent(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	const xliff2Doc = `<?xml version="1.0"?>
<xliff version="2.2" srcLang="en" trgLang="fr" xmlns="urn:oasis:names:tc:xliff:document:2.0">
<file id="app.json" original="app.json">
<unit id="g1"><segment><source>Hello, World!</source><target>Bonjour</target></segment></unit>
</file></xliff>`

	const xliff12Doc = `<?xml version="1.0"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
<file source-language="en" target-language="fr" datatype="plaintext" original="app.json">
<body><trans-unit id="g1"><source>Hello, World!</source><target>Bonjour</target></trans-unit></body>
</file></xliff>`

	tests := []struct {
		name string
		ext  string
		body string
		want registry.FormatID
	}{
		{"2.x by content on .xliff", ".xliff", xliff2Doc, "xliff2"},
		{"2.x by content on .xlf", ".xlf", xliff2Doc, "xliff2"},
		{"1.2 by content on .xliff", ".xliff", xliff12Doc, "xliff"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "doc"+tc.ext)
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o600))

			// The content-blind helper picks the wrong reader for 2.x.
			byExt, err := reg.DetectByExtension(tc.ext)
			require.NoError(t, err)
			assert.Equal(t, registry.FormatID("xliff"), byExt,
				"DetectByExtension is content-blind and always picks the 1.2 reader")

			// The content-aware helper (used by the lab path) picks correctly.
			got, err := reg.DetectFile(path, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestLabInspectBilingualXLIFF2 reproduces the lab "inspect" path end to end:
// detect the format the way lab.go does (content-aware), read the file, and
// build the content tree. A bilingual XLIFF 2.x must read to the SAME number of
// blocks as a source-only one, each block carrying source and target runs.
func TestLabInspectBilingualXLIFF2(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	const srcOnly = `<?xml version="1.0"?>
<xliff version="2.2" srcLang="en" trgLang="fr" xmlns="urn:oasis:names:tc:xliff:document:2.0">
<file id="app.json" original="app.json">
<unit id="g1"><segment><source>Hello, World!</source></segment></unit>
<unit id="g2"><segment><source>Hello, World!</source></segment></unit>
<unit id="g3"><segment><source>Hello, World!</source></segment></unit>
</file></xliff>`

	const bilingual = `<?xml version="1.0"?>
<xliff version="2.2" srcLang="en" trgLang="fr" xmlns="urn:oasis:names:tc:xliff:document:2.0">
<file id="app.json" original="app.json">
<unit id="g1"><segment><source>Hello, World!</source><target>Bonjour</target></segment></unit>
<unit id="g2"><segment><source>Hello, World!</source><target>Bonjour</target></segment></unit>
<unit id="g3"><segment><source>Hello, World!</source><target>Bonjour</target></segment></unit>
</file></xliff>`

	inspect := func(t *testing.T, body string) *editor.ContentTree {
		t.Helper()
		path := filepath.Join(t.TempDir(), "doc.xliff")
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

		fid, err := reg.DetectFile(path, nil)
		require.NoError(t, err)
		require.Equal(t, registry.FormatID("xliff2"), fid)

		reader, err := reg.NewReader(fid)
		require.NoError(t, err)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		doc := &model.RawDocument{Reader: io.NopCloser(bytes.NewReader(data)), Encoding: "UTF-8"}
		require.NoError(t, reader.Open(context.Background(), doc))
		defer reader.Close()

		var parts []*model.Part
		for res := range reader.Read(context.Background()) {
			require.NoError(t, res.Error)
			if res.Part != nil {
				parts = append(parts, res.Part)
			}
		}
		return editor.BuildContentTree(parts, string(fid))
	}

	srcTree := inspect(t, srcOnly)
	biTree := inspect(t, bilingual)

	require.Equal(t, 3, srcTree.Stats.Blocks, "source-only must read to 3 blocks")
	require.Equal(t, srcTree.Stats.Blocks, biTree.Stats.Blocks,
		"bilingual must read to the same block count as source-only")

	// Every bilingual block carries both source and target runs.
	reader, err := reg.NewReader("xliff2")
	require.NoError(t, err)
	doc := &model.RawDocument{Reader: io.NopCloser(bytes.NewReader([]byte(bilingual))), Encoding: "UTF-8"}
	require.NoError(t, reader.Open(context.Background(), doc))
	defer reader.Close()

	var blocks []*model.Block
	for res := range reader.Read(context.Background()) {
		require.NoError(t, res.Error)
		if res.Part != nil && res.Part.Type == model.PartBlock {
			blocks = append(blocks, res.Part.Resource.(*model.Block))
		}
	}
	require.Len(t, blocks, 3)
	for i, b := range blocks {
		assert.NotEmpty(t, b.SourceRuns(), "block %d must have source runs", i)
		assert.True(t, b.HasTarget(model.LocaleFrench), "block %d must carry a French target", i)
		assert.Equal(t, "Bonjour", b.TargetText(model.LocaleFrench), "block %d target text", i)
	}
}
