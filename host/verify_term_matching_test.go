package host

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerify_TerminologyRecordsItsMatchingMode pins that the terminology gate
// says, per target language, how it matched: the English source is read with
// its regular inflections, and the target by containment, with declared forms
// once a rule has them.
func TestVerify_TerminologyRecordsItsMatchingMode(t *testing.T) {
	t.Run("containment only", func(t *testing.T) {
		root, _ := writeVerifyProject(t)
		t.Chdir(root)

		out, _ := runVerifyJSON(t)
		gate, ok := gateByName(out, gateTerms)
		require.True(t, ok, "terminology gate must be present")
		require.NotNil(t, gate.Execution)
		assert.Equal(t, []check.TermMatching{{
			Locale: "fr", Source: check.TermSourceEnglishInflection, Target: check.TermTargetContainment, Rules: 1,
		}}, gate.Execution.TermMatching)
	})

	t.Run("containment and declared forms", func(t *testing.T) {
		root, _ := writeVerifyProject(t)
		seedProjectStore(t, root, func(db *projectdb.DB) {
			require.NoError(t, db.Terms().AddConcept(t.Context(), terms.Concept{
				ID: "c1",
				Terms: []terms.Term{
					{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
					{Text: "Enregistrer", Locale: model.LocaleFrench, Status: model.TermPreferred, Forms: []string{"Enregistrez"}},
				},
			}))
		})
		t.Chdir(root)

		out, _ := runVerifyJSON(t)
		gate, ok := gateByName(out, gateTerms)
		require.True(t, ok, "terminology gate must be present")
		require.NotNil(t, gate.Execution)
		assert.Equal(t, []check.TermMatching{{
			Locale: "fr", Source: check.TermSourceEnglishInflection, Target: check.TermTargetContainmentForms,
			Rules: 1, RulesWithForms: 1,
		}}, gate.Execution.TermMatching)
	})
}
