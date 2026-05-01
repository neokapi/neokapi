package txml_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/formats/txml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
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

const sourceOnlyForConfig = `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Source only text</source></segment></translatable>
</txml>`

const bilingualForConfig = `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello</source><target>Bonjour</target></segment></translatable>
</txml>`

func TestAllowEmptyOutputTargetEnabled(t *testing.T) {
	ctx := t.Context()

	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sourceOnlyForConfig, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write with AllowEmptyOutputTarget=true (default).
	writer := txml.NewWriter()
	writer.Config().AllowEmptyOutputTarget = true

	var buf bytes.Buffer
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleID("fr"))

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "<target/>")
}

func TestAllowEmptyOutputTargetDisabled(t *testing.T) {
	ctx := t.Context()

	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sourceOnlyForConfig, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	writer := txml.NewWriter()
	writer.Config().AllowEmptyOutputTarget = false

	var buf bytes.Buffer
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleID("fr"))

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.NotContains(t, output, "<target")
}

func TestAllowEmptyOutputTargetWithTranslation(t *testing.T) {
	ctx := t.Context()

	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(bilingualForConfig, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	writer := txml.NewWriter()
	writer.Config().AllowEmptyOutputTarget = false

	var buf bytes.Buffer
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleID("fr"))

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "<target>Bonjour</target>")
}
