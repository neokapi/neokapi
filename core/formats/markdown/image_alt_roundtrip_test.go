package markdown_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noTranslatedAlt turns off translateImageAlt, the okapi-faithful setting under
// which an image's alt text is not offered to the translator.
func noTranslatedAlt(c *markdown.Config) {
	_ = c.ApplyMap(map[string]any{"translateImageAlt": false})
}

// TestImageAltSurvivesWithTranslationOff is the #2447 reproducer. buildImageRuns
// walked an image's children only when the alt text was translatable, so with
// the flag off the alt bytes were in neither the runs nor the skeleton and the
// writer had nothing to put back: `![alt](s)` came out as `![](s)`. The alt now
// rides the image's opening code and its canonical alt attribute.
func TestImageAltSurvivesWithTranslationOff(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"![alt](s)\n",
		"![alt](s 't')\n",
		"![alt](s \"t\")\n",
		"x ![alt][r] y\n\n[r]: s\n",
		"x ![alt][] y\n\n[alt]: s\n",
		"x ![alt] y\n\n[alt]: s\n",
		"![](s)\n",
		"![*emphasised* alt](s)\n",
		"![alt with \\] a bracket](s)\n",
		"See ![one](a) and ![two](b) here.\n",
		"[![alt](i)](l)\n",
		"- ![alt](s)\n",
		"> ![alt](s)\n",
		"# ![alt](s)\n",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			out := roundtripWithSkeletonConfig(t, in, noTranslatedAlt)
			assert.Equal(t, in, out, "the alt text has to round-trip untouched")
		})
	}
}

// TestImageAltIsNotTranslatableWithTranslationOff pins the other half of the
// flag: the alt survives, and it is still not the translator's. It stays out of
// the block's text, which is what okapi's MarkdownFilter extracts, and travels
// on the run's alt attribute so a writer for another format can put it back.
func TestImageAltIsNotTranslatableWithTranslationOff(t *testing.T) {
	t.Parallel()
	blocks := readBlocksWithConfig(t, "See ![My Alt](image.png \"My Title\") here.\n", noTranslatedAlt)
	require.Len(t, blocks, 1)
	assert.NotContains(t, blocks[0].SourceText(), "My Alt")
	assert.Contains(t, blocks[0].SourceText(), "My Title", "the title stays translatable")

	var open *model.PcOpenRun
	for _, r := range blocks[0].Source {
		if r.PcOpen != nil && r.PcOpen.Type == "media:image" {
			open = r.PcOpen
			break
		}
	}
	require.NotNil(t, open)
	assert.Equal(t, "My Alt", open.Attr(model.AttrAlt))
	assert.Equal(t, "![My Alt", open.Data)
}

// TestImageAltRebuildsWithTranslationOff covers the no-skeleton path, where the
// alt has to come off the attribute rather than out of the source bytes.
func TestImageAltRebuildsWithTranslationOff(t *testing.T) {
	t.Parallel()
	blocks := readBlocksWithConfig(t, "See ![My Alt](image.png) here.\n", noTranslatedAlt)
	require.Len(t, blocks, 1)
	assert.Equal(t, "See ![My Alt](image.png) here.\n", rebuildBlocks(t, blocks[0]))
}

// TestImageAltTranslatedStaysTheSource writes the document back for another
// locale: the alt is not offered, so it stays as the source spelled it while
// the prose around it is translated.
func TestImageAltTranslatedStaysTheSource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	reader := markdown.NewReader()
	noTranslatedAlt(reader.MarkdownConfig())
	writer := markdown.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	const src = "See ![My Alt](image.png) here.\n"
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(src, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	target := make([]model.Run, 0, len(blocks[0].Source))
	for _, r := range blocks[0].Source {
		if r.Text == nil {
			target = append(target, r)
			continue
		}
		target = append(target, model.Run{Text: &model.TextRun{Text: "X"}})
	}
	blocks[0].SetTargetRuns(model.LocaleGerman, target)

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(model.LocaleGerman)
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	assert.Equal(t, "X![My Alt](image.png)X\n", buf.String())
}

// TestImageAltFixture walks the committed fixture under both settings.
func TestImageAltFixture(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/image-alt.md")
	require.NoError(t, err)
	assertSkeletonByteExact(t, string(data))
	assert.Equal(t, string(data), roundtripWithSkeletonConfig(t, string(data), noTranslatedAlt))
}
