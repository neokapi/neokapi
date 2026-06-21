package xml_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readXMLValidation drives the XML reader at the given validation mode.
func readXMLValidation(t *testing.T, input string, mode format.ValidationMode) (blocks []*model.Block, diags []format.Diagnostic, foundErr bool) {
	t.Helper()
	ctx := t.Context()
	reader := xmlfmt.NewReader()
	reader.Config().(format.ValidationConfig).SetValidationMode(mode)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	for result := range reader.Read(ctx) {
		if result.Error != nil {
			foundErr = true
			continue
		}
		if b, ok := result.Part.Resource.(*model.Block); ok && result.Part.Type == model.PartBlock && b.Translatable {
			blocks = append(blocks, b)
		}
	}
	diags = reader.Diagnostics()
	return blocks, diags, foundErr
}

// TestValidationMode_XML asserts default-off stays byte-identical and report mode
// surfaces a located structure.xml-well-formedness diagnostic on a mismatched
// end tag.
func TestValidationMode_XML(t *testing.T) {
	t.Parallel()
	// The end tag on line 2 doesn't match the open <item>, which the Go XML
	// decoder reports as a syntax error carrying that line.
	const input = "<root>\n  <item>hello</wrong>\n</root>"

	offBlocks, offDiags, offErr := readXMLValidation(t, input, format.ValidationOff)
	assert.Empty(t, offDiags, "validation off must record no diagnostics")
	assert.True(t, offErr, "malformed XML still surfaces a read error in off mode")

	repBlocks, repDiags, repErr := readXMLValidation(t, input, format.ValidationReport)
	assert.True(t, repErr, "report mode keeps the lenient error path")
	assert.Len(t, repBlocks, len(offBlocks), "report mode must not change extraction")

	require.Len(t, repDiags, 1, "report mode records one diagnostic: %+v", repDiags)
	d := repDiags[0]
	assert.Equal(t, "structure.xml-well-formedness", d.Category)
	assert.Equal(t, format.SeverityMajor, d.Severity)
	assert.NotEmpty(t, d.Message)
	assert.Equal(t, 2, d.Line, "the *xml.SyntaxError line should be carried through")
}

// TestValidationMode_XML_CleanInput confirms well-formed XML records nothing.
func TestValidationMode_XML_CleanInput(t *testing.T) {
	t.Parallel()
	const input = "<root><item>hello</item></root>"

	_, offDiags, offErr := readXMLValidation(t, input, format.ValidationOff)
	_, repDiags, repErr := readXMLValidation(t, input, format.ValidationReport)

	assert.False(t, offErr)
	assert.False(t, repErr)
	assert.Empty(t, offDiags)
	assert.Empty(t, repDiags, "clean XML records no diagnostics even in report mode")
}
