package mif

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	"github.com/stretchr/testify/assert"
)

// referenceSortSkelOps is the original hand-rolled stable insertion sort
// that sortSkelOps replaced (#608 L2). It is kept here only as a test
// oracle: the slices.SortStableFunc-based implementation must produce a
// byte-identical ordering for any input.
func referenceSortSkelOps(ops []skelOp) {
	for i := 1; i < len(ops); i++ {
		for j := i; j > 0; j-- {
			a, b := ops[j-1], ops[j]
			if a.start < b.start || (a.start == b.start && a.kind <= b.kind) {
				break
			}
			ops[j-1], ops[j] = b, a
		}
	}
}

func TestSortSkelOps(t *testing.T) {
	// Mix of distinct starts, equal-start/different-kind (tie-break:
	// refs before elides before rewrites), and equal-(start,kind) pairs
	// distinguished by refID to verify stability.
	input := []skelOp{
		{start: 50, kind: opRewrite, rewriteOut: "z"},
		{start: 10, kind: opElide},
		{start: 10, kind: opRef, refID: "0:0"},
		{start: 30, kind: opRef, refID: "1:0"},
		{start: 10, kind: opRef, refID: "0:1"}, // equal (start,kind) — must keep input order after 0:0
		{start: 50, kind: opRef, refID: "2:0"},
		{start: 30, kind: opElide},
		{start: 50, kind: opElide},
	}

	got := append([]skelOp(nil), input...)
	want := append([]skelOp(nil), input...)
	sortSkelOps(got)
	referenceSortSkelOps(want)

	assert.Equal(t, want, got,
		"sortSkelOps must order identically to the original stable insertion sort")

	// Spot-check the documented invariants directly.
	assert.Equal(t, opRef, got[0].kind)
	assert.Equal(t, "0:0", got[0].refID, "equal (start,kind) ops keep insertion order")
	assert.Equal(t, "0:1", got[1].refID)
	assert.Equal(t, opElide, got[2].kind, "ref precedes elide at equal start")

	// Empty and single-element inputs must not panic.
	sortSkelOps(nil)
	sortSkelOps([]skelOp{{start: 1, kind: opRef}})
}

// TestSpec drives every Feature × Example in spec.yaml through the
// native MIF reader. Failures pinpoint the feature and example so the
// spec doubles as documentation and verification.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return NewReader() },
	}
	r.Run(t)
}
