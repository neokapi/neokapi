package xliff_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// XLIFF 1.2 pairs <bpt>/<ept> by id, and the pairs may overlap. The reader
// keeps the overlap as crossed PcOpen/PcClose runs, and the fidelity guard
// that gates `kapi apply` must still accept a text edit that leaves every
// code where it was.
const overlappingPairsDoc = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
<file original="f" source-language="en" target-language="fr" datatype="plaintext">
<body>
<trans-unit id="1">
<source><bpt id="1">[b]</bpt>bold <bpt id="2">[i]</bpt>both<ept id="1">[/b]</ept> italic<ept id="2">[/i]</ept></source>
</trans-unit>
</body>
</file>
</xliff>`

func TestReader_OverlappingPairsStayEditable(t *testing.T) {
	blocks := readBlocks(t, overlappingPairsDoc)
	require.Len(t, blocks, 1)
	source := blocks[0].Source

	text := model.RunsPlaceholderText(source)
	require.Equal(t, `<x id="1"/>bold <x id="2"/>both<x id="/1"/> italic<x id="/2"/>`, text,
		"the reader keeps the source's overlapping pairs")

	edited := model.ParseRunsPlaceholderText(text+"!", source)
	assert.True(t, model.InlineCodesPreserved(source, edited),
		"a text edit that keeps every code in place is faithful")

	closeFirst := model.ParseRunsPlaceholderText(`<x id="/1"/><x id="1"/>bold <x id="2"/>both italic<x id="/2"/>`, source)
	assert.False(t, model.InlineCodesPreserved(source, closeFirst),
		"a close may not move ahead of its open")
}
