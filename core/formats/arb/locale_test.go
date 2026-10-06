package arb_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/arb"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// A translation's file declares its own language: app_fr.arb says
// "@@locale": "fr", however the writer reaches the file, and the source file
// written back keeps the locale it declared.

const localeSource = "{\n  \"@@locale\": \"en\",\n  \"hello\": \"Hello\",\n  \"bye\": \"Goodbye\"\n}\n"

// translate gives every block a French translation.
func translate(parts []*model.Part, locale model.LocaleID) {
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && p.Type == model.PartBlock {
			b.SetTargetText(locale, "fr:"+b.SourceText())
		}
	}
}

// writeVia writes input for locale through one of the writer's three paths.
func writeVia(t *testing.T, path, input string, locale model.LocaleID) string {
	t.Helper()
	ctx := context.Background()
	reader, writer := arb.NewReader(), arb.NewWriter()
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	if locale != "" {
		writer.SetLocale(locale)
	}
	switch path {
	case "original", "skeleton":
		if path == "skeleton" {
			store, err := format.NewSkeletonStore()
			require.NoError(t, err)
			defer store.Close()
			reader.SetSkeletonStore(store)
			writer.SetSkeletonStore(store)
		}
		require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
		parts := testutil.CollectParts(t, reader.Read(ctx))
		reader.Close()
		translate(parts, locale)
		require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	case "streaming":
		file := filepath.Join(t.TempDir(), "app_en.arb")
		require.NoError(t, os.WriteFile(file, []byte(input), 0o644))
		f, err := os.Open(file)
		require.NoError(t, err)
		store := format.NewStreamingSkeletonStore()
		reader.SetSkeletonStore(store)
		writer.SetSkeletonStore(store)
		require.NoError(t, reader.Open(ctx, &model.RawDocument{URI: file, SourceLocale: model.LocaleEnglish, Reader: f}))
		ch := make(chan *model.Part, 64)
		go func() {
			defer close(ch)
			defer store.CloseWrite()
			defer reader.Close()
			for res := range reader.Read(ctx) {
				assert.NoError(t, res.Error)
				if res.Part != nil {
					translate([]*model.Part{res.Part}, locale)
					ch <- res.Part
				}
			}
		}()
		require.NoError(t, writer.Write(ctx, ch))
	case "scratch":
		layer := &model.Layer{ID: "doc1", Format: "arb", Properties: map[string]string{"arb.locale": "en"}}
		b := model.NewBlock("tu1", "Hello")
		b.Name, b.SourceLocale, b.Properties = "hello", "en", map[string]string{"arb.key": "hello"}
		parts := []*model.Part{
			{Type: model.PartLayerStart, Resource: layer},
			{Type: model.PartBlock, Resource: b},
			{Type: model.PartLayerEnd, Resource: layer},
		}
		translate(parts, locale)
		require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	}
	require.NoError(t, writer.Close())
	return buf.String()
}

func TestWriter_DeclaresTheLocaleItWrites(t *testing.T) {
	for _, path := range []string{"original", "skeleton", "streaming", "scratch"} {
		t.Run(path, func(t *testing.T) {
			assert.Contains(t, writeVia(t, path, localeSource, "fr"), `"@@locale": "fr"`,
				"the French file names French")
			assert.Contains(t, writeVia(t, path, localeSource, "pt-BR"), `"@@locale": "pt_BR"`,
				"a region is spelled as Flutter names a locale")
			assert.Contains(t, writeVia(t, path, localeSource, ""), `"@@locale": "en"`,
				"the source written back keeps the locale it declares")
			assert.Contains(t, writeVia(t, path, localeSource, "en"), `"@@locale": "en"`,
				"a file written in the language it declares keeps it")
		})
	}
}

// A source that spells its locale with a hyphen gets its translations spelled
// the same way, and the source written back is byte-exact.
func TestWriter_KeepsTheSourcesSpellingOfALocale(t *testing.T) {
	in := "{\n  \"@@locale\": \"en-US\",\n  \"hello\": \"Hello\"\n}\n"
	for _, path := range []string{"original", "skeleton", "streaming"} {
		t.Run(path, func(t *testing.T) {
			assert.Contains(t, writeVia(t, path, in, "pt-BR"), `"@@locale": "pt-BR"`)
			assert.Equal(t, in, writeVia(t, path, in, ""), "the source replays byte-exact")
		})
	}
}
