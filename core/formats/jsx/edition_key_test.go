package jsx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats/jsx"
	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
)

// minorBundle is a bundle in a later minor of schema 2 whose block holds an
// edition under a key with a dimension this build does not read, beside the
// plain edition in the same language.
const minorBundle = `{
  "schemaVersion": "2.1",
  "kind": "kapi-bundle",
  "generator": {"id": "test", "version": "0"},
  "project": {"id": "app", "sourceLocale": "en"},
  "documents": [{
    "id": "d1",
    "documentType": "jsx",
    "path": "app/Login.tsx",
    "blocks": [{
      "id": "d1:b1",
      "hash": "h1",
      "translatable": true,
      "type": "jsx:element",
      "editions": {
        "": {"runs": [{"text": "Log in"}]},
        "nb": {"runs": [{"text": "Logg inn"}], "status": "translated"},
        "nb;audience=kids": {"runs": [{"text": "Hopp inn"}], "status": "draft", "origin": {"kind": "ai"}}
      },
      "placeholders": [],
      "properties": {}
    }]
  }]
}`

// The edition under the key this build cannot read stays off the model, so the
// plain Norwegian edition keeps its text, and a bundle written back carries it
// as it was read.
func TestKBFKeepsAnEditionUnderAKeyItCannotRead(t *testing.T) {
	blocks := readBlocks(t, minorBundle)
	require.Len(t, blocks, 1)
	b := blocks[0]
	assert.Equal(t, "Logg inn", b.TargetText("nb"), "the plain edition is not replaced")
	assert.Equal(t, []model.EditionKey{{}, {Locale: "nb"}}, b.EditionKeys())

	var file kbf.File
	require.NoError(t, json.Unmarshal([]byte(writeBundle(t, b)), &file))
	out := file.Documents[0].Blocks[0]
	assert.Equal(t, []string{"nb", "nb;audience=kids"}, out.TargetKeys())
	assert.Equal(t, "Logg inn", model.RunsText(out.Editions["nb"].Runs))
	assert.Equal(t, kbf.Edition{
		Runs:   []kbf.Run{{Text: &kbf.TextRun{Text: "Hopp inn"}}},
		Status: "draft",
		Origin: kbf.Origin{Kind: "ai"},
	}, out.Editions["nb;audience=kids"], "carried through verbatim")
}

// writeParts runs parts through a fresh writer and returns the bundle it wrote.
func writeParts(t *testing.T, parts ...*model.Part) kbf.File {
	t.Helper()
	var sink bytes.Buffer
	w := jsx.NewWriter()
	require.NoError(t, w.SetOutputWriter(&sink))
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	require.NoError(t, w.Write(context.Background(), ch))
	require.NoError(t, w.Close())
	var file kbf.File
	require.NoError(t, json.Unmarshal(sink.Bytes(), &file))
	return file
}

// The bundle names the language its source editions are in: the project of
// the bundle the blocks were read from, else the source language a reader
// stamped on the blocks, else the language of their layer.
func TestKBFWriterNamesTheSourceLanguageOfWhatItWrites(t *testing.T) {
	block := func(locale model.LocaleID) *model.Part {
		b := model.NewRunsBlock("b1", []model.Run{model.TextR("Anmelden")})
		b.SourceLocale = locale
		return &model.Part{Type: model.PartBlock, Resource: b}
	}
	layer := func(locale model.LocaleID) *model.Part {
		return &model.Part{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1", Locale: locale}}
	}

	t.Run("a bundle keeps its own project", func(t *testing.T) {
		in := `{
  "schemaVersion": "2.0",
  "kind": "kapi-bundle",
  "generator": {"id": "test", "version": "0"},
  "project": {"id": "shop", "sourceLocale": "de"},
  "documents": [{"id": "d1", "documentType": "jsx", "path": "a.tsx", "blocks": [{
    "id": "d1:b1", "hash": "h1", "translatable": true, "type": "jsx:element",
    "editions": {"": {"runs": [{"text": "Anmelden"}]}, "en": {"runs": [{"text": "Log in"}]}},
    "placeholders": [], "properties": {}
  }]}]
}`
		blocks := readBlocks(t, in)
		require.Len(t, blocks, 1)
		file := writeParts(t, layer("en"), &model.Part{Type: model.PartBlock, Resource: blocks[0]})
		assert.Equal(t, kbf.ProjectInfo{ID: "shop", SourceLocale: "de"}, file.Project)
	})
	t.Run("a block's source language", func(t *testing.T) {
		file := writeParts(t, layer("fr"), block("de"))
		assert.Equal(t, kbf.ProjectInfo{ID: "neokapi-output", SourceLocale: "de"}, file.Project)
	})
	t.Run("the layer's language", func(t *testing.T) {
		file := writeParts(t, layer("fr"), block(""))
		assert.Equal(t, kbf.ProjectInfo{ID: "neokapi-output", SourceLocale: "fr"}, file.Project)
	})
	t.Run("nothing said", func(t *testing.T) {
		file := writeParts(t, block(""))
		assert.Equal(t, kbf.ProjectInfo{ID: "neokapi-output", SourceLocale: "en"}, file.Project)
	})
}
