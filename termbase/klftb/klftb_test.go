package klftb

import (
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/graph"
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

// richRelations builds relations exercising the optional fields: a note, a
// market-tagged validity, and a time-bounded validity authored in a non-UTC
// zone (so canonicalization has something to normalize). Built id-sorted so
// the UTC-normalized form equals the deterministic round-trip output.
func richRelations() []termbase.ConceptRelation {
	t0 := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	oslo := time.FixedZone("Europe/Oslo", 2*60*60)
	from := time.Date(2026, 7, 1, 12, 0, 0, 0, oslo)
	return []termbase.ConceptRelation{
		{
			ID: "r-1", SourceID: "c-2", TargetID: "c-1",
			RelationType: graph.LabelUseInstead, Note: "renamed at launch",
			Validity:  &graph.Validity{Tags: map[string]string{"market": "dach"}},
			CreatedAt: t0,
		},
		{
			ID: "r-2", SourceID: "c-1", TargetID: "c-2",
			RelationType: graph.LabelCompetitor,
			Validity:     &graph.Validity{ValidFrom: &from},
			CreatedAt:    t0.In(oslo),
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

func TestRoundTripWithRelations(t *testing.T) {
	file := FromConcepts(richConcepts())
	file.Relations = richRelations()
	data, err := Marshal(file)
	require.NoError(t, err)

	got, err := Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion, got.SchemaVersion)
	require.Equal(t, canonicalRelations(richRelations()), got.Relations, "relations must round-trip losslessly")

	// Byte-stable: re-encoding the decoded file reproduces the exact bytes.
	again, err := Marshal(got)
	require.NoError(t, err)
	assert.Equal(t, string(data), string(again), "relations round-trip must be byte-identical")

	// The note, the market tag, and the UTC-normalized bound all survive.
	out := string(data)
	assert.Contains(t, out, `"relation_type": "USE_INSTEAD"`)
	assert.Contains(t, out, `"note": "renamed at launch"`)
	assert.Contains(t, out, `"market": "dach"`)
	assert.Contains(t, out, `"valid_from": "2026-07-01T10:00:00Z"`, "validity bounds normalize to UTC")
}

func TestMarshalRelationsDeterministic(t *testing.T) {
	rels := richRelations()
	fileA := FromConcepts(richConcepts())
	fileA.Relations = rels
	a, err := Marshal(fileA)
	require.NoError(t, err)

	fileB := FromConcepts(richConcepts())
	fileB.Relations = []termbase.ConceptRelation{rels[1], rels[0]}
	b, err := Marshal(fileB)
	require.NoError(t, err)
	assert.Equal(t, string(a), string(b), "relation order must not affect the bytes")

	// No relations: nil and empty marshal identically, with the key omitted.
	fileNil := FromConcepts(richConcepts())
	c, err := Marshal(fileNil)
	require.NoError(t, err)
	fileEmpty := FromConcepts(richConcepts())
	fileEmpty.Relations = []termbase.ConceptRelation{}
	d, err := Marshal(fileEmpty)
	require.NoError(t, err)
	assert.Equal(t, string(c), string(d))
	assert.NotContains(t, string(c), `"relations"`)
}

func TestUnmarshalAcceptsNoRelationsKey(t *testing.T) {
	// A document with no relations — the array is omitted, not empty.
	fixture := `{
  "schemaVersion": "1.0",
  "kind": "kapi-termbase-format",
  "concepts": [
    {
      "id": "c-1",
      "domain": "software",
      "terms": [
        {"text": "Get started", "locale": "en", "status": "preferred"}
      ],
      "created_at": "2026-06-01T09:00:00Z",
      "updated_at": "2026-06-01T09:00:00Z"
    }
  ]
}
`
	got, err := Unmarshal([]byte(fixture))
	require.NoError(t, err)
	assert.Equal(t, "1.0", got.SchemaVersion)
	require.Len(t, got.Concepts, 1)
	assert.Equal(t, "c-1", got.Concepts[0].ID)
	assert.Nil(t, got.Relations)
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
