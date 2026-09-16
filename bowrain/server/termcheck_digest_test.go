package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// digestOf snapshots concepts the way a ship pass does and returns the digest
// the gate fingerprint carries.
func digestOf(t *testing.T, concepts ...terms.Concept) string {
	t.Helper()
	tb := &testTermStore{terms.NewInMemoryStore()}
	for _, c := range concepts {
		require.NoError(t, tb.AddConcept(t.Context(), c))
	}
	_, fp := snapshotTerms(t.Context(), tb)
	require.NotEmpty(t, fp, "a snapshot holding concepts digests to something")
	return fp
}

// kapiConcept is one product name, with the do-not-translate flag under test.
func kapiConcept(doNotTranslate bool) terms.Concept {
	return terms.Concept{
		ID:             "c-kapi",
		Source:         terms.TermSourceTerminology,
		DoNotTranslate: doNotTranslate,
		Terms:          []terms.Term{{Text: "kapi", Locale: "en", Status: model.TermPreferred}},
	}
}

// Marking a concept do-not-translate changes what the gate decides about
// unchanged content, so it must change the digest. Without this, merging the
// governed change-set that sets the flag left every stored verdict in place and
// the next pass counted answers computed under the old predicate.
func TestSnapshotDigestMovesWithDoNotTranslate(t *testing.T) {
	assert.NotEqual(t, digestOf(t, kapiConcept(false)), digestOf(t, kapiConcept(true)),
		"the flag decides whether a target must keep the term, so it is governance the digest covers")
}

// Declared forms widen what satisfies a rule, so a synced forms edit changes
// the gate's answer for content nobody touched.
func TestSnapshotDigestMovesWithForms(t *testing.T) {
	bare := terms.Concept{
		ID:     "c-alert",
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "alert", Locale: "en", Status: model.TermPreferred},
			{Text: "varsel", Locale: "nb", Status: model.TermPreferred},
		},
	}
	withForms := terms.Concept{
		ID:     "c-alert",
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "alert", Locale: "en", Status: model.TermPreferred},
			{Text: "varsel", Locale: "nb", Status: model.TermPreferred, Forms: []string{"varsler"}},
		},
	}

	assert.NotEqual(t, digestOf(t, bare), digestOf(t, withForms),
		"a declared form changes which targets pass, so it is governance the digest covers")
}

// The digest is not a free-standing value: it rides in the gate fingerprint,
// which is what decides whether a stored verdict may be counted instead of
// recomputed.
func TestGateFingerprintCarriesTheTermsDigest(t *testing.T) {
	plain := newTermGate("en", nil, digestOf(t, kapiConcept(false)), nil)
	kept := newTermGate("en", nil, digestOf(t, kapiConcept(true)), nil)

	assert.NotEqual(t,
		plain.fingerprint(t.Context(), []string{"fr"}),
		kept.fingerprint(t.Context(), []string{"fr"}),
		"a flag change reaches the fingerprint, so stored verdicts are recomputed")
}

// A digest that ignored the concepts entirely would still differ run to run if
// it folded in anything incidental. Two snapshots of the same concepts agree,
// which is what makes a stored verdict reusable at all.
func TestSnapshotDigestIsStableForUnchangedConcepts(t *testing.T) {
	first := digestOf(t, kapiConcept(true))
	second := digestOf(t, kapiConcept(true))

	assert.Equal(t, first, second,
		"two snapshots of the same concepts digest alike, so a verdict stays reusable")
}
