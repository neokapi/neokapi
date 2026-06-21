package yaml_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readYAMLValidation drives the YAML reader at the given validation mode.
func readYAMLValidation(t *testing.T, input string, mode format.ValidationMode) (blocks []*model.Block, diags []format.Diagnostic, foundErr bool) {
	t.Helper()
	ctx := t.Context()
	reader := yamlfmt.NewReader()
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

// TestValidationMode_YAML asserts default-off stays byte-identical and report
// mode surfaces a located structure.yaml-syntax diagnostic on a malformed doc.
func TestValidationMode_YAML(t *testing.T) {
	t.Parallel()
	// The stray colon on line 2 is a mapping value in an illegal context, which
	// yaml.v3 reports with that line embedded in the message.
	const input = "a: 1\n b: 2\n"

	offBlocks, offDiags, offErr := readYAMLValidation(t, input, format.ValidationOff)
	assert.Empty(t, offDiags, "validation off must record no diagnostics")
	assert.True(t, offErr, "malformed YAML still surfaces a read error in off mode")

	repBlocks, repDiags, repErr := readYAMLValidation(t, input, format.ValidationReport)
	assert.True(t, repErr, "report mode keeps the lenient error path")
	assert.Len(t, repBlocks, len(offBlocks), "report mode must not change extraction")

	require.Len(t, repDiags, 1, "report mode records one diagnostic: %+v", repDiags)
	d := repDiags[0]
	assert.Equal(t, "structure.yaml-syntax", d.Category)
	assert.Equal(t, format.SeverityMajor, d.Severity)
	assert.NotEmpty(t, d.Message)
	assert.Equal(t, 2, d.Line, "yaml.v3 carries the line, best-effort parsed from its message")
}

// TestValidationMode_YAML_CleanInput confirms well-formed YAML records nothing.
func TestValidationMode_YAML_CleanInput(t *testing.T) {
	t.Parallel()
	const input = "title: Hello\nbody: World\n"

	offBlocks, offDiags, offErr := readYAMLValidation(t, input, format.ValidationOff)
	repBlocks, repDiags, repErr := readYAMLValidation(t, input, format.ValidationReport)

	assert.False(t, offErr)
	assert.False(t, repErr)
	assert.Empty(t, offDiags)
	assert.Empty(t, repDiags, "clean YAML records no diagnostics even in report mode")
	assert.Len(t, repBlocks, len(offBlocks))
}
