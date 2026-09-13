package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClipWord(t *testing.T) {
	assert.Equal(t, "kaiplan", ClipWord("kaiplass"))
	assert.Equal(t, "varan", ClipWord("varsel"))
	assert.Equal(t, "Liegeplan", ClipWord("Liegeplatz"))
	assert.Empty(t, ClipWord(" "))
	for _, w := range []string{"kaiplass", "varsel", "plan", "tillegg", "an", "x", "Liegeplatz", "Schiff"} {
		assert.NotContains(t, ClipWord(w), w, "ClipWord(%q) = %q", w, ClipWord(w))
	}
}
