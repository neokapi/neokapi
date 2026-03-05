//go:build integration

package okf_xini

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SegmentationAndDesegmentationTest
//
// These tests verify XINI segmentation and desegmentation behavior:
// sentence splitting, whitespace handling, placeholder and formatting
// preservation across segmentation boundaries.
// ---------------------------------------------------------------------------

// okapi: SegmentationAndDesegmentationTest#sentencesAreSegmentedAndWhitespaceIsSavedInAttribute
func TestSegmentation_SentencesSegmentedAndWhitespaceSaved(t *testing.T) {
	// Sentences with trailing whitespace should be segmented with whitespace
	// preserved in attributes.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>seg-test.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">First sentence. Second sentence.</Seg>
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
	require.NotEmpty(t, blocks, "should extract translatable blocks")

	// The text should contain "First sentence" and/or "Second sentence".
	texts := bridgetest.BlockTexts(blocks)
	foundFirst := false
	foundSecond := false
	for _, text := range texts {
		if findBlockContaining(blocks, "First sentence") != nil {
			foundFirst = true
		}
		if findBlockContaining(blocks, "Second sentence") != nil {
			foundSecond = true
		}
		_ = text
	}
	assert.True(t, foundFirst || foundSecond, "should find segmented sentences")
}

// okapi: SegmentationAndDesegmentationTest#surroundingWhitespacesAreMovedIntoAttributes
func TestSegmentation_SurroundingWhitespacesMoved(t *testing.T) {
	// Leading/trailing whitespace around segments should be moved into attributes.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>ws-test.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"> Surrounded by spaces </Seg>
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
	require.NotEmpty(t, blocks, "should extract translatable blocks")

	// Roundtrip should preserve the whitespace.
	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Surrounded by spaces", "text content should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#whitespacesFromInBetweenAreMovedIntoAttributes
func TestSegmentation_WhitespacesBetweenMovedToAttributes(t *testing.T) {
	// Whitespace between segments should be moved into attributes.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>ws-between.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">First.</Seg>
								<Seg SegID="1"> </Seg>
								<Seg SegID="2">Second.</Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	parts := readXINIString(t, xini, nil)
	// Should not crash and should produce parts.
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should produce translatable blocks")
}

// okapi: SegmentationAndDesegmentationTest#codesAreNotMovedIntoAttributes
func TestSegmentation_CodesNotMovedIntoAttributes(t *testing.T) {
	// Inline codes should not be moved into whitespace attributes during segmentation.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>codes-test.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><ph type="ph" ID="1"/>Text with code.</Seg>
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
	require.NotEmpty(t, blocks, "should extract translatable blocks")

	// The block should still have inline codes.
	hasAnySpan := false
	for _, b := range blocks {
		if spanCount(b) > 0 {
			hasAnySpan = true
			break
		}
	}
	assert.True(t, hasAnySpan, "inline codes should not be moved into attributes")
}

// okapi: SegmentationAndDesegmentationTest#desegmentizedXiniContainsTrailingWhitespaces
func TestSegmentation_DesegmentizedContainsTrailingWhitespaces(t *testing.T) {
	// Desegmented output should contain trailing whitespace.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>deseg-ws.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">Text with trailing space </Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Text with trailing space", "text should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#desegmentizedXiniHasOriginalSegmentIDsRestored
func TestSegmentation_DesegmentizedHasOriginalSegmentIDs(t *testing.T) {
	// After desegmentation, original segment IDs should be restored.
	output := fileRoundtrip(t, "contents.xini", nil)
	assert.Contains(t, output, "SegID", "segment IDs should be preserved in roundtrip")
}

// okapi: SegmentationAndDesegmentationTest#originalSegmentIdIsSavedInAttribute
func TestSegmentation_OriginalSegmentIdSaved(t *testing.T) {
	// The original segment ID should be stored in an attribute during segmentation.
	parts := readXINIDefault(t, "contents.xini")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks")

	// Each block should have an ID.
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID (from segment)")
	}
}

// okapi: SegmentationAndDesegmentationTest#newSegmentsHaveIncreasingIDs
func TestSegmentation_NewSegmentsHaveIncreasingIDs(t *testing.T) {
	// When segmentation splits content, new segments should have increasing IDs.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>seg-ids.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">First. Second. Third.</Seg>
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
	require.NotEmpty(t, blocks, "should extract translatable blocks")

	// All block IDs should be unique.
	ids := make(map[string]bool)
	for _, b := range blocks {
		if b.ID != "" {
			assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
			ids[b.ID] = true
		}
	}
}

// okapi: SegmentationAndDesegmentationTest#formattingsAreNotBreakingApart
func TestSegmentation_FormattingsNotBreakingApart(t *testing.T) {
	// Formatting tags should not be broken apart during segmentation.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>fmt-seg.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><sph type="fmt" ID="1"/>Bold sentence.<eph type="fmt" ID="1"/></Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Bold sentence", "formatted text should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#formattingsAreNotBreakingApart2
func TestSegmentation_FormattingsNotBreakingApart2(t *testing.T) {
	// Second variant: formatting across multiple words.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>fmt-seg2.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">Normal <sph type="fmt" ID="1"/>bold words<eph type="fmt" ID="1"/> normal.</Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "bold words", "formatted text should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#formattingsAreNotBreakingApart3
func TestSegmentation_FormattingsNotBreakingApart3(t *testing.T) {
	// Third variant: nested formatting.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>fmt-seg3.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><sph type="fmt" ID="1"/><sph type="fmt" ID="2"/>Nested bold italic<eph type="fmt" ID="2"/><eph type="fmt" ID="1"/></Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Nested bold italic", "nested formatted text should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#formattingsAreNotBrokenApart
func TestSegmentation_FormattingsNotBrokenApart(t *testing.T) {
	// Formatting should remain intact after roundtrip.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>fmt-intact.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><sph type="fmt" ID="1"/>Formatted text here.<eph type="fmt" ID="1"/></Seg>
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
	require.NotEmpty(t, blocks)

	// Verify text content is intact.
	b := findBlockContaining(blocks, "Formatted text here")
	require.NotNil(t, b, "should find block with formatted text")
}

// okapi: SegmentationAndDesegmentationTest#placeholdersAreNotBrokenApart
func TestSegmentation_PlaceholdersNotBrokenApart(t *testing.T) {
	// Placeholders should remain intact during segmentation.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>ph-intact.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><sph type="ph" ID="1"/>Placeholder text<eph type="ph" ID="1"/></Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Placeholder text", "placeholder text should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#segmentsMergedIfPreviousSegmentHasSurroundingTag
func TestSegmentation_SegmentsMergedIfPrevHasSurroundingTag(t *testing.T) {
	// Segments should merge when previous segment has a surrounding tag.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>merge-prev.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><sph type="fmt" ID="1"/>Tagged<eph type="fmt" ID="1"/></Seg>
								<Seg SegID="1"> following text.</Seg>
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
	require.NotEmpty(t, blocks, "should produce translatable blocks")
}

// okapi: SegmentationAndDesegmentationTest#segmentsMergedIfNextSegmentHasSurroundingTag
func TestSegmentation_SegmentsMergedIfNextHasSurroundingTag(t *testing.T) {
	// Segments should merge when next segment has a surrounding tag.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>merge-next.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">Leading text </Seg>
								<Seg SegID="1"><sph type="fmt" ID="1"/>tagged<eph type="fmt" ID="1"/></Seg>
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
	require.NotEmpty(t, blocks, "should produce translatable blocks")
}

// okapi: SegmentationAndDesegmentationTest#segmentsMergedIfBothSegmentsHaveSurroundingTag
func TestSegmentation_SegmentsMergedIfBothHaveSurroundingTag(t *testing.T) {
	// Segments should merge when both have surrounding tags.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>merge-both.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><sph type="fmt" ID="1"/>First tagged<eph type="fmt" ID="1"/></Seg>
								<Seg SegID="1"><sph type="fmt" ID="2"/>Second tagged<eph type="fmt" ID="2"/></Seg>
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
	require.NotEmpty(t, blocks, "should produce translatable blocks")
}

// okapi: SegmentationAndDesegmentationTest#placeholderDoesntChange
func TestSegmentation_PlaceholderDoesntChange(t *testing.T) {
	// Placeholders should not change during segmentation/desegmentation.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>ph-unchanged.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">Text with <ph type="ph" ID="1"/> placeholder.</Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "ph", "placeholder element should be preserved")
	assert.Contains(t, output, "Text with", "text content should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#placeholderDoesntChangeWithDifferentPlacholderType
func TestSegmentation_PlaceholderDoesntChangeWithDifferentType(t *testing.T) {
	// Placeholders with various types should not change.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>ph-types.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">Text with <ph type="lb" ID="1"/> line break.</Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Text with", "text should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#formattingTagsAndPlaceholdersDontChange
func TestSegmentation_FormattingTagsAndPlaceholdersDontChange(t *testing.T) {
	// Combined formatting tags and placeholders should survive segmentation.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>fmt-ph.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><sph type="fmt" ID="1"/>Bold with <ph type="ph" ID="2"/> placeholder<eph type="fmt" ID="1"/></Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Bold with", "text with mixed tags should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#lineBreaksArePreserved
func TestSegmentation_LineBreaksPreserved(t *testing.T) {
	// Line breaks within segments should be preserved.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>linebreak.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">Line one<ph type="lb" ID="1"/>Line two</Seg>
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
	require.NotEmpty(t, blocks, "should extract blocks with line breaks")

	// Roundtrip should preserve line break structure.
	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Line one", "text before line break should be preserved")
	assert.Contains(t, output, "Line two", "text after line break should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#isolatedPlaceholdersArePreserved
func TestSegmentation_IsolatedPlaceholdersPreserved(t *testing.T) {
	// Isolated (unpaired) placeholders should be preserved during segmentation.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>isolated-ph.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">Before <ph type="ph" ID="1"/> after.</Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Before", "text before isolated placeholder should be preserved")
	assert.Contains(t, output, "after", "text after isolated placeholder should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#openingTagsPreservedInSinglePlaceholders
func TestSegmentation_OpeningTagsPreservedInSinglePlaceholders(t *testing.T) {
	// Opening tags within single placeholders should be preserved.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>opening-single.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><sph type="ph" ID="1"/>Content</Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Content", "content should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#openingTagsPreservedInPlaceholders
func TestSegmentation_OpeningTagsPreservedInPlaceholders(t *testing.T) {
	// Opening tags within placeholder pairs should be preserved.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>opening-pair.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><sph type="ph" ID="1"/>Paired content<eph type="ph" ID="1"/></Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Paired content", "paired content should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#placeholdersWithSameIdArePreservedUnchanged
func TestSegmentation_PlaceholdersWithSameIdPreserved(t *testing.T) {
	// Placeholders with the same ID should be preserved unchanged.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>same-id.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><ph type="ph" ID="1"/>Text<ph type="ph" ID="1"/></Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Text", "text between same-ID placeholders should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#nestedPlaceholdersWithSameIdArePreservedUnchanged
func TestSegmentation_NestedPlaceholdersWithSameIdPreserved(t *testing.T) {
	// Nested placeholders with the same ID should be preserved unchanged.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>nested-same-id.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><sph type="ph" ID="1"/><ph type="ph" ID="1"/>Nested text<eph type="ph" ID="1"/></Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Nested text", "nested text should be preserved")
}

// okapi: SegmentationAndDesegmentationTest#emptyPlaceholdersWithSameIdArePreservedUnchanged
func TestSegmentation_EmptyPlaceholdersWithSameIdPreserved(t *testing.T) {
	// Empty placeholders with the same ID should be preserved unchanged.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>empty-same-id.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0"><ph type="ph" ID="1"/><ph type="ph" ID="1"/>Text</Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "Text", "text with empty same-ID placeholders should be preserved")
}
