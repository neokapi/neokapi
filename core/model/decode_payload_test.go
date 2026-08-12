package model_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// DecodePayload is the one decode every store and wire reader uses, so what it
// does with each shape of body is the contract those readers inherit.
func TestDecodePayload(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		body     string
		assert   func(t *testing.T, got model.Payload)
	}{
		{
			name:     "registered type decodes into its concrete type",
			typeName: "note",
			body:     `{"items":[{"text":"check the tone","from":"reviewer"}]}`,
			assert: func(t *testing.T, got model.Payload) {
				notes, ok := got.(*model.Notes)
				require.True(t, ok, "got %T", got)
				require.Len(t, notes.Items, 1)
				assert.Equal(t, "check the tone", notes.Items[0].Text)
			},
		},
		{
			name:     "generic body keeps the shape formats file metadata in",
			typeName: "fonts",
			body:     `{"type":"fonts","fields":{"ascii":"Calibri"}}`,
			assert: func(t *testing.T, got model.Payload) {
				ga, ok := got.(*model.GenericAnnotation)
				require.True(t, ok, "got %T", got)
				assert.Equal(t, "fonts", ga.Kind)
				assert.Equal(t, "Calibri", ga.Fields["ascii"])
			},
		},
		{
			name:     "generic body without a type name takes the key's",
			typeName: "fonts",
			body:     `{"fields":{"ascii":"Calibri"}}`,
			assert: func(t *testing.T, got model.Payload) {
				ga, ok := got.(*model.GenericAnnotation)
				require.True(t, ok, "got %T", got)
				assert.Equal(t, "fonts", ga.Kind)
			},
		},
		{
			name:     "an unregistered schema survives verbatim",
			typeName: "qa",
			body:     `{"findings":[{"rule":"punctuation"}],"score":91}`,
			assert: func(t *testing.T, got model.Payload) {
				raw, ok := got.(*model.RawAnnotation)
				require.True(t, ok, "got %T", got)
				assert.Equal(t, "qa", raw.TypeName())
				assert.JSONEq(t, `{"findings":[{"rule":"punctuation"}],"score":91}`, string(raw.Body))
			},
		},
		{
			name:     "a body that is not an object survives verbatim",
			typeName: "vendor.counts",
			body:     `[1,2,3]`,
			assert: func(t *testing.T, got model.Payload) {
				raw, ok := got.(*model.RawAnnotation)
				require.True(t, ok, "got %T", got)
				assert.JSONEq(t, `[1,2,3]`, string(raw.Body))
			},
		},
		{
			name:     "an empty object carries no discriminator and stays as written",
			typeName: "qa",
			body:     `{}`,
			assert: func(t *testing.T, got model.Payload) {
				raw, ok := got.(*model.RawAnnotation)
				require.True(t, ok, "got %T", got)
				assert.JSONEq(t, `{}`, string(raw.Body))
			},
		},
		{
			name:     "a registered type whose body does not fit is kept rather than emptied",
			typeName: "note",
			body:     `{"items":"not a list"}`,
			assert: func(t *testing.T, got model.Payload) {
				raw, ok := got.(*model.RawAnnotation)
				require.True(t, ok, "got %T", got)
				assert.JSONEq(t, `{"items":"not a list"}`, string(raw.Body))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.assert(t, model.DecodePayload(tt.typeName, []byte(tt.body)))
		})
	}
}

// A decoded payload re-encodes to what arrived, so a reader that cannot name a
// type still hands the body back to the writer that can.
func TestDecodePayloadRoundTrip(t *testing.T) {
	for _, body := range []string{
		`{"findings":[{"rule":"punctuation","severity":"info"}],"score":91}`,
		`{"items":[{"text":"check the tone","from":"reviewer"}]}`,
		`{"type":"fonts","fields":{"ascii":"Calibri"}}`,
		`{}`,
		`[1,2,3]`,
	} {
		t.Run(body, func(t *testing.T) {
			got, err := json.Marshal(model.DecodePayload("qa", []byte(body)))
			require.NoError(t, err)
			assert.JSONEq(t, body, string(got))
		})
	}
}
