package kpz

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleOverlays() []OverlayDoc {
	return []OverlayDoc{
		{Kind: "targets/fr-FR", BlockHash: "h2", Payload: json.RawMessage(`{"text":"Salut"}`)},
		{Kind: "targets/fr-FR", BlockHash: "h1", Payload: json.RawMessage(`{"text":"Bonjour"}`)},
		{Kind: "annotations/qa", BlockHash: "h1", Payload: json.RawMessage(`{"issues":[]}`)},
	}
}

// TestOverlaysRoundTrip verifies the overlays member round-trips through a
// pack/unpack, sorted deterministically by (kind, blockHash).
func TestOverlaysRoundTrip(t *testing.T) {
	pkg := &Package{Created: "2026-06-04T00:00:00Z", Overlays: sampleOverlays()}

	data, err := pkg.Marshal()
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)

	require.Len(t, got.Overlays, 3)
	assert.Equal(t, "annotations/qa", got.Overlays[0].Kind)
	assert.Equal(t, "targets/fr-FR", got.Overlays[1].Kind)
	assert.Equal(t, "h1", got.Overlays[1].BlockHash)
	assert.Equal(t, "targets/fr-FR", got.Overlays[2].Kind)
	assert.Equal(t, "h2", got.Overlays[2].BlockHash)
	assert.JSONEq(t, `{"text":"Bonjour"}`, string(got.Overlays[1].Payload))
}

// TestOverlaysDeterministic verifies a given overlay set packs to byte-stable
// bytes regardless of input order or payload spacing.
func TestOverlaysDeterministic(t *testing.T) {
	a := &Package{Overlays: []OverlayDoc{
		{Kind: "targets/fr", BlockHash: "h1", Payload: json.RawMessage(`{"text":"Bonjour","provider":"pseudo"}`)},
		{Kind: "targets/fr", BlockHash: "h2", Payload: json.RawMessage(`{"text":"Salut"}`)},
	}}
	b := &Package{Overlays: []OverlayDoc{
		{Kind: "targets/fr", BlockHash: "h2", Payload: json.RawMessage(`{ "text": "Salut" }`)},
		{Kind: "targets/fr", BlockHash: "h1", Payload: json.RawMessage("{\n  \"provider\": \"pseudo\",\n  \"text\": \"Bonjour\"\n}")},
	}}

	da, err := a.Marshal()
	require.NoError(t, err)
	db, err := b.Marshal()
	require.NoError(t, err)
	assert.Equal(t, da, db, "same overlay set must pack to identical bytes")
}

func rootOf(t *testing.T, p *Package) string {
	t.Helper()
	members, err := p.serializeMembers()
	require.NoError(t, err)
	return rootHash(members)
}

// TestHistoryExcludedFromRootHash verifies the advisory history log is
// content-subordinate: it does not affect the package's content identity
// (RootHash), and deleting it leaves the content unchanged.
func TestHistoryExcludedFromRootHash(t *testing.T) {
	base := &Package{Overlays: sampleOverlays()}
	withHistory := &Package{
		Overlays: sampleOverlays(),
		History: AppendHistory(AppendHistory(nil,
			HistoryEvent{Timestamp: "2026-06-04T10:00:00Z", Event: "pack", Note: "demo.kapi"}),
			HistoryEvent{Timestamp: "2026-06-04T10:05:00Z", Event: "pack", Note: "demo.kapi"}),
	}

	assert.Equal(t, rootOf(t, base), rootOf(t, withHistory),
		"history must not change the content RootHash")

	data, err := withHistory.Marshal()
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, withHistory.History, got.History)
	lines := strings.Split(strings.TrimSpace(string(got.History)), "\n")
	assert.Len(t, lines, 2)
	assert.Contains(t, lines[0], `"event":"pack"`)

	got.History = nil
	assert.Equal(t, rootOf(t, base), rootOf(t, got))
}

// TestHistoryHashChain verifies the advisory log is a tamper-evident hash
// chain: an intact log verifies, and any edit, reorder, insertion, or
// deletion of a past line is detected.
func TestHistoryHashChain(t *testing.T) {
	log := AppendHistory(nil, HistoryEvent{Timestamp: "t1", Event: "pack", Note: "a"})
	log = AppendHistory(log, HistoryEvent{Timestamp: "t2", Event: "pack", Note: "b"})
	log = AppendHistory(log, HistoryEvent{Timestamp: "t3", Event: "pack", Note: "c"})

	require.NoError(t, VerifyHistory(log))
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	require.Len(t, lines, 3)
	var first, second HistoryEvent
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &second))
	assert.Empty(t, first.Prev, "genesis line has no prev")
	assert.NotEmpty(t, first.Hash)
	assert.Equal(t, first.Hash, second.Prev, "each line links to the previous hash")

	tampered := strings.Replace(string(log), `"note":"b"`, `"note":"HACKED"`, 1)
	require.Error(t, VerifyHistory([]byte(tampered)), "an edited line must fail verification")

	dropped := lines[0] + "\n" + lines[2] + "\n"
	require.Error(t, VerifyHistory([]byte(dropped)), "a removed line must break the chain")

	require.NoError(t, VerifyHistory(nil))
}

// TestHistoryChainSurvivesInvalidUTF8 pins #1608: a log AppendHistory wrote must
// verify, whatever bytes the event fields carried.
//
// It did not. The digest was computed over the in-memory Go string while
// encoding/json wrote U+FFFD in place of every invalid byte, so VerifyHistory
// recomputed the hash over different bytes and reported a log we had just
// written as TAMPERED. A tamper-evidence signal that fires on benign input is
// worse than no signal, and the reachable path is mundane: Step and Note carry
// filenames, and a filename on Linux is an arbitrary byte string.
func TestHistoryChainSurvivesInvalidUTF8(t *testing.T) {
	for name, bad := range map[string]string{
		"lone continuation byte": "\xb1",
		"truncated 2-byte rune":  "a\xc3",
		"truncated 3-byte rune":  "a\xe2\x82b",
		"bare 0xff":              "\xff\xfe",
		"valid text unaffected":  "ordinary note",
	} {
		t.Run(name, func(t *testing.T) {
			log := AppendHistory(nil, HistoryEvent{Timestamp: "t1", Event: "pack", Step: bad, Note: bad})
			log = AppendHistory(log, HistoryEvent{Timestamp: "t2", Event: "pack", Step: bad, Note: bad})
			require.NoError(t, VerifyHistory(log), "a chain AppendHistory built must pass its own verifier")

			// The chain must still be tamper-EVIDENT after the coercion — the
			// fix must not have flattened the fields into something a forger
			// can edit freely.
			lines := strings.Split(strings.TrimSpace(string(log)), "\n")
			require.Len(t, lines, 2)
			var ev HistoryEvent
			require.NoError(t, json.Unmarshal([]byte(lines[0]), &ev))
			assert.True(t, utf8.ValidString(ev.Step), "the written field is valid UTF-8")
			assert.True(t, utf8.ValidString(ev.Note), "the written field is valid UTF-8")

			edited := strings.Replace(string(log), `"event":"pack"`, `"event":"HACKED"`, 1)
			require.Error(t, VerifyHistory([]byte(edited)), "an edited line must still fail verification")
		})
	}
}

func TestOverlaySetRejectsBadEnvelope(t *testing.T) {
	_, err := unmarshalOverlaySet([]byte(`{"schemaVersion":"1.0","kind":"wrong","overlays":[]}`))
	require.Error(t, err)

	_, err = unmarshalOverlaySet([]byte(`{"schemaVersion":"9.0","kind":"kapi-overlay-set","overlays":[]}`))
	require.Error(t, err)
}
