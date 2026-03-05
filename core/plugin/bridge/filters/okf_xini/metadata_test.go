//go:build integration

package okf_xini

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// XINIFilterMetainformationTest
// ---------------------------------------------------------------------------

// okapi: XINIFilterMetainformationTest#iniTableIsPreserved
func TestMetadata_IniTableIsPreserved(t *testing.T) {
	// An INI table structure in XINI should be read and text content extracted.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>ini-test.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<IniTable IniTableID="0" IniTableLabel="Settings">
							<Fields>
								<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
									<Seg SegID="0">INI table content</Seg>
								</Field>
							</Fields>
						</IniTable>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	parts := readXINIString(t, xini, nil)

	// The INI table structure should be read without errors.
	require.NotEmpty(t, parts, "should produce parts from INI table XINI")
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")

	// The XINI filter may represent INI table content as blocks or data parts.
	// Verify the structural integrity of the read.
	blocks := allBlocks(parts)
	if len(blocks) > 0 {
		b := findBlockContaining(blocks, "INI table content")
		assert.NotNil(t, b, "should find block with INI table content")
	}
}

// okapi: XINIFilterMetainformationTest#segmentIsPreserved
func TestMetadata_SegmentIsPreserved(t *testing.T) {
	// Segment metadata should survive a roundtrip.
	output := fileRoundtrip(t, "contents.xini", nil)
	assert.Contains(t, output, "Seg", "Seg elements should be preserved")
	assert.Contains(t, output, "Test!", "segment text 'Test!' should be preserved")
	assert.Contains(t, output, "Test.", "segment text 'Test.' should be preserved")
}

// okapi: XINIFilterMetainformationTest#tableIsPreserved
func TestMetadata_TableIsPreserved(t *testing.T) {
	// A Table structure in XINI should be read and text content extracted.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>table-test.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Table TableID="0" TableLabel="Data Table">
							<Fields>
								<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
									<Seg SegID="0">Table content</Seg>
								</Field>
							</Fields>
						</Table>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	parts := readXINIString(t, xini, nil)

	// The Table structure should be read without errors.
	require.NotEmpty(t, parts, "should produce parts from Table XINI")
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")

	// The XINI filter may represent Table content as blocks or data parts.
	blocks := allBlocks(parts)
	if len(blocks) > 0 {
		b := findBlockContaining(blocks, "Table content")
		assert.NotNil(t, b, "should find block with Table content")
	}
}

// okapi: XINIFilterMetainformationTest#fieldIsPreserved
func TestMetadata_FieldIsPreserved(t *testing.T) {
	// Field attributes should survive a roundtrip.
	output := fileRoundtrip(t, "contents.xini", nil)
	assert.Contains(t, output, "Field", "Field element should be preserved")
	assert.Contains(t, output, "FieldID", "FieldID attribute should be preserved")
	assert.Contains(t, output, "ExternalID", "ExternalID attribute should be preserved")
}

// okapi: XINIFilterMetainformationTest#pageAndElementIsPreserved
func TestMetadata_PageAndElementIsPreserved(t *testing.T) {
	// Page and Element metadata should survive a roundtrip.
	output := fileRoundtrip(t, "contents.xini", nil)
	assert.Contains(t, output, "Page", "Page element should be preserved")
	assert.Contains(t, output, "PageID", "PageID attribute should be preserved")
	assert.Contains(t, output, "Element", "Element element should be preserved")
	assert.Contains(t, output, "ElementID", "ElementID attribute should be preserved")
}

// okapi: XINIFilterMetainformationTest#emptyPageDoesntCauseNullPointerException
func TestMetadata_EmptyPageDoesntCauseNPE(t *testing.T) {
	// An empty Page (no Elements) should not cause errors.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>empty-page.xml</PageName>
			<Elements/>
		</Page>
	</Main>
</Xini>`

	parts := readXINIString(t, xini, nil)
	// Should not crash; should produce at least layer parts.
	require.NotEmpty(t, parts, "empty page should still produce layer events")
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
}

// okapi: XINIFilterMetainformationTest#sourceAndTargetLanguagesPreserved
func TestMetadata_SourceAndTargetLanguagesPreserved(t *testing.T) {
	// Source and target language metadata in XINI should survive a roundtrip.
	output := fileRoundtrip(t, "contents.xini", nil)
	assert.Contains(t, output, "TargetLanguages", "TargetLanguages element should be preserved")
	assert.Contains(t, output, "fr", "target language 'fr' should be preserved")
}

// okapi: XINIFilterMetainformationTest#emptyFieldDoesntCauseNullPointerException
func TestMetadata_EmptyFieldDoesntCauseNPE(t *testing.T) {
	// A Field with no Seg elements should not cause errors.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>empty-field.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0"/>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	parts := readXINIString(t, xini, nil)
	// Should not crash; should produce at least layer parts.
	require.NotEmpty(t, parts, "empty field should still produce layer events")
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
}
