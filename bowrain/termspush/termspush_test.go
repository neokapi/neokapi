package termspush

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/graph"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

func TestValidate(t *testing.T) {
	approved := []apiclient.TermInfo{{Text: "Sign up", Locale: "en", Status: "approved"}}
	for _, tc := range []struct {
		name     string
		edits    *apiclient.TermsEdits
		governed bool
		invalid  bool
	}{
		{name: "nothing to check", edits: nil},
		{name: "an ordinary create", edits: &apiclient.TermsEdits{Creates: []apiclient.CreateConceptParams{{Terms: approved}}}},
		{
			name:     "a create with a forbidden term",
			edits:    &apiclient.TermsEdits{Creates: []apiclient.CreateConceptParams{{Terms: []apiclient.TermInfo{{Text: "x", Locale: "en", Status: "forbidden"}}}}},
			governed: true,
		},
		{
			name:     "a create with the do-not-translate flag",
			edits:    &apiclient.TermsEdits{Creates: []apiclient.CreateConceptParams{{DoNotTranslate: true, Terms: approved}}},
			governed: true,
		},
		{
			name:     "a REPLACED_BY relation on the direct path",
			edits:    &apiclient.TermsEdits{RelationAdds: []apiclient.RelationEdit{{SourceID: "a", Relation: apiclient.AddRelationParams{TargetID: "b", RelationType: graph.LabelReplacedBy}}}},
			governed: true,
		},
		{
			name:    "a relation with no target",
			edits:   &apiclient.TermsEdits{RelationAdds: []apiclient.RelationEdit{{SourceID: "a"}}},
			invalid: true,
		},
		{
			name:    "a concept edit naming no concept",
			edits:   &apiclient.TermsEdits{Updates: []apiclient.ConceptEdit{{}}},
			invalid: true,
		},
		{
			name: "a malformed proposed operation",
			edits: &apiclient.TermsEdits{Proposal: &apiclient.TermsProposal{
				Ops: []apiclient.ChangeSetOpInput{{Op: "concept.delete", Payload: json.RawMessage(`{}`)}},
			}},
			invalid: true,
		},
		{
			name: "a well-formed proposal",
			edits: &apiclient.TermsEdits{Proposal: &apiclient.TermsProposal{
				Ops: []apiclient.ChangeSetOpInput{{Op: "concept.delete", Payload: json.RawMessage(`{"concept_id":"c1"}`)}},
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.edits)
			switch {
			case tc.governed:
				require.ErrorIs(t, err, ErrGoverned)
			case tc.invalid:
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrGoverned)
			default:
				require.NoError(t, err)
			}
		})
	}
}
