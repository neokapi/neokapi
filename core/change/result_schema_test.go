package change_test

import (
	"encoding/json"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changeschema"
)

// The result schema describes what the service writes: an applied result, a
// refused one, and a change set refused as a whole each validate against it.
func TestResultSchema_DescribesTheResults(t *testing.T) {
	var s jsonschema.Schema
	require.NoError(t, json.Unmarshal(changeschema.ResultSchema(), &s))
	rs, err := s.Resolve(nil)
	require.NoError(t, err)
	record, after, blocked := "0p5tkjdkbfe5jfkns1fhzyyp", "sha256:a07d", 1
	at := &change.Ref{Doc: "docs/guide.html", Block: "p"}
	for name, res := range map[string]*change.Result{
		"applied": {Schema: change.ResultSchemaID, Status: change.SetApplied, Record: &record,
			Docs: []change.DocResult{{Doc: "docs/guide.html", Home: "file", Written: true, Before: "sha256:5e1c", After: &after,
				Findings: []change.Finding{{Rule: "terms.vocabulary", Message: "use: handbook", At: at,
					Range: &change.Resolved{Start: change.Position{Run: 0}, End: change.Position{Run: 0, Offset: 4}}, Replacement: "handbook"}}}},
			Ops: []change.OpResult{{I: 0, Op: change.KindReplaceText, Status: change.OpApplied, At: at,
				Before: "r:3f9a1c0e7b2d4a55", After: "r:c41e92d07a8b1f30",
				Resolved: []change.Resolved{{Start: change.Position{Run: 2}, End: change.Position{Run: 3}}}}}},
		"refused": {Schema: change.ResultSchemaID, Status: change.SetRefused,
			Docs: []change.DocResult{{Doc: "docs/guide.html", Home: "file", Before: "sha256:5e1c"}},
			Ops: []change.OpResult{
				{I: 0, Op: change.KindReplaceText, Status: change.OpNotApplied, At: at, BlockedBy: &blocked},
				{I: 1, Op: change.KindSetAttribute, Status: change.OpRefused, At: at,
					Error:   &change.Error{Code: change.CodeStale, Field: "if_match", Message: "changed"},
					Current: &change.Current{Rev: "r:0d71f30c75e4a087", Text: "Read"}},
			}},
		"refused whole": change.ErrorResult(&change.Error{Code: change.CodeInvalid, Pointer: "/ops", Message: "must be an array"}),
	} {
		b, err := json.Marshal(res)
		require.NoError(t, err)
		var instance any
		require.NoError(t, json.Unmarshal(b, &instance))
		assert.NoError(t, rs.Validate(instance), name)
	}
}
