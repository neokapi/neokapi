package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// The translations table holds every stored row's target_json in one shape,
// and the status projections address its status key in SQL. An edition encodes
// to exactly those bytes, so rows written before and after read alike.
func TestTargetJSON_Shape(t *testing.T) {
	tests := []struct {
		name string
		e    model.Edition
		want string
	}{
		{
			name: "every field",
			e: model.Edition{
				Runs:   []model.Run{model.TextR("Hallo "), {Ph: &model.PlaceholderRun{ID: "p1", Equiv: "{name}"}}},
				Status: model.Status(model.TargetStatusTranslated),
				Origin: model.Origin{Kind: model.OriginMT, Engine: "deepl"},
				Score:  0.87,
			},
			want: `{"runs":[{"text":"Hallo "},{"ph":{"id":"p1","type":"","data":"","equiv":"{name}"}}],"status":"translated","origin":{"kind":"mt","engine":"deepl"},"score":0.87}`,
		},
		{name: "no runs", e: model.Edition{}, want: `{"runs":null}`},
		{name: "empty runs", e: model.Edition{Runs: []model.Run{}}, want: `{"runs":[]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MarshalTargetJSON(tc.e)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))

			back, err := UnmarshalTargetJSON(got)
			require.NoError(t, err)
			assert.Equal(t, tc.e, back)
		})
	}
}

// A stored row decodes as json.Unmarshal reads it: null is the zero edition,
// a key the shape does not name is ignored, and bytes that are not JSON fail.
func TestUnmarshalTargetJSON(t *testing.T) {
	e, err := UnmarshalTargetJSON([]byte(`null`))
	require.NoError(t, err)
	assert.Equal(t, model.Edition{}, e)

	e, err = UnmarshalTargetJSON([]byte(`{"runs":[{"text":"Bonjour"}],"status":"established","extra":1}`))
	require.NoError(t, err)
	assert.Equal(t, "Bonjour", model.RunsText(e.Runs))
	assert.Equal(t, model.Status(model.TargetStatusEstablished), e.Status)

	for _, bad := range []string{``, `{`, `"text"`} {
		_, err := UnmarshalTargetJSON([]byte(bad))
		assert.Error(t, err, "%q", bad)
	}
}
