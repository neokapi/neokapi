package ttx_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/formats/ttx"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigDefaults(t *testing.T) {
	cfg := &ttx.Config{}
	cfg.Reset()
	assert.Equal(t, "ttx", cfg.FormatName())
	assert.Equal(t, ttx.SegmentModeAuto, cfg.SegmentMode)
	assert.False(t, cfg.EscapeGT)
	require.NoError(t, cfg.Validate())
}

func TestConfigApplyMapAll(t *testing.T) {
	cfg := &ttx.Config{}
	cfg.Reset()

	err := cfg.ApplyMap(map[string]any{
		"segmentMode": float64(2),
		"escapeGT":    true,
	})
	require.NoError(t, err)
	assert.Equal(t, ttx.SegmentModeAll, cfg.SegmentMode)
	assert.True(t, cfg.EscapeGT)
}

func TestConfigApplyMapIntSegmentMode(t *testing.T) {
	cfg := &ttx.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{"segmentMode": 1})
	require.NoError(t, err)
	assert.Equal(t, ttx.SegmentModeExistingOnly, cfg.SegmentMode)
}

func TestConfigApplyMapUnknown(t *testing.T) {
	cfg := &ttx.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{"unknown": "value"})
	require.Error(t, err)
}

func TestConfigApplyMapTypeMismatch(t *testing.T) {
	cfg := &ttx.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{"escapeGT": "notabool"})
	require.Error(t, err)
}

func TestConfigValidateInvalidSegmentMode(t *testing.T) {
	cfg := &ttx.Config{SegmentMode: 5}
	err := cfg.Validate()
	require.Error(t, err)
}

func TestConfigKind(t *testing.T) {
	cfg := &ttx.Config{}
	kind := cfg.ConfigKind()
	assert.Contains(t, string(kind), "Ttx")
}

func TestSegmentModeAll(t *testing.T) {
	ctx := t.Context()
	reader := ttx.NewReader()

	cfg := reader.Config().(*ttx.Config)
	cfg.SegmentMode = ttx.SegmentModeAll

	// TTX with both Tu elements and raw unsegmented text
	input := `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
Some unsegmented text before
<Tu MatchPercent="0">
<Tuv Lang="EN-US">Segmented text</Tuv>
</Tu>
More unsegmented text after
</Raw>
</Body>
</TRADOStag>`

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	// Should extract both unsegmented text parts and segmented Tu
	require.GreaterOrEqual(t, len(blocks), 2)
	// Find the segmented block
	var foundSegmented, foundUnsegmented bool
	for _, b := range blocks {
		if b.SourceText() == "Segmented text" {
			foundSegmented = true
		}
		if b.Properties["unsegmented"] == "true" {
			foundUnsegmented = true
		}
	}
	assert.True(t, foundSegmented, "should have a segmented block")
	assert.True(t, foundUnsegmented, "should have unsegmented blocks")
}

func TestSegmentModeExistingOnly(t *testing.T) {
	ctx := t.Context()
	reader := ttx.NewReader()

	cfg := reader.Config().(*ttx.Config)
	cfg.SegmentMode = ttx.SegmentModeExistingOnly

	input := `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
Some unsegmented text
<Tu MatchPercent="0">
<Tuv Lang="EN-US">Segmented text</Tuv>
</Tu>
</Raw>
</Body>
</TRADOStag>`

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	// Should only extract the Tu element, not unsegmented text
	require.Len(t, blocks, 1)
	assert.Equal(t, "Segmented text", blocks[0].SourceText())
}

func TestEscapeGTWriter(t *testing.T) {
	ctx := t.Context()

	// Read a simple TTX to get parts
	reader := ttx.NewReader()
	input := `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
<Tu MatchPercent="0">
<Tuv Lang="EN-US">a &gt; b</Tuv>
</Tu>
</Raw>
</Body>
</TRADOStag>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write with EscapeGT=false
	writer := ttx.NewWriter()
	writer.Config().EscapeGT = false

	var buf bytes.Buffer
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale("EN-US")

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	// With EscapeGT=false, > should NOT be escaped
	assert.Contains(t, output, "a > b")
	assert.NotContains(t, output, "a &gt; b")
}

func TestEscapeGTWriterEnabled(t *testing.T) {
	ctx := t.Context()

	reader := ttx.NewReader()
	input := `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
<Tu MatchPercent="0">
<Tuv Lang="EN-US">a &gt; b</Tuv>
</Tu>
</Raw>
</Body>
</TRADOStag>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write with EscapeGT=true (default)
	writer := ttx.NewWriter()
	writer.Config().EscapeGT = true

	var buf bytes.Buffer
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale("EN-US")

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	// With EscapeGT=true, > should be escaped
	assert.Contains(t, output, "&gt;")
}
