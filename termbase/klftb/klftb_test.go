package klftb

import (
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// richConcepts builds concepts exercising the fields TBX drops: the term
// Source (brand_vocabulary), the CompetitorTerm flag, and the extensible
// Properties map — alongside the standard definition/domain/status/POS fields.
// Built in canonical form (concepts id-sorted, terms locale/text-sorted, UTC
// times) so it equals the deterministic round-trip output directly.
func richConcepts() []termbase.Concept {
	t0 := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	return []termbase.Concept{
		{
			ID: "c-1", ProjectID: "proj-1", Domain: "software",
			Definition: "the primary call to action",
			Source:     termbase.TermSourceBrandVocabulary,
			Terms: []termbase.Term{
				{Text: "Get started", Locale: "en", Status: model.TermStatus("preferred"), PartOfSpeech: "verb", Note: "imperative"},
				{Text: "Los geht's", Locale: "de", Status: model.TermStatus("approved"), Gender: "neuter"},
			},
			Properties: map[string]string{"tone": "friendly"},
			CreatedAt:  t0, UpdatedAt: t0.Add(time.Hour),
		},
		{
			ID: "c-2", Domain: "brand",
			Source: termbase.TermSourceTerminology,
			Terms: []termbase.Term{
				{Text: "CompetitorCorp", Locale: "en", Status: model.TermStatus("forbidden"), CompetitorTerm: true},
			},
			CreatedAt: t0, UpdatedAt: t0,
		},
	}
}

func TestRoundTripLossless(t *testing.T) {
	concepts := richConcepts()
	data, err := Marshal(FromConcepts(concepts))
	require.NoError(t, err)

	got, err := Unmarshal(data)
	require.NoError(t, err)
	// Marshal canonicalizes (sorts concepts + terms, UTC times); the lossless
	// invariant is that the round-trip equals that canonical form — no field
	// dropped, only deterministic reordering.
	require.Equal(t, canonicalConcepts(concepts), got.Concepts, "concepts must round-trip losslessly")
}

func TestRoundTripPreservesBrandAndCompetitorFields(t *testing.T) {
	data, err := Marshal(FromConcepts(richConcepts()))
	require.NoError(t, err)
	out := string(data)
	// Exactly the fields TBX cannot represent.
	assert.Contains(t, out, `"source": "brand_vocabulary"`)
	assert.Contains(t, out, `"competitor_term": true`)
	assert.Contains(t, out, `"tone": "friendly"`)
}

func TestMarshalIsDeterministic(t *testing.T) {
	concepts := richConcepts()
	a, err := Marshal(FromConcepts(concepts))
	require.NoError(t, err)
	reversed := []termbase.Concept{concepts[1], concepts[0]}
	b, err := Marshal(FromConcepts(reversed))
	require.NoError(t, err)
	assert.Equal(t, string(a), string(b), "Marshal must be order-independent and deterministic")
}

func TestUnmarshalRejectsBadEnvelope(t *testing.T) {
	_, err := Unmarshal([]byte(`{"schemaVersion":"1.0","kind":"wrong","concepts":[]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected kind")

	_, err = Unmarshal([]byte(`{"schemaVersion":"2.0","kind":"kapi-termbase-format","concepts":[]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported major")

	_, err = Unmarshal([]byte(`{"schemaVersion":"1.7","kind":"kapi-termbase-format","concepts":[]}`))
	require.NoError(t, err, "unknown minor accepted")
}

func TestMarshalShape(t *testing.T) {
	data, err := Marshal(FromConcepts(richConcepts()))
	require.NoError(t, err)
	out := string(data)
	assert.True(t, strings.HasSuffix(out, "}\n"))
	assert.Contains(t, out, `"kind": "kapi-termbase-format"`)
}
