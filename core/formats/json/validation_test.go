package json_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readJSONValidation drives the JSON reader at the given validation mode and
// returns the translatable blocks, the recorded diagnostics, and whether a read
// error surfaced. The lenient extraction itself never changes with the mode —
// only diagnostics are added — so callers compare blocks across modes.
func readJSONValidation(t *testing.T, input string, mode format.ValidationMode) (blocks []*model.Block, diags []format.Diagnostic, foundErr bool) {
	t.Helper()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
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

// TestValidationMode_JSON checks that Reader Validation-Mode is default-off
// (no diagnostics, identical lenient extraction) and, when on, surfaces located
// structure.json-* diagnostics for genuinely malformed input.
func TestValidationMode_JSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		category string
		line     int // expected 1-based line; 0 = don't assert
	}{
		{
			name:     "unterminated string",
			input:    `{"appTitle": "Flutter`,
			category: "structure.json-syntax",
		},
		{
			// The '@' on line 3 is not a valid value start; the scanner reports
			// it with that position.
			name:     "unexpected character with line",
			input:    "{\n  \"a\": \"ok\",\n  \"b\": @bad\n}",
			category: "structure.json-syntax",
			line:     3,
		},
		{
			name:     "invalid unicode escape",
			input:    `{"k": "\uZZZZ"}`,
			category: "structure.json-unicode-escape",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			offBlocks, offDiags, offErr := readJSONValidation(t, tt.input, format.ValidationOff)
			// Off is byte-identical: no diagnostics, the same (lenient) outcome.
			assert.Empty(t, offDiags, "validation off must record no diagnostics")
			assert.True(t, offErr, "malformed input still surfaces a read error in off mode")

			repBlocks, repDiags, repErr := readJSONValidation(t, tt.input, format.ValidationReport)
			assert.True(t, repErr, "report mode keeps the lenient error path")
			// Extraction is unchanged between modes.
			assert.Len(t, repBlocks, len(offBlocks), "report mode must not change extraction")

			require.Len(t, repDiags, 1, "report mode records exactly one diagnostic: %+v", repDiags)
			d := repDiags[0]
			assert.Equal(t, tt.category, d.Category)
			assert.Equal(t, format.SeverityMajor, d.Severity)
			assert.NotEmpty(t, d.Message)
			assert.Positive(t, d.Line, "a located diagnostic carries a 1-based line")
			assert.Positive(t, d.Column, "a located diagnostic carries a 1-based column")
			if tt.line != 0 {
				assert.Equal(t, tt.line, d.Line, "line should match the bad token's position")
			}
		})
	}
}

// TestValidationMode_JSON_CleanInput confirms a well-formed file records no
// diagnostics even in report mode, and extraction is unchanged.
func TestValidationMode_JSON_CleanInput(t *testing.T) {
	t.Parallel()
	const input = `{"title": "Hello", "body": "World"}`

	offBlocks, offDiags, offErr := readJSONValidation(t, input, format.ValidationOff)
	repBlocks, repDiags, repErr := readJSONValidation(t, input, format.ValidationReport)

	assert.False(t, offErr)
	assert.False(t, repErr)
	assert.Empty(t, offDiags)
	assert.Empty(t, repDiags, "clean input records no diagnostics even in report mode")
	assert.Len(t, repBlocks, len(offBlocks))
	assert.Len(t, repBlocks, 2)
}
