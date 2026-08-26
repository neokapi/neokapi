package kmb

import (
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// richEntries builds content-memory entries exercising every lossless field: multilingual
// variants with inline runs, entity mappings with a terms store ConceptID
// cross-link, provenance origins, per-entry properties, and a note. These are
// exactly the fields TMX cannot represent.
func richEntries() ([]memory.Entry, []memory.ImportSession) {
	t0 := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	sessions := []memory.ImportSession{{
		ID: "sess-1", FileKey: "errors.tmx", FileHash: "sha256:abc", FileSizeBytes: 2048,
		ImportedAt: t0, ImportedBy: "alice", ToolName: "neokapi", ToolVersion: "1.0",
		SegType: "sentence", SrcLang: "en", DataType: "PlainText", EntryCount: 2,
		Properties: map[string]string{"batch": "q2"},
	}}
	entries := []memory.Entry{
		{
			ID:          "tm-1",
			ProjectID:   "proj-1",
			HintSrcLang: "en",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: "Hello "}}, {Ph: &model.PlaceholderRun{ID: "1", Type: "var", Data: "{name}", Equiv: "name"}}},
				"fr": {{Text: &model.TextRun{Text: "Bonjour "}}, {Ph: &model.PlaceholderRun{ID: "1", Type: "var", Data: "{name}", Equiv: "name"}}},
			},
			Entities: []memory.EntityMapping{{
				PlaceholderID: "e1", Type: model.EntityType("person"), ConceptID: "concept-42",
				Values: map[model.LocaleID]memory.EntityValue{
					"en": {Text: "Ada", Start: 6, End: 9},
					"fr": {Text: "Ada", Start: 8, End: 11},
				},
			}},
			Properties: map[string]string{"domain": "greeting"},
			Origins: []memory.Origin{
				{Source: "import", Key: "greeting.hello", Reference: "job-9", AddedAt: t0, AddedBy: "alice", SessionID: "sess-1"},
			},
			Note:      "reviewed",
			CreatedAt: t0,
			UpdatedAt: t0.Add(time.Hour),
		},
		{
			ID:          "tm-2",
			HintSrcLang: "en",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: "Goodbye"}}},
				"de": {{Text: &model.TextRun{Text: "Auf Wiedersehen"}}},
			},
			CreatedAt: t0,
			UpdatedAt: t0,
		},
	}
	return entries, sessions
}

func TestRoundTripLossless(t *testing.T) {
	entries, sessions := richEntries()
	data, err := Marshal(FromModel(entries, sessions))
	require.NoError(t, err)

	got, err := Unmarshal(data)
	require.NoError(t, err)

	// Entries are sorted by id on marshal; our fixture is already id-sorted.
	require.Equal(t, entries, got.ModelEntries(), "content-memory entries must round-trip byte-faithfully")
	require.Equal(t, sessions, got.ModelImportSessions(), "import sessions must round-trip")
}

func TestRoundTripPreservesConceptCrossLink(t *testing.T) {
	entries, _ := richEntries()
	data, err := Marshal(FromModel(entries, nil))
	require.NoError(t, err)
	require.Contains(t, string(data), `"conceptId": "concept-42"`, "terms cross-link must survive (TMX drops this)")

	got, err := Unmarshal(data)
	require.NoError(t, err)
	require.Equal(t, "concept-42", got.ModelEntries()[0].Entities[0].ConceptID)
}

func TestMarshalIsDeterministic(t *testing.T) {
	entries, sessions := richEntries()
	a, err := Marshal(FromModel(entries, sessions))
	require.NoError(t, err)

	// Reversed input order must produce identical bytes (entries sort by id).
	reversed := []memory.Entry{entries[1], entries[0]}
	b, err := Marshal(FromModel(reversed, sessions))
	require.NoError(t, err)
	assert.Equal(t, string(a), string(b), "Marshal must be order-independent and deterministic")
}

func TestMarshalNoHTMLEscape(t *testing.T) {
	entries := []memory.Entry{{
		ID:       "x",
		Variants: map[model.LocaleID][]model.Run{"en": {{Text: &model.TextRun{Text: "a < b && c"}}}},
	}}
	data, err := Marshal(FromModel(entries, nil))
	require.NoError(t, err)
	assert.Contains(t, string(data), "a < b && c")
	assert.NotContains(t, string(data), `u003c`)
	assert.NotContains(t, string(data), `u0026`)
}

func TestUnmarshalRejectsBadEnvelope(t *testing.T) {
	require.NoError(t, func() error {
		_, e := Unmarshal([]byte(`{"schemaVersion":"1.0","kind":"kapi-memory","entries":[]}`))
		return e
	}())

	_, err := Unmarshal([]byte(`{"schemaVersion":"1.0","kind":"wrong","entries":[]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected kind")

	_, err = Unmarshal([]byte(`{"schemaVersion":"2.0","kind":"kapi-memory","entries":[]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported major")

	_, err = Unmarshal([]byte(`{"schemaVersion":"x","kind":"kapi-memory","entries":[]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid schemaVersion")
}

func TestUnknownMinorAccepted(t *testing.T) {
	_, err := Unmarshal([]byte(`{"schemaVersion":"1.99","kind":"kapi-memory","entries":[]}`))
	require.NoError(t, err, "unknown minor of a known major must be accepted")
}

func TestMarshalShape(t *testing.T) {
	entries, _ := richEntries()
	data, err := Marshal(FromModel(entries, nil))
	require.NoError(t, err)
	out := string(data)
	assert.True(t, strings.HasPrefix(out, "{\n"), "indented")
	assert.True(t, strings.HasSuffix(out, "}\n"), "trailing newline")
	assert.Contains(t, out, `"kind": "kapi-memory"`)
}

// TestBundleCarriesPointUnitAndGovernance guards the round trip the version
// chain rests on. The committed bundle is the truth and the store is its
// projection, so anything the bundle drops is gone on the next fresh clone.
//
// Point did not survive before this: an entry exported and re-seeded came back
// approved nowhere, which reads to the matcher as an ad-hoc addition and
// quietly disables the disambiguation the point exists to provide. Unit and the
// governing fingerprint would have been lost the same way — and a version whose
// governance is unknown cannot be judged, only used.
func TestBundleCarriesPointUnitAndGovernance(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	in := memory.Entry{
		ID:          "v1",
		Point:       memory.NewPoint("acme", "support", "acme-help"),
		Unit:        "u-1",
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Get started"}}},
			"nb": {{Text: &model.TextRun{Text: "Kom i gang"}}},
		},
		Origins: []memory.Origin{{
			Source:             "tool",
			AddedAt:            at,
			ContextFingerprint: "fp-now",
		}},
		CreatedAt: at,
		UpdatedAt: at,
	}

	data, err := Marshal(FromModel([]memory.Entry{in}, nil))
	require.NoError(t, err)
	file, err := Unmarshal(data)
	require.NoError(t, err)
	out := file.ModelEntries()
	require.Len(t, out, 1)

	assert.Equal(t, in.Point, out[0].Point, "the point an answer was approved at")
	assert.Equal(t, in.Unit, out[0].Unit, "the block it was approved for")
	require.Len(t, out[0].Origins, 1)
	assert.Equal(t, "fp-now", out[0].Origins[0].ContextFingerprint, "what governed it")
}
