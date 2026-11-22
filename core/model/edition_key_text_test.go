package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// TestEditionKeyTextIsOneToOne checks that the text form decodes to the key it
// was encoded from, a key whose parts hold the separator included, and that a
// key made of ordinary parts keeps its plain spelling.
func TestEditionKeyTextIsOneToOne(t *testing.T) {
	for _, tc := range []struct {
		key  model.EditionKey
		text string
	}{
		{model.EditionKey{Locale: "fr"}, "fr"},
		{model.EditionKey{Locale: "nb", Tone: "formal"}, "nb;tone=formal"},
		{model.EditionKey{Locale: "en", Tone: "casual", Channel: "short"}, "en;tone=casual;channel=short"},
		// Found by FuzzBlockRoundTrip: a target filed under ";" decoded as the
		// zero key, the edition the block was read in.
		{model.EditionKey{Locale: ";"}, "%3B"},
		{model.EditionKey{Locale: "fr;tone=formal"}, "fr%3Btone=formal"},
		{model.EditionKey{Locale: "%3B"}, "%253B"},
		{model.EditionKey{Locale: "fr", Channel: "a;b"}, "fr;channel=a%3Bb"},
	} {
		text, err := tc.key.MarshalText()
		require.NoError(t, err)
		assert.Equal(t, tc.text, string(text))
		var got model.EditionKey
		require.NoError(t, got.UnmarshalText(text))
		assert.Equal(t, tc.key, got, "%q decodes to the key it was encoded from", text)
	}
}
