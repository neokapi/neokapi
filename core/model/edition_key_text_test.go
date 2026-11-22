package model_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// editionKeyVector is one entry of testdata/edition_key_text.json: a key and
// the text MarshalText writes for it. packages/kapi-format reads the same file,
// so the Go and TypeScript printers and parsers agree on one list.
type editionKeyVector struct {
	Locale  string `json:"locale"`
	Tone    string `json:"tone"`
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

func editionKeyVectors(t *testing.T) []editionKeyVector {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "edition_key_text.json"))
	require.NoError(t, err)
	var vectors []editionKeyVector
	require.NoError(t, json.Unmarshal(data, &vectors))
	require.NotEmpty(t, vectors)
	return vectors
}

// TestEditionKeyTextIsOneToOne checks that the text form decodes to the key it
// was encoded from, a key whose parts hold the escaped characters included,
// and that a key made of ordinary parts keeps its plain spelling. The first
// such key, a target filed under ";", was found by FuzzBlockRoundTrip.
func TestEditionKeyTextIsOneToOne(t *testing.T) {
	for _, v := range editionKeyVectors(t) {
		key := model.EditionKey{Locale: model.LocaleID(v.Locale), Tone: v.Tone, Channel: v.Channel}
		text, err := key.MarshalText()
		require.NoError(t, err)
		assert.Equal(t, v.Text, string(text))
		var got model.EditionKey
		require.NoError(t, got.UnmarshalText(text))
		assert.Equal(t, key, got, "UnmarshalText(%q)", text)
	}
}

// TestParseEditionKeyReadsTheEscapedForm checks that the strict parser reads
// the dimensions the printer escaped. Its language must be a locale, so only
// the vectors with one are read here.
func TestParseEditionKeyReadsTheEscapedForm(t *testing.T) {
	read := 0
	for _, v := range editionKeyVectors(t) {
		if canon, err := model.CanonicalLocale(v.Locale); err != nil || string(canon) != v.Locale {
			continue
		}
		read++
		got, err := model.ParseEditionKey(v.Text)
		require.NoError(t, err)
		assert.Equal(t, model.EditionKey{Locale: model.LocaleID(v.Locale), Tone: v.Tone, Channel: v.Channel}, got, "ParseEditionKey(%q)", v.Text)
	}
	assert.GreaterOrEqual(t, read, 4)
}
