package xliff2_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oddTargetLocale repeats an extension singleton. It is not well-formed
// BCP-47, the locale gate accepts it, and its canonical form is reached in two
// reads, so the key the reader files the translation under is reached only
// when the canonical form is its own canonical form.
const oddTargetLocale = "fr-u-01-01-u-00-00"

// The segment state reaches the translation's status, and the status reaches
// the segment state, whatever spelling of a locale the document's trgLang
// carries, and with no target language at all.
func TestXLIFF2_StateUnderAnyTargetLocale(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		doc := strings.Replace(statefulXLIFF2, `trgLang="fr"`, `trgLang="`+oddTargetLocale+`"`, 1)
		reader := xliff2.NewReader()
		require.NoError(t, reader.Open(t.Context(), testutil.RawDocFromString(doc, model.LocaleEnglish)))
		defer reader.Close()
		blocks := testutil.CollectBlocks(t, reader.Read(t.Context()))
		require.Len(t, blocks, 4)
		tgt, ok := blocks[1].Edition(model.EditionKey{Locale: oddTargetLocale})
		require.True(t, ok)
		assert.Equal(t, "Au revoir", model.RunsText(tgt.Runs))
		assert.Equal(t, model.Status(model.TargetStatusEstablished), tgt.Status, "state=final")
	})
	t.Run("write", func(t *testing.T) {
		out := writeEstablished(t, oddTargetLocale)
		assert.Contains(t, out, "<target>Bonjour</target>")
		assert.Contains(t, out, `state="final"`)
	})
	// With no target language the writer writes a translation filed under the
	// empty locale, such as one the Qt TS reader files for a file with no
	// language attribute, and reports its status.
	t.Run("write with no target language", func(t *testing.T) {
		out := writeEstablished(t, "")
		assert.Contains(t, out, "<target>Bonjour</target>")
		assert.Contains(t, out, `state="final"`)
	})
}

// writeEstablished writes one block from scratch with an established
// translation under locale, naming locale as the document's target language.
func writeEstablished(t *testing.T, locale model.LocaleID) string {
	t.Helper()
	buf := &bytes.Buffer{}
	w := xliff2.NewWriter()
	require.NoError(t, w.SetOutputWriter(buf))
	layer := &model.Layer{ID: "file-f1", Name: "f1", Format: "xliff2", Locale: "en", IsMultilingual: true}
	if locale != "" {
		layer.Properties = map[string]string{"target-language": string(locale)}
	}
	block := model.NewBlock("u1", "Hello")
	block.SetTargetText(locale, "Bonjour")
	block.StampTargetProvenance(locale, model.TargetStatusEstablished, model.Origin{Kind: model.OriginHuman})
	parts := make(chan *model.Part, 2)
	parts <- &model.Part{Type: model.PartLayerStart, Resource: layer}
	parts <- &model.Part{Type: model.PartBlock, Resource: block}
	close(parts)
	require.NoError(t, w.Write(context.Background(), parts))
	return buf.String()
}
