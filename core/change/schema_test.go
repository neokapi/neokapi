package change_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

var update = flag.Bool("update", false, "rewrite the schema golden file")

// The schema is generated from the Go types and pinned: a change to it is a
// change to the contract, and shows up in review as a change to this file.
func TestSchemaGolden(t *testing.T) {
	got := change.Schema()
	path := filepath.Join("testdata", "schema.golden.json")
	if *update {
		require.NoError(t, os.WriteFile(path, append(got, '\n'), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run go test ./core/change -run TestSchemaGolden -update to create it")
	assert.Equal(t, string(bytes.TrimSpace(want)), string(got))
}

// Every operation is a oneOf member with a const discriminator and no other
// properties allowed.
func TestSchema_EachOperationIsAClosedOneOfMember(t *testing.T) {
	var s struct {
		Properties map[string]struct {
			Items struct {
				OneOf []struct {
					Properties map[string]struct {
						Const string `json:"const"`
					} `json:"properties"`
					AdditionalProperties *bool    `json:"additionalProperties"`
					Required             []string `json:"required"`
				} `json:"oneOf"`
			} `json:"items"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(change.Schema(), &s))
	members := s.Properties["ops"].Items.OneOf
	var kinds []string
	for _, m := range members {
		kinds = append(kinds, m.Properties["op"].Const)
		require.NotNil(t, m.AdditionalProperties, m.Properties["op"].Const)
		assert.False(t, *m.AdditionalProperties, "%s allows no other properties", m.Properties["op"].Const)
		assert.Contains(t, m.Required, "op")
	}
	want := make([]string, 0, len(change.Kinds()))
	for _, k := range change.Kinds() {
		want = append(want, string(k))
	}
	assert.Equal(t, want, kinds)
	assert.NotContains(t, kinds, string(change.KindProvenance), "the in-process provenance operation is not on the wire")
}

func resolvedSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	var s jsonschema.Schema
	require.NoError(t, json.Unmarshal(change.Schema(), &s))
	rs, err := s.Resolve(nil)
	require.NoError(t, err)
	return rs
}

// What the decoder accepts, the schema accepts, and the other way round, for
// the examples of the contract.
func TestSchema_AgreesWithTheDecoder(t *testing.T) {
	rs := resolvedSchema(t)
	for _, tc := range decodeCases() {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.object {
				t.Skip("not a single JSON object")
			}
			var instance any
			require.NoError(t, json.Unmarshal([]byte(tc.in), &instance))
			err := rs.Validate(instance)
			if tc.pointer == "" {
				assert.NoError(t, err, "the schema accepts what the decoder accepts")
			} else if !tc.semantic {
				assert.Error(t, err, "the schema refuses what the decoder refuses at %s", tc.pointer)
			}
		})
	}
}
