package openxml

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestOptimiseDMLBlockProperties_SingleRunHoist mirrors upstream Okapi's
// StyleOptimisation.Default for a single-run DrawingML paragraph: the
// run's entire rPr (attrs + children) gets hoisted into the paragraph's
// pPr as <a:defRPr>, and the run is left with no rPr.
//
// Fixture: DrawingML_Test.docx "Important" textbox paragraph
// (`<a:p><a:pPr algn="ctr"/><a:r><a:rPr sz="2000" b="1"><a:solidFill>
// <a:srgbClr val="FFFFFF"/></a:solidFill></a:rPr><a:t>Important</a:t>
// </a:r>...</a:p>`).
//
// The input here is the BODY between `<a:p…>` and `</a:p>` —
// optimiseDMLBlockProperties operates on the slice the
// stripDMLRunPropertyAttrs loop hands it.
func TestOptimiseDMLBlockProperties_SingleRunHoist(t *testing.T) {
	in := `<a:pPr algn="ctr"></a:pPr><a:r><a:rPr sz="2000" b="1"><a:solidFill><a:srgbClr val="FFFFFF"></a:srgbClr></a:solidFill></a:rPr><a:t>Important</a:t></a:r><a:endParaRPr sz="2000" b="1"><a:solidFill><a:srgbClr val="FFFFFF"></a:srgbClr></a:solidFill></a:endParaRPr>`
	// Expected: pPr gains a defRPr first child carrying the hoisted
	// attrs + children (attrs sorted alphabetically); run loses its
	// rPr entirely. endParaRPr is untouched.
	want := `<a:pPr algn="ctr"><a:defRPr b="1" sz="2000"><a:solidFill><a:srgbClr val="FFFFFF"></a:srgbClr></a:solidFill></a:defRPr></a:pPr><a:r><a:t>Important</a:t></a:r><a:endParaRPr sz="2000" b="1"><a:solidFill><a:srgbClr val="FFFFFF"></a:srgbClr></a:solidFill></a:endParaRPr>`
	assert.Equal(t, want, optimiseDMLBlockProperties(in))
}

// TestOptimiseDMLBlockProperties_TwoRunsCommonSubset mirrors
// StyleOptimisation.Default.commonRunPropertiesOf: only properties
// shared by every run are hoisted. The run-specific properties stay
// on the per-run rPr.
//
// Two runs share `b="1"` but only one has `i="1"`. Expected: hoist
// `b="1"`, keep `i="1"` on the first run.
func TestOptimiseDMLBlockProperties_TwoRunsCommonSubset(t *testing.T) {
	in := `<a:r><a:rPr b="1" i="1"></a:rPr><a:t>Foo</a:t></a:r><a:r><a:rPr b="1"></a:rPr><a:t>Bar</a:t></a:r>`
	// rPr with only attrs and no children is rebuilt as the
	// self-closing form (matches how upstream Okapi's
	// RunProperties.getEvents emits an empty rPr).
	want := `<a:pPr><a:defRPr b="1"/></a:pPr><a:r><a:rPr i="1"/><a:t>Foo</a:t></a:r><a:r><a:t>Bar</a:t></a:r>`
	assert.Equal(t, want, optimiseDMLBlockProperties(in))
}

// TestOptimiseDMLBlockProperties_NoCommonProps: when runs share no
// properties, the optimisation must be a no-op. Mirrors
// StyleOptimisation.Default.applyTo lines 108-111 ("there is nothing
// to optimise (the run properties are all different)") which falls
// back to bypassOptimisation.applyTo(chunks).
func TestOptimiseDMLBlockProperties_NoCommonProps(t *testing.T) {
	in := `<a:r><a:rPr b="1"></a:rPr><a:t>Foo</a:t></a:r><a:r><a:rPr i="1"></a:rPr><a:t>Bar</a:t></a:r>`
	// No common — output equals input.
	assert.Equal(t, in, optimiseDMLBlockProperties(in))
}

// TestOptimiseDMLBlockProperties_OneRunWithoutRPrBails: a run without
// any direct rPr has empty direct properties, which clears the common
// set per upstream commonRunPropertiesOf lines 225-228. The
// optimisation aborts and the input passes through unchanged.
func TestOptimiseDMLBlockProperties_OneRunWithoutRPrBails(t *testing.T) {
	in := `<a:r><a:rPr b="1"></a:rPr><a:t>Foo</a:t></a:r><a:r><a:t>Bar</a:t></a:r>`
	assert.Equal(t, in, optimiseDMLBlockProperties(in))
}

// TestOptimiseDMLBlockProperties_NoRuns: a paragraph with no `<a:r>`
// elements is a no-op (no candidate runs).
func TestOptimiseDMLBlockProperties_NoRuns(t *testing.T) {
	in := `<a:pPr></a:pPr><a:endParaRPr lang="en-US"></a:endParaRPr>`
	assert.Equal(t, in, optimiseDMLBlockProperties(in))
}

// TestOptimiseDMLBlockProperties_SynthesisePPr: when the paragraph has
// no `<a:pPr>` element at all, we synthesise one to carry the hoisted
// defRPr. Mirrors StyleOptimisation.Default.paragraphBlockPropertiesOf
// lines 158-185 which creates an empty `<a:pPr>` envelope when missing.
func TestOptimiseDMLBlockProperties_SynthesisePPr(t *testing.T) {
	in := `<a:r><a:rPr sz="1800"></a:rPr><a:t>Hello</a:t></a:r>`
	want := `<a:pPr><a:defRPr sz="1800"/></a:pPr><a:r><a:t>Hello</a:t></a:r>`
	assert.Equal(t, want, optimiseDMLBlockProperties(in))
}

// TestOptimiseDMLBlockProperties_SelfClosingPPrExpansion: a
// self-closing `<a:pPr lvl="1"/>` must expand to its open form so the
// defRPr can sit inside it.
func TestOptimiseDMLBlockProperties_SelfClosingPPrExpansion(t *testing.T) {
	in := `<a:pPr lvl="1"/><a:r><a:rPr sz="2400"></a:rPr><a:t>X</a:t></a:r>`
	want := `<a:pPr lvl="1"><a:defRPr sz="2400"/></a:pPr><a:r><a:t>X</a:t></a:r>`
	assert.Equal(t, want, optimiseDMLBlockProperties(in))
}

// TestOptimiseDMLBlockProperties_EmptyRPrIsNoOp: an `<a:rPr/>` with
// nothing in it counts as empty direct properties; like the
// missing-rPr case the optimisation bails.
func TestOptimiseDMLBlockProperties_EmptyRPrIsNoOp(t *testing.T) {
	in := `<a:r><a:rPr/><a:t>Foo</a:t></a:r>`
	assert.Equal(t, in, optimiseDMLBlockProperties(in))
}
