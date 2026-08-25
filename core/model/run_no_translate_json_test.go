package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A protected run has to survive serialization, because the flag is read a long
// way from where it is set. The reader marks a code span; the parse cache and
// the block store both round-trip the block through JSON; the tools read the
// flag afterwards. Drop it in the middle and everything still runs — the
// command is simply translated, which is how `kapi check --ship` reached the
// docs site as `ķàþî çĥéçķ --šĥîþ`.

func TestTextRunNoTranslateRoundTrips(t *testing.T) {
	in := []Run{
		{Text: &TextRun{Text: "Run "}},
		{PcOpen: &PcOpenRun{ID: "1", Type: "fmt:code", SubType: "md:code"}},
		{Text: &TextRun{Text: "kapi check --ship", NoTranslate: true}},
		{PcClose: &PcCloseRun{ID: "1", Type: "fmt:code", SubType: "md:code"}},
		{Text: &TextRun{Text: " to verify."}},
	}

	raw, err := json.Marshal(in)
	require.NoError(t, err)

	var out []Run
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Len(t, out, len(in))

	assert.True(t, out[2].Text.NoTranslate, "the protected run lost its marking")
	assert.Equal(t, "kapi check --ship", out[2].Text.Text)
	assert.False(t, out[0].Text.NoTranslate, "ordinary prose must stay translatable")
	assert.False(t, out[4].Text.NoTranslate)
}

// The flag is written only when set, so every document without a protected run
// serializes exactly as it did before the field existed. This is what keeps the
// change additive for stored blocks and for the TypeScript mirror.
func TestPlainTextRunWireFormUnchanged(t *testing.T) {
	raw, err := json.Marshal([]Run{{Text: &TextRun{Text: "ordinary prose"}}})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"text":"ordinary prose"}]`, string(raw))

	raw, err = json.Marshal([]Run{{Text: &TextRun{Text: "kapi up", NoTranslate: true}}})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"text":"kapi up","noTranslate":true}]`, string(raw))
}

// `noTranslate` sits beside `text` rather than nested under it, so the decoder
// must not mistake it for a second discriminator — and must apply it whatever
// order the JSON object's keys arrive in.
func TestNoTranslateIsNotADiscriminator(t *testing.T) {
	for _, body := range []string{
		`{"text":"kapi up","noTranslate":true}`,
		`{"noTranslate":true,"text":"kapi up"}`,
	} {
		var r Run
		require.NoError(t, json.Unmarshal([]byte(body), &r), body)
		require.NotNil(t, r.Text, body)
		assert.True(t, r.Text.NoTranslate, body)
	}

	// It carries no meaning on a run that is not text, and must not resurrect
	// one that has no discriminator at all.
	var r Run
	assert.Error(t, json.Unmarshal([]byte(`{"noTranslate":true}`), &r))
}
