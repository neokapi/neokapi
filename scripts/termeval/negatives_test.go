package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNegativesFor(t *testing.T) {
	assert.Equal(t, []Negative{
		{Kind: "deleted", Target: "Book en kaiplass nå, eller ."},
		{Kind: "clipped", Target: "Book en kaiplass nå, eller Kaiplasan."},
	}, negativesFor("Book en kaiplass nå, eller Kaiplasser.", "Kaiplasser"),
		"only the words that contain the rendering change: kaiplass does not contain Kaiplasser")

	got := negativesFor("Book en kaiplass nå", "kaiplass")
	assert.Equal(t, "Book en  nå", got[0].Target)
	assert.Equal(t, "Book en kaiplan nå", got[1].Target, "the clip of kaiplass is the reviewed Kaiplan")

	assert.Equal(t, "Se inline-kan her", negativesFor("Se inline-koder her", "inline-kode")[1].Target,
		"a hyphenated word is one word")
	assert.Nil(t, negativesFor("Ingen treff her", "kaiplass"))
	assert.Nil(t, negativesFor("Ingen treff her", " "))
}

// TestNegativesForAll_LeavesNoSurface: a target that uses a rule through a
// rendering and a form keeps neither in its negatives, so neither can satisfy
// the rule again.
func TestNegativesForAll_LeavesNoSurface(t *testing.T) {
	surfaces := []string{"varsel", "varsler"}
	negs := negativesForAll("Ett varsel og to varsler", surfaces)
	assert.Len(t, negs, 2)
	for _, n := range negs {
		for _, s := range surfaces {
			assert.NotContains(t, strings.ToLower(n.Target), s, "%s negative %q keeps %q", n.Kind, n.Target, s)
		}
	}
	assert.Equal(t, "Ett varan og to varan", negs[1].Target, "every surface word is clipped from the first surface used")
}
