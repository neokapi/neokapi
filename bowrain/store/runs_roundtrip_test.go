package store

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRoundTrip_MixedRunKinds verifies that a Block containing every Run
// kind (TextRun, PcOpenRun, PcCloseRun, PlaceholderRun) round-trips
// through StoreBlocks → GetBlocks cleanly under the Runs-native JSON
// schema introduced by RFC 0001 Phase 3.
func TestRoundTrip_MixedRunKinds(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	srcRuns := []model.Run{
		{Text: &model.TextRun{Text: "Click "}},
		{PcOpen: &model.PcOpenRun{
			ID: "1", Type: "link:hyperlink", SubType: "html:a", Data: `<a href="x">`, Disp: "[A]",
			Constraints: &model.RunConstraints{Deletable: false, Cloneable: false, Reorderable: true},
		}},
		{Text: &model.TextRun{Text: "here"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "link:hyperlink", SubType: "html:a", Data: "</a>"}},
		{Text: &model.TextRun{Text: " or call "}},
		{Ph: &model.PlaceholderRun{
			ID: "2", Type: "entity:phone", Data: "+1-555-0100", Equiv: "phone", Disp: "[PHONE]",
			Constraints: &model.RunConstraints{Deletable: false, Cloneable: true, Reorderable: false},
		}},
	}

	tgtRuns := []model.Run{
		{Text: &model.TextRun{Text: "Cliquez "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "link:hyperlink", SubType: "html:a", Data: `<a href="x">`}},
		{Text: &model.TextRun{Text: "ici"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "link:hyperlink", SubType: "html:a", Data: "</a>"}},
		{Text: &model.TextRun{Text: " ou appelez "}},
		{Ph: &model.PlaceholderRun{ID: "2", Type: "entity:phone", Data: "+1-555-0100", Equiv: "phone"}},
	}

	b := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       srcRuns,
		Targets: map[model.VariantKey]*model.Target{
			model.Variant(model.LocaleFrench): {Runs: tgtRuns},
		},
		Properties: map[string]string{"client": "acme"},
	}

	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))

	got, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: ""})
	require.NoError(t, err)
	require.Len(t, got, 1)

	r := got[0].Block
	require.NotEmpty(t, r.Source)
	assert.Equal(t, "tu1", r.ID)

	// Source runs survive byte-for-byte.
	assertRunsEqual(t, srcRuns, r.Source)

	// Target runs survive byte-for-byte.
	require.NotNil(t, r.Target(model.LocaleFrench))
	assertRunsEqual(t, tgtRuns, r.TargetRuns(model.LocaleFrench))
}

// TestRoundTrip_RunsKeysComputedFromRuns verifies that the structural
// and generalized keys exposed via the bowrain TM matchers compute the
// same values from a Run sequence as their Runs-native helpers.
func TestRoundTrip_RunsKeysComputedFromRuns(t *testing.T) {
	runs := []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{Ph: &model.PlaceholderRun{ID: "1", Type: "entity:person", Data: "Alice", Equiv: "person"}},
		{Text: &model.TextRun{Text: " says "}},
		{PcOpen: &model.PcOpenRun{ID: "2", Type: "fmt:bold", Data: "<b>"}},
		{Text: &model.TextRun{Text: "hi"}},
		{PcClose: &model.PcCloseRun{ID: "2", Type: "fmt:bold", Data: "</b>"}},
	}

	plain := model.RunsPlainText(runs)
	flat := model.FlattenRuns(runs)
	struc := model.RunsStructuralText(runs)
	gen := model.RunsGeneralizedText(runs)

	assert.Equal(t, "Hello  says hi", plain)
	assert.Equal(t, "Hello {person} says hi", flat)
	assert.Equal(t, "Hello {1/} says {2}hi{/2}", struc)
	assert.Equal(t, "Hello {PERSON} says {2}hi{/2}", gen)
}

// assertRunsEqual asserts that two Run sequences are equal field-by-field.
// Adjacent TextRuns are compared individually (no coalescing).
func assertRunsEqual(t *testing.T, expected, actual []model.Run) {
	t.Helper()
	require.Len(t, actual, len(expected), "run count")
	for i := range expected {
		er, ar := expected[i], actual[i]
		assert.Equal(t, er.Kind(), ar.Kind(), "run[%d] kind", i)
		switch er.Kind() {
		case model.RunKindText:
			assert.Equal(t, er.Text.Text, ar.Text.Text, "run[%d] text", i)
		case model.RunKindPh:
			assert.Equal(t, er.Ph.ID, ar.Ph.ID, "run[%d] ph id", i)
			assert.Equal(t, er.Ph.Type, ar.Ph.Type, "run[%d] ph type", i)
			assert.Equal(t, er.Ph.SubType, ar.Ph.SubType, "run[%d] ph subType", i)
			assert.Equal(t, er.Ph.Data, ar.Ph.Data, "run[%d] ph data", i)
			assert.Equal(t, er.Ph.Equiv, ar.Ph.Equiv, "run[%d] ph equiv", i)
			assert.Equal(t, er.Ph.Disp, ar.Ph.Disp, "run[%d] ph disp", i)
		case model.RunKindPcOpen:
			assert.Equal(t, er.PcOpen.ID, ar.PcOpen.ID, "run[%d] pcOpen id", i)
			assert.Equal(t, er.PcOpen.Type, ar.PcOpen.Type, "run[%d] pcOpen type", i)
			assert.Equal(t, er.PcOpen.SubType, ar.PcOpen.SubType, "run[%d] pcOpen subType", i)
			assert.Equal(t, er.PcOpen.Data, ar.PcOpen.Data, "run[%d] pcOpen data", i)
			assert.Equal(t, er.PcOpen.Equiv, ar.PcOpen.Equiv, "run[%d] pcOpen equiv", i)
			assert.Equal(t, er.PcOpen.Disp, ar.PcOpen.Disp, "run[%d] pcOpen disp", i)
		case model.RunKindPcClose:
			assert.Equal(t, er.PcClose.ID, ar.PcClose.ID, "run[%d] pcClose id", i)
			assert.Equal(t, er.PcClose.Type, ar.PcClose.Type, "run[%d] pcClose type", i)
			assert.Equal(t, er.PcClose.SubType, ar.PcClose.SubType, "run[%d] pcClose subType", i)
			assert.Equal(t, er.PcClose.Data, ar.PcClose.Data, "run[%d] pcClose data", i)
		}
	}
}
