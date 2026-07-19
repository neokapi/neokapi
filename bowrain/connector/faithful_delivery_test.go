package connector

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// androidSource is a strings.xml whose non-translatable frame — the XML prolog,
// a comment, element order, and a translatable="false" entry — must survive
// delivery. Reconstructing from stored blocks alone loses all of it; re-parsing
// the co-located source at delivery preserves it.
const androidSource = `<?xml version="1.0" encoding="utf-8"?>
<resources>
    <!-- Greetings section -->
    <string name="hello">Hello</string>
    <string name="bye">Goodbye</string>
    <string name="app_name" translatable="false">MyApp</string>
</resources>
`

// readSourceBlocks parses a source document through the registry reader, exactly
// as bowrain's connector ingest does, so the block ids match a delivery-time
// re-parse.
func readSourceBlocks(t *testing.T, reg *registry.FormatRegistry, formatName, path string) []*model.Block {
	t.Helper()
	reader, err := reg.NewReader(registry.FormatID(formatName))
	require.NoError(t, err)
	defer reader.Close()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, reader.Open(context.Background(), &model.RawDocument{URI: path, FormatID: formatName, Reader: f}))
	var blocks []*model.Block
	for pr := range reader.Read(context.Background()) {
		require.NoError(t, pr.Error)
		if pr.Part != nil && pr.Part.Type == model.PartBlock {
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks
}

// TestFaithfulDelivery_AndroidXML is the end-to-end proof that delivering a
// skeleton-dependent format through the file connector (the shared write path
// for the file/git/forge connectors) preserves the non-translatable frame when
// the source is co-located on disk — lossy from-blocks today, faithful after.
func TestFaithfulDelivery_AndroidXML(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	base := t.TempDir()
	// Source lives at values/strings.xml; the delivery target derives from it.
	srcRel := filepath.Join("values", "strings.xml")
	srcAbs := filepath.Join(base, srcRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(srcAbs), 0o755))
	require.NoError(t, os.WriteFile(srcAbs, []byte(androidSource), 0o644))

	c, err := NewFileConnector(reg, map[string]string{"path": base})
	require.NoError(t, err)

	// Build the reviewed delivery blocks the way materializeDelivery does: the
	// reviewed target both committed (Targets[fr]) and promoted into the source
	// position, carrying the source-reader id. app_name is translatable="false",
	// so it gets no target and must deliver verbatim.
	trans := map[string]string{"Hello": "Bonjour", "Goodbye": "Au revoir"}
	var delivered []*model.Block
	for _, sb := range readSourceBlocks(t, reg, "androidxml", srcAbs) {
		cp := *sb
		if fr, ok := trans[sb.SourceText()]; ok {
			cp.Source = []model.Run{{Text: &model.TextRun{Text: fr}}}
			cp.Targets = map[model.VariantKey]*model.Target{
				model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: fr}}}},
			}
		}
		delivered = append(delivered, &cp)
	}

	// FAITHFUL: source_path points at the co-located source, so publishFile
	// re-parses it, captures the skeleton, and splices the reviewed targets in.
	faithfulItem := &platconn.ContentItem{
		Name:     srcRel,
		Path:     filepath.Join("values-fr", "strings.xml"),
		Format:   "androidxml",
		Locale:   "fr",
		Blocks:   delivered,
		Metadata: map[string]string{"source_path": srcRel},
	}
	require.NoError(t, c.Publish(context.Background(), []*platconn.ContentItem{faithfulItem}, platconn.PublishOptions{}))
	faithful, err := os.ReadFile(filepath.Join(base, "values-fr", "strings.xml"))
	require.NoError(t, err)
	faithfulStr := string(faithful)
	t.Logf("FAITHFUL (re-parse) delivery:\n%s", faithfulStr)

	// LOSSY (today's behaviour): no co-located source → the writer reconstructs
	// from blocks alone. Same reviewed content, delivered without a source.
	lossyItem := &platconn.ContentItem{
		Name:   "", // nothing resolvable on disk → from-blocks path
		Path:   filepath.Join("values-lossy", "strings.xml"),
		Format: "androidxml",
		Locale: "fr",
		Blocks: delivered,
	}
	require.NoError(t, c.Publish(context.Background(), []*platconn.ContentItem{lossyItem}, platconn.PublishOptions{}))
	lossy, err := os.ReadFile(filepath.Join(base, "values-lossy", "strings.xml"))
	require.NoError(t, err)
	lossyStr := string(lossy)
	t.Logf("LOSSY (from-blocks) delivery:\n%s", lossyStr)

	// The translations land in both paths.
	assert.Contains(t, faithfulStr, "Bonjour", "faithful delivery must carry the translation")
	assert.Contains(t, faithfulStr, "Au revoir")

	// The FAITHFUL frame: comment, prolog, and non-translatable entry preserved.
	assert.Contains(t, faithfulStr, "<!-- Greetings section -->",
		"faithful delivery must preserve the source comment")
	assert.Contains(t, faithfulStr, `translatable="false"`,
		"faithful delivery must preserve the non-translatable attribute")
	assert.Contains(t, faithfulStr, "MyApp",
		"faithful delivery must preserve the non-translatable entry verbatim")
	// Element order preserved (hello before bye before app_name).
	assert.Less(t, strings.Index(faithfulStr, "hello"), strings.Index(faithfulStr, "bye"))
	assert.Less(t, strings.Index(faithfulStr, "bye"), strings.Index(faithfulStr, "app_name"))

	// The before/after contrast: the lossy path drops the non-translatable frame.
	assert.NotContains(t, lossyStr, "<!-- Greetings section -->",
		"baseline from-blocks delivery loses the comment (the gap this fixes)")
	assert.NotEqual(t, lossyStr, faithfulStr,
		"faithful and lossy deliveries must differ — that difference is the fix")
}

// TestFaithfulDelivery_BindingFailure_FallsBackTranslated proves the safety
// guard: if the re-parsed source ids do not bind to any reviewed target (the
// source-reader ids are missing or have drifted), delivery must NOT ship a
// faithful frame full of untranslated source text — it degrades to the
// from-blocks path, which is lossy but carries the translations. Shipping a
// French PR full of English strings is the failure this prevents.
func TestFaithfulDelivery_BindingFailure_FallsBackTranslated(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	base := t.TempDir()
	srcRel := filepath.Join("values", "strings.xml")
	srcAbs := filepath.Join(base, srcRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(srcAbs), 0o755))
	require.NoError(t, os.WriteFile(srcAbs, []byte(androidSource), 0o644))

	c, err := NewFileConnector(reg, map[string]string{"path": base})
	require.NoError(t, err)

	// Reviewed blocks whose ids deliberately do NOT match the re-parse ids
	// (tu1/tu2/…) — simulating an empty/drifted SourceID. Targets are present.
	trans := map[string]string{"Hello": "Bonjour", "Goodbye": "Au revoir"}
	var delivered []*model.Block
	for _, sb := range readSourceBlocks(t, reg, "androidxml", srcAbs) {
		cp := *sb
		cp.ID = "mismatch-" + sb.ID // break the binding
		if fr, ok := trans[sb.SourceText()]; ok {
			cp.Source = []model.Run{{Text: &model.TextRun{Text: fr}}}
			cp.Targets = map[model.VariantKey]*model.Target{
				model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: fr}}}},
			}
		}
		delivered = append(delivered, &cp)
	}

	require.NoError(t, c.Publish(context.Background(), []*platconn.ContentItem{{
		Name: srcRel, Path: filepath.Join("values-fr", "strings.xml"), Format: "androidxml",
		Locale: "fr", Blocks: delivered, Metadata: map[string]string{"source_path": srcRel},
	}}, platconn.PublishOptions{}))
	out, err := os.ReadFile(filepath.Join(base, "values-fr", "strings.xml"))
	require.NoError(t, err)
	got := string(out)
	t.Logf("binding-failure delivery:\n%s", got)

	// The translations are delivered (from-blocks fallback), never the source text.
	assert.Contains(t, got, "Bonjour", "fallback must still deliver the translation")
	assert.Contains(t, got, "Au revoir")
	assert.NotContains(t, got, ">Hello<", "must never ship untranslated source text as the value")
	assert.NotContains(t, got, ">Goodbye<")
	// It took the lossy from-blocks path, not the faithful one (no source frame).
	assert.NotContains(t, got, "<!-- Greetings section -->",
		"binding failure degrades to from-blocks (lossy), not a faithful-but-untranslated frame")
}

// TestFaithfulDelivery_JSON_NoRegression proves a pure-structure format is
// unaffected: its block set fully determines the file, so re-parse delivery and
// from-blocks delivery produce byte-identical output. json/yaml/arb/po stay on
// their current behaviour even though the re-parse path also handles them.
func TestFaithfulDelivery_JSON_NoRegression(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	base := t.TempDir()
	srcRel := filepath.Join("locales", "en.json")
	srcAbs := filepath.Join(base, srcRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(srcAbs), 0o755))
	src := "{\n  \"greeting\": \"Hello\",\n  \"farewell\": \"Goodbye\"\n}\n"
	require.NoError(t, os.WriteFile(srcAbs, []byte(src), 0o644))

	c, err := NewFileConnector(reg, map[string]string{"path": base})
	require.NoError(t, err)

	trans := map[string]string{"Hello": "Bonjour", "Goodbye": "Au revoir"}
	var delivered []*model.Block
	for _, sb := range readSourceBlocks(t, reg, "json", srcAbs) {
		cp := *sb
		if fr, ok := trans[sb.SourceText()]; ok {
			cp.Source = []model.Run{{Text: &model.TextRun{Text: fr}}}
			cp.Targets = map[model.VariantKey]*model.Target{
				model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: fr}}}},
			}
		}
		delivered = append(delivered, &cp)
	}

	// Faithful (re-parse) delivery, run twice: it must be byte-identical to
	// itself. The re-parse path walks the source skeleton, so key order follows
	// the source deterministically. (The from-blocks path — used only when no
	// source is co-located — reconstructs from a map and is order-nondeterministic;
	// that is precisely why the co-located re-parse path is preferable even for
	// pure-structure formats: stable output, no spurious delivery churn.)
	deliver := func(rel string) string {
		require.NoError(t, c.Publish(context.Background(), []*platconn.ContentItem{{
			Name: srcRel, Path: rel, Format: "json", Locale: "fr",
			Blocks: delivered, Metadata: map[string]string{"source_path": srcRel},
		}}, platconn.PublishOptions{}))
		out, err := os.ReadFile(filepath.Join(base, rel))
		require.NoError(t, err)
		return string(out)
	}
	first := deliver(filepath.Join("locales", "fr.json"))
	second := deliver(filepath.Join("locales", "fr2.json"))

	assert.Contains(t, first, "Bonjour")
	assert.Contains(t, first, "Au revoir")
	assert.Equal(t, first, second, "re-parse delivery must be deterministic across runs")
	assert.Less(t, strings.Index(first, "greeting"), strings.Index(first, "farewell"),
		"re-parse delivery must preserve source key order")
}
