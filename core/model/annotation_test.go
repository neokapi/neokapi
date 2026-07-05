package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAltTranslationJSONCamelCase pins the canonical camelCase key spelling
// (AD-034: the model's JSON casing matches protojson of neokapi.content.v1).
func TestAltTranslationJSONCamelCase(t *testing.T) {
	a := AltTranslation{
		Locale:        LocaleID("fr-FR"),
		Origin:        "tm",
		Score:         0.95,
		MatchType:     MatchFuzzy,
		CombinedScore: 0.9,
		FuzzyScore:    0.85,
		QualityScore:  0.8,
		Engine:        "acme-mt",
		ToolID:        "tm-leverage",
		AltTransType:  "proposal",
		FromOriginal:  true,
		SegmentIndex:  2,
	}
	data, err := json.Marshal(a)
	require.NoError(t, err)

	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &keys))
	for _, want := range []string{
		"matchType", "combinedScore", "fuzzyScore", "qualityScore",
		"toolId", "altTransType", "fromOriginal", "segmentIndex",
	} {
		assert.Contains(t, keys, want)
	}
	for k := range keys {
		assert.NotContains(t, k, "_", "canonical JSON keys are camelCase")
	}

	var back AltTranslation
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, a, back, "camelCase round-trip")
}

// TestAltTranslationJSONLegacySnakeCase verifies the dual-read shim: payloads
// written before the camelCase switch — and payloads from released
// okapi-bridge JARs — carry snake_case keys and must still decode.
func TestAltTranslationJSONLegacySnakeCase(t *testing.T) {
	legacy := `{
		"source": [{"text": "Hello"}],
		"target": [{"text": "Bonjour"}],
		"locale": "fr-FR",
		"origin": "tm",
		"score": 0.95,
		"match_type": "fuzzy",
		"combined_score": 0.9,
		"fuzzy_score": 0.85,
		"quality_score": 0.8,
		"engine": "acme-mt",
		"tool_id": "tm-leverage",
		"alt_trans_type": "proposal",
		"from_original": true,
		"segment_index": 2
	}`
	var a AltTranslation
	require.NoError(t, json.Unmarshal([]byte(legacy), &a))
	assert.Equal(t, MatchFuzzy, a.MatchType)
	assert.InDelta(t, 0.9, a.CombinedScore, 1e-9)
	assert.InDelta(t, 0.85, a.FuzzyScore, 1e-9)
	assert.InDelta(t, 0.8, a.QualityScore, 1e-9)
	assert.Equal(t, "tm-leverage", a.ToolID)
	assert.Equal(t, "proposal", a.AltTransType)
	assert.True(t, a.FromOriginal)
	assert.Equal(t, 2, a.SegmentIndex)
	assert.Equal(t, "Hello", RunsText(a.Source))
	assert.Equal(t, "Bonjour", RunsText(a.Target))
}

// TestAltTranslationJSONCamelWinsOverLegacy pins precedence when a document
// carries both spellings of a key.
func TestAltTranslationJSONCamelWinsOverLegacy(t *testing.T) {
	both := `{"matchType": "exact", "match_type": "fuzzy", "toolId": "new", "tool_id": "old"}`
	var a AltTranslation
	require.NoError(t, json.Unmarshal([]byte(both), &a))
	assert.Equal(t, MatchExact, a.MatchType)
	assert.Equal(t, "new", a.ToolID)
}
