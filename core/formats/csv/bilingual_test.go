package csv_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	csvfmt "github.com/neokapi/neokapi/core/formats/csv"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for bilingual (column-role) mode — the translation-table shape
// absorbed from the retired transtable format (#1074): a key column, a
// source column and a target column, one unit per row. Configured via the
// `columns` config key.

func bilingualTSVReader(t *testing.T) *csvfmt.Reader {
	t.Helper()
	reader := csvfmt.NewTSVReader()
	cfg := reader.Config().(*csvfmt.Config)
	require.NoError(t, cfg.ApplyMap(map[string]any{
		"columns": map[string]any{"key": 0, "source": 1, "target": 2},
	}))
	require.NoError(t, cfg.Validate())
	return reader
}

func bilingualRawDoc(t *testing.T, content string) *model.RawDocument {
	t.Helper()
	return &model.RawDocument{
		URI:          "test://translation-table.tsv",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleID("fr"),
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(strings.NewReader(content)),
	}
}

func loadTranslationTableFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "translation-table.tsv"))
	require.NoError(t, err)
	return string(data)
}

func TestBilingual_TSV_Read(t *testing.T) {
	ctx := t.Context()
	reader := bilingualTSVReader(t)
	require.NoError(t, reader.Open(ctx, bilingualRawDoc(t, loadTranslationTableFixture(t))))
	defer reader.Close()

	all := testutil.CollectBlocks(t, reader.Read(ctx))
	// The table projection also emits non-translatable header cells;
	// the translation units are the translatable blocks.
	var blocks []*model.Block
	for _, b := range all {
		if b.Translatable {
			blocks = append(blocks, b)
		}
	}
	require.Len(t, blocks, 3)

	assert.Equal(t, "greeting", blocks[0].ID)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.True(t, blocks[0].HasTarget(model.LocaleID("fr")))
	assert.Equal(t, "Bonjour", blocks[0].TargetText(model.LocaleID("fr")))

	assert.Equal(t, "farewell", blocks[1].ID)
	assert.Equal(t, "Goodbye", blocks[1].SourceText())
	assert.False(t, blocks[1].HasTarget(model.LocaleID("fr")), "empty target cell should not produce a target")

	assert.Equal(t, "question", blocks[2].ID)
	assert.Equal(t, "How are you?", blocks[2].SourceText())
	assert.Equal(t, "Comment allez-vous ?", blocks[2].TargetText(model.LocaleID("fr")))
}

// bilingualSkeletonRoundtrip runs read → (mutate) → write through a
// skeleton store and returns the reconstructed document.
func bilingualSkeletonRoundtrip(t *testing.T, reader *csvfmt.Reader, writer *csvfmt.Writer, input string, locale model.LocaleID, mutate func([]*model.Part)) string {
	t.Helper()
	ctx := t.Context()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, bilingualRawDoc(t, input)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	if mutate != nil {
		mutate(parts)
	}

	var buf bytes.Buffer
	if !locale.IsEmpty() {
		writer.SetLocale(locale)
	}
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	return buf.String()
}

func TestBilingual_TSV_Roundtrip_ByteExact(t *testing.T) {
	input := loadTranslationTableFixture(t)
	output := bilingualSkeletonRoundtrip(t, bilingualTSVReader(t), csvfmt.NewTSVWriter(), input, "", nil)
	assert.Equal(t, input, output, "unchanged bilingual table should round-trip byte-exact")
}

func TestBilingual_TSV_Merge_FillsTargetColumn(t *testing.T) {
	locale := model.LocaleID("fr")
	input := loadTranslationTableFixture(t)
	output := bilingualSkeletonRoundtrip(t, bilingualTSVReader(t), csvfmt.NewTSVWriter(), input, locale, func(parts []*model.Part) {
		for _, p := range parts {
			if p.Type != model.PartBlock {
				continue
			}
			b := p.Resource.(*model.Block)
			switch b.ID {
			case "greeting":
				b.SetTargetText(locale, "Salut")
			case "farewell":
				b.SetTargetText(locale, "Au revoir")
			}
		}
	})

	want := "id\tsource\ttarget\n" +
		"greeting\tHello\tSalut\n" +
		"farewell\tGoodbye\tAu revoir\n" +
		"question\tHow are you?\tComment allez-vous ?\n"
	assert.Equal(t, want, output, "merge should fill the target column and leave key/source columns untouched")
}

func TestBilingual_CSV_QuotedCells_Roundtrip(t *testing.T) {
	reader := func() *csvfmt.Reader {
		r := csvfmt.NewReader()
		cfg := r.Config().(*csvfmt.Config)
		require.NoError(t, cfg.ApplyMap(map[string]any{
			"columns": map[string]any{"key": 0, "source": 1, "target": 2},
		}))
		require.NoError(t, cfg.Validate())
		return r
	}

	input := "id,source,target\n" +
		"greeting,\"Hello, world\",\"Bonjour, monde\"\n" +
		"farewell,Goodbye,\n"

	output := bilingualSkeletonRoundtrip(t, reader(), csvfmt.NewWriter(), input, "", nil)
	assert.Equal(t, input, output, "quoted bilingual CSV should round-trip byte-exact")

	locale := model.LocaleID("fr")
	output = bilingualSkeletonRoundtrip(t, reader(), csvfmt.NewWriter(), input, locale, func(parts []*model.Part) {
		for _, p := range parts {
			if p.Type != model.PartBlock {
				continue
			}
			b := p.Resource.(*model.Block)
			if b.ID == "farewell" {
				b.SetTargetText(locale, "Au revoir")
			}
		}
	})
	want := "id,source,target\n" +
		"greeting,\"Hello, world\",\"Bonjour, monde\"\n" +
		"farewell,Goodbye,Au revoir\n"
	assert.Equal(t, want, output)
}

func TestBilingual_Config_Validation(t *testing.T) {
	cfg := csvfmt.NewTSVReader().Config().(*csvfmt.Config)
	require.NoError(t, cfg.ApplyMap(map[string]any{"columns": map[string]any{"source": 1}}))
	assert.Error(t, cfg.Validate(), "source without target should fail validation")

	cfg = csvfmt.NewTSVReader().Config().(*csvfmt.Config)
	require.NoError(t, cfg.ApplyMap(map[string]any{"columns": map[string]any{"source": 1, "target": 1}}))
	assert.Error(t, cfg.Validate(), "identical source and target columns should fail validation")

	cfg = csvfmt.NewTSVReader().Config().(*csvfmt.Config)
	err := cfg.ApplyMap(map[string]any{"columns": map[string]any{"origin": 1}})
	assert.Error(t, err, "unknown column role should be rejected")
}
