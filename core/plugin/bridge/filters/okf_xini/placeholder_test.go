//go:build integration

package okf_xini

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// XINIFilterPlaceholderTest
// ---------------------------------------------------------------------------

// okapi: XINIFilterPlaceholderTest#placeholdersBecomePreserved
func TestPlaceholder_PlaceholdersBecomePreserved(t *testing.T) {
	// Placeholders (ph elements) should survive a roundtrip.
	// contents.xini has <ph type="ph" ID="2"/> placeholders.
	output := fileRoundtrip(t, "contents.xini", nil)
	assert.Contains(t, output, "ph", "placeholder elements should be preserved in roundtrip output")
	assert.Contains(t, output, "Test!", "text content should be preserved")
}

// okapi: XINIFilterPlaceholderTest#placeholdersBecomeCodes
func TestPlaceholder_PlaceholdersBecomeCodes(t *testing.T) {
	// Placeholders (ph elements) should become inline codes (spans) in blocks.
	// contents.xini has <sph>, <eph>, <ph> elements.
	parts := readXINIDefault(t, "contents.xini")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks")

	// At least one block should have inline code spans from placeholders.
	hasSpans := false
	for _, b := range blocks {
		if spanCount(b) > 0 {
			hasSpans = true
			break
		}
	}
	assert.True(t, hasSpans, "placeholders should produce inline code spans in at least one block")
}

// okapi: XINIFilterPlaceholderTest#isolatedPlaceholdersBecomeCodes
func TestPlaceholder_IsolatedPlaceholdersBecomeCodes(t *testing.T) {
	// Isolated (unpaired) placeholders should also become inline codes.
	// contents.xini has <ph type="ph" ID="2"/> which is isolated.
	parts := readXINIDefault(t, "contents.xini")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks")

	// Check for placeholder-type spans.
	hasPlaceholder := false
	for _, b := range blocks {
		if hasSpanOfType(b, model.SpanPlaceholder) {
			hasPlaceholder = true
			break
		}
	}
	assert.True(t, hasPlaceholder, "isolated placeholders should produce SpanPlaceholder spans")
}

// okapi: XINIFilterPlaceholderTest#phTypeDeletedPreserved
func TestPlaceholder_PhTypeDeletedPreserved(t *testing.T) {
	// A placeholder with type="deleted" should be preserved through roundtrip.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>ph-deleted.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">Before <ph type="deleted" ID="1"/>after</Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "deleted", "placeholder type 'deleted' should be preserved")
}

// okapi: XINIFilterPlaceholderTest#phTypeMemory100Preserved
func TestPlaceholder_PhTypeMemory100Preserved(t *testing.T) {
	// A placeholder with type="memory100" should be preserved through roundtrip.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>ph-memory.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">Before <ph type="memory100" ID="1"/>after</Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "memory100", "placeholder type 'memory100' should be preserved")
}

// okapi: XINIFilterPlaceholderTest#phTypeUpdatedPreserved
func TestPlaceholder_PhTypeUpdatedPreserved(t *testing.T) {
	// A placeholder with type="updated" should be preserved through roundtrip.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>ph-updated.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">Before <ph type="updated" ID="1"/>after</Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "updated", "placeholder type 'updated' should be preserved")
}

// okapi: XINIFilterPlaceholderTest#phTypeInsertedPreserved
func TestPlaceholder_PhTypeInsertedPreserved(t *testing.T) {
	// A placeholder with type="inserted" should be preserved through roundtrip.
	xini := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Xini SchemaVersion="1.0">
	<TargetLanguages>
		<Language>fr</Language>
	</TargetLanguages>
	<Main>
		<Page PageID="1">
			<PageName>ph-inserted.xml</PageName>
			<Elements>
				<Element ElementID="10">
					<ElementContent>
						<Fields>
							<Field EmptySegmentsFlags="0" ExternalID="tu1" FieldID="0">
								<Seg SegID="0">Before <ph type="inserted" ID="1"/>after</Seg>
							</Field>
						</Fields>
					</ElementContent>
				</Element>
			</Elements>
		</Page>
	</Main>
</Xini>`

	output := snippetRoundtrip(t, xini, nil)
	assert.Contains(t, output, "inserted", "placeholder type 'inserted' should be preserved")
}
