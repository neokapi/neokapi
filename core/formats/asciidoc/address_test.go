package asciidoc_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A block's readable name carries its sections' titles, so a translated document
// names the same paragraph in its own language. The translation-invariant
// address writes each section as the identity of the heading that opened it, so
// it reads the same on both sides — which is what pairs a source file with its
// translation unit by unit.

func addressesInOrder(t *testing.T, input string) []string {
	t.Helper()
	var out []string
	for _, b := range readBlocks(t, input) {
		if addr := b.StructuralAddress(); addr != "" {
			out = append(out, addr)
			continue
		}
		out = append(out, b.Name)
	}
	return out
}

func namesInOrder(t *testing.T, input string) []string {
	t.Helper()
	var out []string
	for _, b := range readBlocks(t, input) {
		out = append(out, b.Name)
	}
	return out
}

func TestAsciiDocAddress_SurvivesTranslation(t *testing.T) {
	const source = `== What it reads

The first paragraph of the section.

The second paragraph of the section.

=== Detail

A paragraph one level deeper.

== How it reports

A paragraph under the second heading.
`

	const translated = `== Hva den leser

Det første avsnittet i seksjonen.

Det andre avsnittet i seksjonen.

=== Detalj

Et avsnitt ett nivå dypere.

== Hvordan den rapporterer

Et avsnitt under den andre overskriften.
`

	sourceNames := namesInOrder(t, source)
	targetNames := namesInOrder(t, translated)
	require.Len(t, targetNames, len(sourceNames))
	assert.NotEqual(t, sourceNames, targetNames,
		"the readable names are written in each document's own language — if they "+
			"already agreed there would be nothing for the address to fix")

	assert.Equal(t, addressesInOrder(t, source), addressesInOrder(t, translated),
		"the invariant address must not depend on any block's words")

	assert.Equal(t, []string{
		"h2",
		"h2/p",
		"h2/p#2",
		"h2/h3",
		"h2/h3/p",
		"h2#2",
		"h2#2/p",
	}, addressesInOrder(t, source))
}

// Outside any section a name carries no title, so it is already invariant and
// no address is composed.
func TestAsciiDocAddress_AbsentWhenTheNameIsAlreadyInvariant(t *testing.T) {
	const doc = `A paragraph before any heading.

Another one.
`
	for _, b := range readBlocks(t, doc) {
		assert.Empty(t, b.StructuralAddress(), "block %q", b.Name)
	}
}
