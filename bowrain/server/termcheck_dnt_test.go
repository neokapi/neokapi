package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/terms"
)

// TestTermGateDoNotTranslate_AgreesWithTermCheck holds blocks to a
// do-not-translate concept through the server's compliance predicate and through
// term-check, over the cases where a verbatim comparison and term-check could
// differ: the source's own casing and a placeholder name. The server gives the
// answer term-check gives for each.
func TestTermGateDoNotTranslate_AgreesWithTermCheck(t *testing.T) {
	ctx := context.Background()
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:             "c-kapi",
		Source:         terms.TermSourceTerminology,
		DoNotTranslate: true,
		Terms:          []terms.Term{{Text: "kapi", Locale: "en", Status: model.TermPreferred}},
	}))
	concepts, err := tb.Concepts(ctx)
	require.NoError(t, err)
	rules := terms.RulesFromConcepts(concepts, "en", "fr")
	gate := newTermGate("en", tb, "dnt-agreement", nil)

	cases := []struct {
		name      string
		source    string
		target    string
		compliant bool
	}{
		{"kept verbatim", "Open kapi", "Ouvrir kapi", true},
		{"translated", "Open kapi", "Ouvrir capi", false},
		{"kept in the source's own casing", "Kapi opens the project", "Kapi ouvre le projet", true},
		{"used only as a placeholder name", "{kapi} is ready", "{kapi} est prêt", true},
		{"kept only as a placeholder name", "Open kapi with {kapi}", "Ouvrir capi avec {kapi}", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs, _ := coretools.TermCheckViolations(&coretools.TermCheckConfig{
				TermRules: rules, SourceLocale: "en", TargetLocale: "fr",
			}, c.source, c.target)
			assert.Equal(t, c.compliant, len(errs) == 0, "term-check: %v", errs)

			want := platstore.TermComplianceCompliant
			if !c.compliant {
				want = platstore.TermComplianceViolation
			}
			assert.Equal(t, want, gate.compliance(ctx, mkFrBlock(c.source, c.target), "fr"), "the server predicate")
		})
	}
}
