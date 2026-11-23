package venue

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/terms"
)

// TestEditionWriteEditionIsStable pins the bytes of EditionWrite.Edition, the
// key of the sync cache's writes_sent map.
func TestEditionWriteEditionIsStable(t *testing.T) {
	assert.Equal(t, "u1\x00nb", EditionWrite{Unit: "u1", Variant: "nb"}.Edition())
}

// TestDecisionsComponentIsStable pins the decisions component, which the ref
// cache and the server compare, over the names decisionKey gives.
func TestDecisionsComponentIsStable(t *testing.T) {
	d := UnitDecision{ItemName: "docs/a.md", Unit: "u1", Variant: "nb", Note: "checked"}
	assert.Equal(t, "docs/a.md\x00u1\x00nb", decisionKey(d))
	assert.Equal(t, "8fee0c835bca8c11a53a3be17f2640032d9e8c0cafaf5728988bf8a2fef2d2ad", DecisionsComponent([]UnitDecision{d}))
}

// TestTermsComponentIsStable pins the terms component, which the ref cache
// and the server compare, over a concept with two terms.
func TestTermsComponentIsStable(t *testing.T) {
	c := terms.Concept{ID: "c1", Terms: []terms.Term{{Locale: "en", Text: "save"}, {Locale: "nb", Text: "lagre"}}}
	assert.Equal(t, "eb3a3f2896ef829103dadbea7f2b5905346c705e615de34e5e9a002c9d55bd7c", TermsComponent([]terms.Concept{c}, nil))
}
