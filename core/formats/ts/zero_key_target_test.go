package ts_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noLanguageTS has no language attribute, so a reader given no source locale
// files its translation under the empty locale.
const noLanguageTS = `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0">
<context>
    <name>Ctx</name>
    <message>
        <source>Hello</source>
        <translation>Hallo</translation>
    </message>
</context>
</TS>
`

// writeNoLanguage reads noLanguageTS with no source locale, lets edit change
// the block, and writes the parts without a skeleton and without a locale.
func writeNoLanguage(t *testing.T, edit func(*model.Block)) (string, error) {
	t.Helper()
	ctx := t.Context()
	reader := ts.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(noLanguageTS, "")))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	require.Equal(t, "Hallo", model.RunsText(blocks[0].TargetRuns("")), "the reader files the translation under the empty locale")
	edit(blocks[0])

	var buf bytes.Buffer
	writer := ts.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	err := writer.Write(ctx, testutil.PartsToChannel(parts))
	writer.Close()
	return buf.String(), err
}

// The writer writes the translation filed under the empty locale, so the XML
// check guards it as it guards every other translation.
func TestWriter_ZeroKeyTargetIsCheckedAsWritten(t *testing.T) {
	out, err := writeNoLanguage(t, func(*model.Block) {})
	require.NoError(t, err)
	assert.Contains(t, out, "<translation>Hallo</translation>")

	_, err = writeNoLanguage(t, func(b *model.Block) { b.SetTargetText("", "Hal\x01lo") })
	var uv *format.UnrepresentableValueError
	require.ErrorAs(t, err, &uv)
	require.EqualError(t, err, "ts writer: block tu1 (Ctx/Hello): U+0001 at byte 3 cannot be represented in XML: XML 1.0 has no escape and no character reference for it")
	assert.Empty(t, uv.Locale, "a translation under the empty locale is reported with no locale")
}
