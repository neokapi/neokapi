package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeepLongestDeclared(t *testing.T) {
	text := "Open the berth plan and the content memory bank."
	span := func(term string, index int) DeclaredSpan {
		hits := FindTerm(text, term)
		if len(hits) == 0 {
			t.Fatalf("%q not found in %q", term, text)
		}
		return DeclaredSpan{Start: hits[0][0], End: hits[0][1], Index: index}
	}
	berth, berthPlan := span("berth", 0), span("berth plan", 1)
	contentMemory, memoryBank := span("content memory", 2), span("memory bank", 3)
	berthAgain := DeclaredSpan{Start: berth.Start, End: berth.End, Index: 4}

	got := KeepLongestDeclared([]DeclaredSpan{berth, memoryBank, berthPlan, contentMemory})
	assert.Equal(t, []DeclaredSpan{berthPlan, contentMemory, memoryBank}, got,
		"berth inside berth plan is dropped; content memory and memory bank only overlap, so both stay")

	assert.Equal(t, []DeclaredSpan{berth, berthAgain}, KeepLongestDeclared([]DeclaredSpan{berthAgain, berth}),
		"two declarations covering the same bytes are both kept")

	in := []DeclaredSpan{berth, berthPlan}
	_ = KeepLongestDeclared(in)
	assert.Equal(t, []DeclaredSpan{berth, berthPlan}, in, "the input is not modified")
	assert.Empty(t, KeepLongestDeclared(nil))
}
