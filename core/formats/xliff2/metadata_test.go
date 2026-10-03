package xliff2_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

const unitMetadataDoc = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0" xmlns:mda="urn:oasis:names:tc:xliff:metadata:2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <mda:metadata>
        <mda:metaGroup category="kapi">
          <mda:meta type="if-match">absent</mda:meta>
          <mda:meta type="basis">r:0123456789abcdef</mda:meta>
        </mda:metaGroup>
        <mda:metaGroup category="vendor">
          <mda:meta type="job">42</mda:meta>
        </mda:metaGroup>
      </mda:metadata>
      <segment id="s1">
        <source>Hello</source>
        <target>Bonjour</target>
      </segment>
    </unit>
    <unit id="u2">
      <segment id="s1">
        <source>Plain</source>
      </segment>
    </unit>
  </file>
</xliff>`

// readUnits reads doc with the reader, through the streaming skeleton path
// when skeleton is set and the document tree otherwise.
func readUnits(t *testing.T, doc string, skeleton bool) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := xliff2.NewReader()
	if skeleton {
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		t.Cleanup(func() { _ = store.Close() })
		reader.SetSkeletonStore(store)
	}
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(doc, model.LocaleEnglish)))
	defer reader.Close()
	var blocks []*model.Block
	for _, p := range testutil.CollectParts(t, reader.Read(ctx)) {
		if b, ok := p.Resource.(*model.Block); ok {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

func TestUnitMetadata_ReadIntoBlockProperties(t *testing.T) {
	for _, tc := range []struct {
		name     string
		skeleton bool
	}{
		{name: "document tree", skeleton: false},
		{name: "streaming skeleton", skeleton: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blocks := readUnits(t, unitMetadataDoc, tc.skeleton)
			require.Len(t, blocks, 2)
			props := blocks[0].Properties
			assert.Equal(t, "absent", props[xliff2.UnitMetaKey(xliff2.FileNoteCategoryKapi, xliff2.UnitMetaIfMatch)])
			assert.Equal(t, "r:0123456789abcdef", props[xliff2.UnitMetaKey(xliff2.FileNoteCategoryKapi, xliff2.UnitMetaBasis)])
			assert.Equal(t, "42", props[xliff2.UnitMetaKey("vendor", "job")])
			for k := range blocks[1].Properties {
				assert.NotContains(t, k, xliff2.UnitMetaPropertyPrefix, "a unit with no metadata carries none")
			}
		})
	}
}

func TestUnitMetadata_WrittenFromBlockProperties(t *testing.T) {
	ctx := t.Context()
	block := &model.Block{ID: "u1", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "Hello"}}},
		Properties: map[string]string{
			xliff2.UnitMetaKey(xliff2.FileNoteCategoryKapi, xliff2.UnitMetaBasis):   "r:0123456789abcdef",
			xliff2.UnitMetaKey(xliff2.FileNoteCategoryKapi, xliff2.UnitMetaIfMatch): "absent",
			"context": "not metadata",
		}}
	writer := xliff2.NewWriter()
	writer.SetLocale("fr")
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "f1", Format: "xliff2", Locale: "en", IsMultilingual: true}},
		{Type: model.PartBlock, Resource: block},
	}
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())
	out := buf.String()
	assert.Contains(t, out, `<mda:metadata xmlns:mda="urn:oasis:names:tc:xliff:metadata:2.0">`)
	assert.Contains(t, out, `<mda:metaGroup category="kapi">`)
	assert.Less(t, bytes.Index(buf.Bytes(), []byte(`type="basis"`)), bytes.Index(buf.Bytes(), []byte(`type="if-match"`)), "metas are written in type order")
	assert.NotContains(t, out, "not metadata")

	back := readUnits(t, out, false)
	require.Len(t, back, 1)
	assert.Equal(t, "absent", back[0].Properties[xliff2.UnitMetaKey(xliff2.FileNoteCategoryKapi, xliff2.UnitMetaIfMatch)])
	assert.Equal(t, "r:0123456789abcdef", back[0].Properties[xliff2.UnitMetaKey(xliff2.FileNoteCategoryKapi, xliff2.UnitMetaBasis)])
}

func TestUnitMetadata_SurvivesASkeletonRoundTrip(t *testing.T) {
	assert.Equal(t, unitMetadataDoc, snippetRoundtripWithSkeleton(t, unitMetadataDoc))
}
