//go:build integration

package xini

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// XINIFilterFormattingTest
// ---------------------------------------------------------------------------

// okapi: XINIFilterFormattingTest#formattingsBecomePreserved
func TestFormatting_FormattingsBecomePreserved(t *testing.T) {
	// Formatting tags in XINI should survive a roundtrip.
	// Use a XINI snippet with formatting tags (sph/eph pairs for bold, italic, etc.)
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>format-test.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><sph type="fmt" ID="1"/>Bold text<eph type="fmt" ID="1"/></Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	// Roundtrip the content.
	output := snippetRoundtrip(t, xini, nil)

	// The output should preserve formatting elements.
	assert.Contains(t, output, "Bold text", "text content should be preserved")
	// Formatting tags should appear in the output as sph/eph elements.
	// Note: The XINI writer may strip the type attribute but preserves sph/eph structure.
	assert.Contains(t, output, "sph", "sph formatting tag should be preserved in output")
	assert.Contains(t, output, "eph", "eph formatting tag should be preserved in output")
}

// okapi: XINIFilterFormattingTest#tagsBecomeCodes
func TestFormatting_TagsBecomeCodes(t *testing.T) {
	// Formatting tags (sph/eph) in XINI should become inline codes (spans) in blocks.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>format-test.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><sph type="fmt" ID="1"/>Bold text<eph type="fmt" ID="1"/></Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	parts := readXINIString(t, xini, nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks with formatting")

	// Find a block containing "Bold text".
	b := findBlockContaining(blocks, "Bold text")
	require.NotNil(t, b, "should find block with 'Bold text'")

	// The formatting tags should become inline code spans.
	assert.Greater(t, inlineCodeCount(b), 0, "formatting tags should produce inline code spans")

	// Should have opening and closing span types.
	assert.True(t, hasSpanOfType(b, model.SpanOpening) || hasSpanOfType(b, model.SpanPlaceholder),
		"should have opening or placeholder spans for formatting")
}
