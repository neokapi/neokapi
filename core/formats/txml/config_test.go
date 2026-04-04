package txml_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/formats/txml"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigDefaults(t *testing.T) {
	cfg := &txml.Config{}
	cfg.Reset()
	assert.Equal(t, "txml", cfg.FormatName())
	assert.True(t, cfg.AllowEmptyOutputTarget)
	require.NoError(t, cfg.Validate())
}

func TestConfigApplyMap(t *testing.T) {
	cfg := &txml.Config{}
	cfg.Reset()

	err := cfg.ApplyMap(map[string]any{
		"allowEmptyOutputTarget": false,
	})
	require.NoError(t, err)
	assert.False(t, cfg.AllowEmptyOutputTarget)
}

func TestConfigApplyMapUnknown(t *testing.T) {
	cfg := &txml.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{"unknown": "value"})
	require.Error(t, err)
}

func TestConfigApplyMapTypeMismatch(t *testing.T) {
	cfg := &txml.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{"allowEmptyOutputTarget": "notabool"})
	require.Error(t, err)
}

func TestConfigKind(t *testing.T) {
	cfg := &txml.Config{}
	kind := cfg.ConfigKind()
	assert.Contains(t, string(kind), "Txml")
}

func TestAllowEmptyOutputTargetEnabled(t *testing.T) {
	ctx := context.Background()

	// Read a TXML with source-only segments
	reader := txml.NewReader()
	input := `<?xml version="1.0" encoding="utf-8"?>
<txml locale="en-US" targetlocale="fr-FR" version="1.0" datatype="xml">
<header/>
<body>
<segment segtype="block">
<source>Source only text</source>
</segment>
</body>
</txml>`

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write with AllowEmptyOutputTarget=true (default)
	writer := txml.NewWriter()
	writer.Config().AllowEmptyOutputTarget = true

	var buf bytes.Buffer
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleID("fr-FR"))

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	// Should include empty <target/> element
	assert.Contains(t, output, "<target/>")
}

func TestAllowEmptyOutputTargetDisabled(t *testing.T) {
	ctx := context.Background()

	reader := txml.NewReader()
	input := `<?xml version="1.0" encoding="utf-8"?>
<txml locale="en-US" targetlocale="fr-FR" version="1.0" datatype="xml">
<header/>
<body>
<segment segtype="block">
<source>Source only text</source>
</segment>
</body>
</txml>`

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write with AllowEmptyOutputTarget=false
	writer := txml.NewWriter()
	writer.Config().AllowEmptyOutputTarget = false

	var buf bytes.Buffer
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleID("fr-FR"))

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	// Should NOT include empty target element
	assert.NotContains(t, output, "<target")
}

func TestAllowEmptyOutputTargetWithTranslation(t *testing.T) {
	ctx := context.Background()

	reader := txml.NewReader()
	input := `<?xml version="1.0" encoding="utf-8"?>
<txml locale="en-US" targetlocale="fr-FR" version="1.0" datatype="xml">
<header/>
<body>
<segment segtype="block">
<source>Hello</source>
<target>Bonjour</target>
</segment>
</body>
</txml>`

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write with AllowEmptyOutputTarget=false - should still include non-empty target
	writer := txml.NewWriter()
	writer.Config().AllowEmptyOutputTarget = false

	var buf bytes.Buffer
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleID("fr-FR"))

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	// Should include the actual translation
	assert.Contains(t, output, "<target>Bonjour</target>")
}
