package vignette_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/vignette"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalEmptyDoc is a namespaced packageBody with one empty
// importProject and no content instances. The smallest valid VGNXML
// shape that the bridge daemon's document-open path accepts.
const minimalEmptyDoc = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<packageBody xmlns="http://www.vignette.com/xmlschemas/importexport">` +
	`<importProject/>` +
	`</packageBody>`

// simpleBilingualPair mirrors VignetteFilterTest#createSimpleDoc — one
// es_ES target instance + one en_US source instance for content id1,
// with HTML-wrapped body payloads.
const simpleBilingualPair = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<packageBody xmlns="http://www.vignette.com/xmlschemas/importexport">` +
	`<importProject>` +
	`<importContentInstance><contentInstance>` +
	`<attribute name="SMCCONTENT-CONTENT-ID"><valueString>id1ES</valueString></attribute>` +
	`<attribute name="SMCCONTENT-BODY"><valueCLOB>&lt;p&gt;ES&lt;/p&gt;</valueCLOB></attribute>` +
	`<attribute name="SOURCE_ID"><valueString>id1</valueString></attribute>` +
	`<attribute name="LOCALE_ID"><valueString>es_ES</valueString></attribute>` +
	`</contentInstance></importContentInstance>` +
	`<importContentInstance><contentInstance>` +
	`<attribute name="SMCCONTENT-CONTENT-ID"><valueString>id1</valueString></attribute>` +
	`<attribute name="SMCCONTENT-BODY"><valueCLOB>&lt;p&gt;ENtext&lt;/p&gt;</valueCLOB></attribute>` +
	`<attribute name="SOURCE_ID"><valueString>id1</valueString></attribute>` +
	`<attribute name="LOCALE_ID"><valueString>en_US</valueString></attribute>` +
	`</contentInstance></importContentInstance>` +
	`</importProject></packageBody>`

// complexTwoPair mirrors VignetteFilterTest#createComplexDoc — four
// importContentInstance blocks forming two bilingual pairs (id1, id2),
// interleaved in document order.
const complexTwoPair = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<packageBody xmlns="http://www.vignette.com/xmlschemas/importexport">` +
	`<importProject>` +
	`<importContentInstance><contentInstance>` +
	`<attribute name="SMCCONTENT-CONTENT-ID"><valueString>id1ES</valueString></attribute>` +
	`<attribute name="SMCCONTENT-BODY"><valueCLOB>ES-id1</valueCLOB></attribute>` +
	`<attribute name="SOURCE_ID"><valueString>id1</valueString></attribute>` +
	`<attribute name="LOCALE_ID"><valueString>es_ES</valueString></attribute>` +
	`</contentInstance></importContentInstance>` +
	`<importContentInstance><contentInstance>` +
	`<attribute name="SMCCONTENT-CONTENT-ID"><valueString>id2</valueString></attribute>` +
	`<attribute name="SMCCONTENT-BODY"><valueCLOB>EN-id2</valueCLOB></attribute>` +
	`<attribute name="SOURCE_ID"><valueString>id2</valueString></attribute>` +
	`<attribute name="LOCALE_ID"><valueString>en_US</valueString></attribute>` +
	`</contentInstance></importContentInstance>` +
	`<importContentInstance><contentInstance>` +
	`<attribute name="SMCCONTENT-CONTENT-ID"><valueString>id2ES</valueString></attribute>` +
	`<attribute name="SMCCONTENT-BODY"><valueCLOB>ES-id2</valueCLOB></attribute>` +
	`<attribute name="SOURCE_ID"><valueString>id2</valueString></attribute>` +
	`<attribute name="LOCALE_ID"><valueString>es_ES</valueString></attribute>` +
	`</contentInstance></importContentInstance>` +
	`<importContentInstance><contentInstance>` +
	`<attribute name="SMCCONTENT-CONTENT-ID"><valueString>id1</valueString></attribute>` +
	`<attribute name="SMCCONTENT-BODY"><valueCLOB>EN-id1</valueCLOB></attribute>` +
	`<attribute name="SOURCE_ID"><valueString>id1</valueString></attribute>` +
	`<attribute name="LOCALE_ID"><valueString>en_US</valueString></attribute>` +
	`</contentInstance></importContentInstance>` +
	`</importProject></packageBody>`

// plainBilingualPair is a bilingual pair with plain-text SMCCONTENT-BODY
// payloads (no HTML), so the okf_html sub-filter is a no-op and the
// extracted source text equals the raw payload.
const plainBilingualPair = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<packageBody xmlns="http://www.vignette.com/xmlschemas/importexport">` +
	`<importProject>` +
	`<importContentInstance><contentInstance>` +
	`<attribute name="SMCCONTENT-CONTENT-ID"><valueString>id1ES</valueString></attribute>` +
	`<attribute name="SMCCONTENT-BODY"><valueCLOB>bonjour</valueCLOB></attribute>` +
	`<attribute name="SOURCE_ID"><valueString>id1</valueString></attribute>` +
	`<attribute name="LOCALE_ID"><valueString>es_ES</valueString></attribute>` +
	`</contentInstance></importContentInstance>` +
	`<importContentInstance><contentInstance>` +
	`<attribute name="SMCCONTENT-CONTENT-ID"><valueString>id1</valueString></attribute>` +
	`<attribute name="SMCCONTENT-BODY"><valueCLOB>hello</valueCLOB></attribute>` +
	`<attribute name="SOURCE_ID"><valueString>id1</valueString></attribute>` +
	`<attribute name="LOCALE_ID"><valueString>en_US</valueString></attribute>` +
	`</contentInstance></importContentInstance>` +
	`</importProject></packageBody>`

func TestReadEmptyProject(t *testing.T) {
	ctx := t.Context()
	reader := vignette.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(minimalEmptyDoc, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	assert.Empty(t, blocks, "an empty importProject should produce zero translatable Blocks")
}

func TestReadSimpleBilingualPair(t *testing.T) {
	ctx := t.Context()
	reader := vignette.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleBilingualPair, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1, "one bilingual pair → one Block (the source-side body)")
	assert.Equal(t, "ENtext", blocks[0].SourceText())
	assert.Equal(t, "SMCCONTENT-BODY", blocks[0].Properties["attribute"])
	assert.Equal(t, "okf_html", blocks[0].Properties["subfilter"])
	assert.Equal(t, "en_US", blocks[0].Properties["localeId"])
	assert.Equal(t, "id1", blocks[0].Properties["sourceId"])
}

func TestReadComplexTwoPairs(t *testing.T) {
	ctx := t.Context()
	reader := vignette.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(complexTwoPair, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2, "two bilingual pairs → two Blocks in target-driven order")
	// Walking in document order, we encounter id1's target first (es_ES at
	// position 1); the source-side payload (en_US) is "EN-id1". Then id2.
	texts := []string{blocks[0].SourceText(), blocks[1].SourceText()}
	assert.Contains(t, texts, "EN-id1")
	assert.Contains(t, texts, "EN-id2")
}

func TestReadPlainPayloadBilingualPair(t *testing.T) {
	ctx := t.Context()
	reader := vignette.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(plainBilingualPair, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "hello", blocks[0].SourceText())
}

func TestReadMonolingualMode(t *testing.T) {
	ctx := t.Context()
	reader := vignette.NewReader()
	cfg := reader.Config().(*vignette.Config)
	cfg.Monolingual = true
	err := reader.Open(ctx, testutil.RawDocFromString(complexTwoPair, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	// In monolingual mode every importContentInstance contributes its
	// extracted attributes — four instances × one SMCCONTENT-BODY each.
	require.Len(t, blocks, 4)
}

func TestReadOnlySourceInstanceWithoutPair(t *testing.T) {
	// An instance whose SOURCE_ID has no partner is skipped silently in
	// bilingual mode (matching the upstream warning + drop behavior).
	input := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<packageBody xmlns="http://www.vignette.com/xmlschemas/importexport">` +
		`<importProject>` +
		`<importContentInstance><contentInstance>` +
		`<attribute name="SMCCONTENT-CONTENT-ID"><valueString>only</valueString></attribute>` +
		`<attribute name="SMCCONTENT-BODY"><valueCLOB>orphan</valueCLOB></attribute>` +
		`<attribute name="SOURCE_ID"><valueString>only</valueString></attribute>` +
		`<attribute name="LOCALE_ID"><valueString>en_US</valueString></attribute>` +
		`</contentInstance></importContentInstance>` +
		`</importProject></packageBody>`

	ctx := t.Context()
	reader := vignette.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	assert.Empty(t, blocks, "unpaired bilingual instance should not extract")
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := vignette.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := vignette.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := vignette.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(minimalEmptyDoc, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "vignette", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := vignette.NewReader()
	sig := reader.Signature()
	require.NotNil(t, sig.Sniff, "vignette uses sniff-only detection (generic .xml extension)")
	assert.True(t, sig.Sniff([]byte(`<packageBody xmlns="http://www.vignette.com/xmlschemas/importexport">`)))
	assert.True(t, sig.Sniff([]byte(`<importContentInstance>`)))
	assert.False(t, sig.Sniff([]byte(`<html>`)))
	assert.False(t, sig.Sniff([]byte(`<root xmlns="http://example.com/other"/>`)))
}

func TestReaderMetadata(t *testing.T) {
	reader := vignette.NewReader()
	assert.Equal(t, "vignette", reader.Name())
	assert.Equal(t, "Vignette CMS Export", reader.DisplayName())
}

func TestConfigFormatName(t *testing.T) {
	cfg := &vignette.Config{}
	assert.Equal(t, "vignette", cfg.FormatName())
}

func TestConfigValidate(t *testing.T) {
	cfg := &vignette.Config{}
	cfg.Reset()
	require.NoError(t, cfg.Validate())
}

func TestConfigReset(t *testing.T) {
	cfg := &vignette.Config{}
	cfg.Reset()
	assert.Equal(t, vignette.DefaultPartsNames, cfg.PartsNames)
	assert.Equal(t, vignette.DefaultPartsConfigurations, cfg.PartsConfigurations)
	assert.Equal(t, vignette.DefaultSourceID, cfg.SourceID)
	assert.Equal(t, vignette.DefaultLocaleID, cfg.LocaleID)
	assert.False(t, cfg.Monolingual)
	assert.True(t, cfg.UseCDATA)
}

func TestConfigApplyMapKnown(t *testing.T) {
	cfg := &vignette.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{
		"partsNames":          "X, Y, Z",
		"partsConfigurations": "default, okf_html, default",
		"sourceId":            "MY_SOURCE",
		"localeId":            "MY_LOCALE",
		"monolingual":         true,
		"useCDATA":            false,
	})
	require.NoError(t, err)
	assert.Equal(t, "X, Y, Z", cfg.PartsNames)
	assert.Equal(t, "default, okf_html, default", cfg.PartsConfigurations)
	assert.Equal(t, "MY_SOURCE", cfg.SourceID)
	assert.Equal(t, "MY_LOCALE", cfg.LocaleID)
	assert.True(t, cfg.Monolingual)
	assert.False(t, cfg.UseCDATA)
}

func TestConfigApplyMapUnknown(t *testing.T) {
	cfg := &vignette.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{"unknown": true})
	require.Error(t, err)
}

func TestConfigApplyMapTypeMismatch(t *testing.T) {
	cfg := &vignette.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{"monolingual": "yes"})
	require.Error(t, err)
}

func TestConfigApplyMapEmpty(t *testing.T) {
	cfg := &vignette.Config{}
	cfg.Reset()
	require.NoError(t, cfg.ApplyMap(map[string]any{}))
}

func TestConfigPartsMap(t *testing.T) {
	cfg := &vignette.Config{}
	cfg.Reset()
	m := cfg.PartsMap()
	assert.Equal(t, "okf_html", m["SMCCONTENT-BODY"])
	assert.Equal(t, "default", m["SMCCONTENT-TITLE"])
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	reader := vignette.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleBilingualPair, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)
	var count int
	for range ch {
		count++
	}
	assert.LessOrEqual(t, count, 10)
}

// TestRealWorldTest01 reads the upstream Test01.xml fixture from
// okapi-testdata. Skips cleanly when the corpus isn't available.
func TestRealWorldTest01(t *testing.T) {
	root, err := spec.FindOkapiTestdataRoot()
	if err != nil {
		t.Skipf("okapi-testdata not available: %v", err)
	}
	path := filepath.Join(root, "okapi", "filters", "vignette", "src", "test", "resources", "Test01.xml")
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("Test01.xml not available: %v", err)
	}
	defer f.Close()

	ctx := t.Context()
	reader := vignette.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, path, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	// The fixture contains 6 importContentInstance blocks pairing as
	// 3 bilingual groups across en_US / es_ES / zh_CN locales. Asserts
	// at least one Block survives extraction and the fixture parses
	// cleanly.
	assert.NotEmpty(t, blocks, "Test01.xml should yield at least one extracted Block")
}
