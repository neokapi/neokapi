package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const norwegian = model.LocaleID("nb")

func serverRule() *tools.TermCheckConfig {
	return &tools.TermCheckConfig{
		TermRules:    []coreprofile.TermRule{{Term: "server", Replacement: "tjener", ConceptID: "c-server"}},
		SourceLocale: model.LocaleEnglish,
		TargetLocale: norwegian,
	}
}

func TestTermCheckUses_PositionsBothSides(t *testing.T) {
	t.Parallel()
	uses := tools.TermCheckUses(serverRule(), "Can't reach your server", "Får ikke kontakt med tjeneren")
	require.Len(t, uses, 1)
	use := uses[0]
	assert.True(t, use.Satisfied)
	assert.Equal(t, [][2]int{{17, 23}}, use.Source)
	// Containment, as the gate decides it: the definite form holds the rendering.
	assert.Equal(t, [][2]int{{22, 28}}, use.Target)
}

func TestTermCheckUses_MissingRenderingIsTheViolation(t *testing.T) {
	t.Parallel()
	cfg := serverRule()
	uses := tools.TermCheckUses(cfg, "Can't reach your server", "Får ikke kontakt med serveren")
	require.Len(t, uses, 1)
	assert.False(t, uses[0].Satisfied)
	assert.Empty(t, uses[0].Target)

	errs, _ := tools.TermCheckViolations(cfg, "Can't reach your server", "Får ikke kontakt med serveren")
	assert.Len(t, errs, 1, "the violation and the use are one decision")
}

func TestTermCheckUses_CaseFoldsWithoutShiftingOffsets(t *testing.T) {
	t.Parallel()
	uses := tools.TermCheckUses(serverRule(), "Server down", "Ærlig talt: TJENEREN er nede")
	require.Len(t, uses, 1)
	target := "Ærlig talt: TJENEREN er nede"
	h := uses[0].Target[0]
	assert.Equal(t, "TJENER", target[h[0]:h[1]])
}

func TestTermCheckOverlays_AnchorsSourceAndTranslation(t *testing.T) {
	t.Parallel()
	src := []model.Run{
		{Text: &model.TextRun{Text: "Can't reach your "}},
		{Ph: &model.PlaceholderRun{ID: "1", Data: "<b>"}},
		{Text: &model.TextRun{Text: "server"}},
	}
	tgt := []model.Run{{Text: &model.TextRun{Text: "Får ikke kontakt med tjeneren"}}}

	ovs := tools.TermCheckOverlays(serverRule(), src, tgt)
	require.Len(t, ovs, 2)

	s := ovs[0]
	assert.Equal(t, model.OverlayTerm, s.Type)
	assert.True(t, s.OnSource())
	require.Len(t, s.Spans, 1)
	assert.Equal(t, "term-check:0", s.Spans[0].ID)
	assert.Equal(t, "true", s.Spans[0].Props["satisfied"])
	assert.Equal(t, "tjener", s.Spans[0].Props["replacement"])
	// A boundary at the end of a text run belongs to the run after it (model.RangeAnchor).
	assert.Equal(t, model.RunPos{Run: 1, Offset: 0}, s.Spans[0].Range.Start)
	assert.Equal(t, model.RunPos{Run: 3, Offset: 0}, s.Spans[0].Range.End)
	ann, ok := s.Spans[0].Value.(*model.TermAnnotation)
	require.True(t, ok)
	assert.Equal(t, "c-server", ann.ConceptID)
	assert.Equal(t, "tjener", ann.TargetTerms[0].Text)

	g := ovs[1]
	assert.Equal(t, model.Variant(norwegian), g.Edition)
	require.Len(t, g.Spans, 1)
	assert.Equal(t, "term-check:0", g.Spans[0].ID, "a rendering pairs with its source term by id")
	assert.Equal(t, model.RunPos{Run: 0, Offset: 21}, g.Spans[0].Range.Start)
	assert.Equal(t, model.RunPos{Run: 0, Offset: 27}, g.Spans[0].Range.End)
}

func TestTermCheckOverlays_NoDemandedTermIsNil(t *testing.T) {
	t.Parallel()
	src := []model.Run{{Text: &model.TextRun{Text: "Retry"}}}
	assert.Nil(t, tools.TermCheckOverlays(serverRule(), src, src))
}
