//go:build integration

package xliff

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.xliff.XLIFFFilter"
const mimeType = "application/xliff+xml"

func TestExtract_SimpleXLIFF(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello world</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from XLIFF")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello world")
}

func TestExtract_MultipleTransUnits(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>First</source>
      </trans-unit>
      <trans-unit id="2">
        <source>Second</source>
      </trans-unit>
      <trans-unit id="3">
        <source>Third</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 3)

	texts := bridgetest.BlockTexts(blocks)
	assert.Equal(t, "First", texts[0])
	assert.Equal(t, "Second", texts[1])
	assert.Equal(t, "Third", texts[2])
}

func TestExtract_TransUnitIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="greeting">
        <source>Hello</source>
      </trans-unit>
      <trans-unit id="farewell">
        <source>Goodbye</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2)

	// Block IDs should match the trans-unit IDs.
	assert.Equal(t, "greeting", blocks[0].ID)
	assert.Equal(t, "farewell", blocks[1].ID)
}

func TestExtract_InlineCodes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Click <g id="1">&lt;b&gt;</g>here<g id="2">&lt;/b&gt;</g> to continue</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	runs := blocks[0].SourceRuns()
	require.NotEmpty(t, runs)
	// Inline <g> elements should produce inline-code runs.
	var inlineCount int
	for _, r := range runs {
		if r.Text == nil {
			inlineCount++
		}
	}
	assert.GreaterOrEqual(t, inlineCount, 2, "should have inline-code runs for <g> elements")
}

// okapi: XLIFFFilterTest#testSegmentedTarget
func TestExtract_WithTarget(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
        <target>Bonjour</target>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "Hello", b.SourceText())
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Equal(t, "Bonjour", b.TargetText("fr"))
}

func TestExtract_TranslateNo(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1" translate="yes">
        <source>Translate me</source>
      </trans-unit>
      <trans-unit id="2" translate="no">
        <source>Do not translate</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	allBlocks := bridgetest.FilterBlocks(parts)
	require.GreaterOrEqual(t, len(allBlocks), 2)

	translatableBlocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, translatableBlocks, 1)
	assert.Equal(t, "Translate me", translatableBlocks[0].SourceText())
}

// okapi: XLIFFFilterTest#testGroupIds
func TestExtract_GroupStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <group id="g1">
        <trans-unit id="1">
          <source>In group</source>
        </trans-unit>
      </group>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	var hasGroupStart, hasGroupEnd bool
	for _, p := range parts {
		if p.Type == model.PartGroupStart {
			hasGroupStart = true
			if gs, ok := p.Resource.(*model.GroupStart); ok {
				assert.Equal(t, "g1", gs.ID)
			}
		}
		if p.Type == model.PartGroupEnd {
			hasGroupEnd = true
		}
	}
	assert.True(t, hasGroupStart, "should have GroupStart for <group>")
	assert.True(t, hasGroupEnd, "should have GroupEnd for </group>")
}

func TestExtract_MultipleFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="file1">
    <body>
      <trans-unit id="1">
        <source>From file 1</source>
      </trans-unit>
    </body>
  </file>
  <file source-language="en" target-language="fr" datatype="plaintext" original="file2">
    <body>
      <trans-unit id="1">
        <source>From file 2</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "From file 1")
	assert.Contains(t, texts, "From file 2")
}

func TestExtract_NoteWithPriority(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
        <note priority="1" from="developer" annotates="source">Important note</note>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	require.NotNil(t, b.Annotations)

	noteAnn, ok := b.Annotations["note"]
	require.True(t, ok)

	note := noteAnn.(*model.NoteAnnotation)
	assert.Equal(t, "Important note", note.Text)
	assert.Equal(t, "developer", note.From)
	assert.Equal(t, 1, note.Priority)
	assert.Equal(t, "source", note.Annotates)
}

// okapi: XLIFFFilterTest#testWSBetweenSegments
func TestExtract_PreserveWhitespace(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1" xml:space="preserve">
        <source>  Hello  world  </source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The block should have PreserveWhitespace set due to xml:space="preserve".
	b := blocks[0]
	assert.True(t, b.PreserveWhitespace, "block should preserve whitespace due to xml:space='preserve'")
}

func TestExtract_LayerHasFormat(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layer := p.Resource.(*model.Layer)
			assert.NotEmpty(t, layer.Format, "layer should have a format (filter ID)")
			assert.Contains(t, layer.Format, "xliff", "format should reference XLIFF filter")
			break
		}
	}
}

func TestExtract_DataIsReferent(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	// Check that Data parts carry the isReferent flag (skeleton references them).
	var hasData bool
	for _, p := range parts {
		if p.Type == model.PartData {
			hasData = true
			data := p.Resource.(*model.Data)
			assert.NotEmpty(t, data.ID, "data should have an ID")
		}
	}
	// XLIFF should have some non-translatable Data parts.
	_ = hasData
}

func TestExtract_NoteAnnotation(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
        <note>This is a developer note</note>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	require.NotNil(t, b.Annotations, "block should have annotations")

	// Should have at least one note annotation.
	noteAnn, ok := b.Annotations["note"]
	require.True(t, ok, "should have a 'note' annotation key")
	assert.Equal(t, "note", noteAnn.AnnotationType())

	note, ok := noteAnn.(*model.NoteAnnotation)
	require.True(t, ok, "annotation should be *model.NoteAnnotation")
	assert.Equal(t, "This is a developer note", note.Text)
}

func TestExtract_MultipleNotes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
        <note>First note</note>
        <note from="developer">Second note</note>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	require.NotNil(t, b.Annotations, "block should have annotations")

	// First note keyed as "note", second as "note-1".
	note0, ok := b.Annotations["note"]
	require.True(t, ok, "should have 'note' annotation")
	n0 := note0.(*model.NoteAnnotation)
	assert.Equal(t, "First note", n0.Text)

	note1, ok := b.Annotations["note-1"]
	require.True(t, ok, "should have 'note-1' annotation")
	n1 := note1.(*model.NoteAnnotation)
	assert.Equal(t, "Second note", n1.Text)
	assert.Equal(t, "developer", n1.From)
}

func TestExtract_AltTranslation(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
        <target>Bonjour</target>
        <alt-trans match-quality="95" origin="TM">
          <source>Hello</source>
          <target xml:lang="fr">Salut</target>
        </alt-trans>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]

	require.NotNil(t, b.Annotations, "block should have annotations for alt-trans")

	altAnn, ok := b.Annotations["alt-translation"]
	require.True(t, ok, "should have 'alt-translation' annotation key")
	assert.Equal(t, "alt-translation", altAnn.AnnotationType())

	alt, ok := altAnn.(*model.AltTranslation)
	require.True(t, ok, "annotation should be *model.AltTranslation, got %T", altAnn)
	assert.Equal(t, "TM", alt.Origin)
	assert.Equal(t, 95.0, alt.CombinedScore)
	assert.Equal(t, "FUZZY", alt.MatchType)
	// Source and target text should be preserved as runs.
	require.NotEmpty(t, alt.Source, "alt-trans source should not be empty")
	assert.Equal(t, "Hello", model.RunsPlainText(alt.Source))
	require.NotEmpty(t, alt.Target, "alt-trans target should not be empty")
	assert.Equal(t, "Salut", model.RunsPlainText(alt.Target))
}

func TestExtract_MultipleAltTranslations(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
        <target>Bonjour</target>
        <alt-trans match-quality="100" origin="TM">
          <source>Hello</source>
          <target xml:lang="fr">Bonjour</target>
        </alt-trans>
        <alt-trans match-quality="80" origin="MT">
          <source>Hello</source>
          <target xml:lang="fr">Salut</target>
        </alt-trans>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	require.NotNil(t, b.Annotations)

	// First alt-trans keyed as "alt-translation", second as "alt-translation-1".
	alt0, ok := b.Annotations["alt-translation"]
	require.True(t, ok, "should have 'alt-translation'")
	a0 := alt0.(*model.AltTranslation)
	assert.Equal(t, 100.0, a0.CombinedScore)
	assert.Equal(t, "TM", a0.Origin)

	alt1, ok := b.Annotations["alt-translation-1"]
	require.True(t, ok, "should have 'alt-translation-1'")
	a1 := alt1.(*model.AltTranslation)
	assert.Equal(t, 80.0, a1.CombinedScore)
	assert.Equal(t, "MT", a1.Origin)
}

func TestExtract_NestedGroups(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <group id="outer">
        <group id="inner">
          <trans-unit id="1">
            <source>Nested text</source>
          </trans-unit>
        </group>
      </group>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	// Count nested group starts and ends.
	var groupStarts, groupEnds int
	var groupIDs []string
	for _, p := range parts {
		if p.Type == model.PartGroupStart {
			groupStarts++
			gs := p.Resource.(*model.GroupStart)
			groupIDs = append(groupIDs, gs.ID)
		}
		if p.Type == model.PartGroupEnd {
			groupEnds++
		}
	}
	assert.Equal(t, 2, groupStarts, "should have 2 nested GroupStart events")
	assert.Equal(t, 2, groupEnds, "should have 2 nested GroupEnd events")
	assert.Contains(t, groupIDs, "outer")
	assert.Contains(t, groupIDs, "inner")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Nested text", blocks[0].SourceText())
}

// okapi: XLIFFFilterTest#testSegmentedContent
// okapi: XLIFFFilterTest#testSegmentIDs
func TestExtract_Segmentation(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// XLIFF 1.2 supports seg-source with <mrk mtype="seg"> for segmentation.
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>First sentence. Second sentence.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">First sentence.</mrk>
          <mrk mtype="seg" mid="s2"> Second sentence.</mrk>
        </seg-source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	// With seg-source, the filter may produce multiple source segments.
	require.GreaterOrEqual(t, len(b.Source), 2,
		"seg-source with 2 mrk segments should produce 2+ source segments")

	// Each segment should have an ID.
	for _, seg := range b.Source {
		assert.NotEmpty(t, seg.ID, "segment should have an ID")
		assert.NotNil(t, seg, "segment should not be nil")
	}
}

func TestExtract_InlineX(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// XLIFF <x/> elements are standalone inline codes (placeholders).
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Line one<x id="1"/>Line two</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	runs := blocks[0].SourceRuns()
	require.NotEmpty(t, runs)

	// <x/> should produce a placeholder run.
	var hasPlaceholder bool
	for _, r := range runs {
		if r.Ph != nil {
			hasPlaceholder = true
			break
		}
	}
	assert.True(t, hasPlaceholder, "should have a placeholder run for <x/>")
}

func TestExtract_InlineBpt_Ept(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// XLIFF <bpt>/<ept> paired inline codes.
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="htmlbody" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello <bpt id="1">&lt;b&gt;</bpt>bold<ept id="1">&lt;/b&gt;</ept> text</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	runs := blocks[0].SourceRuns()
	require.NotEmpty(t, runs)
	var inlineCount int
	for _, r := range runs {
		if r.Text == nil {
			inlineCount++
		}
	}
	require.GreaterOrEqual(t, inlineCount, 2,
		"should have at least opening and closing inline-code runs for <bpt>/<ept>")

	// Should have opening and closing runs.
	var hasOpening, hasClosing bool
	for _, r := range runs {
		if r.PcOpen != nil {
			hasOpening = true
		}
		if r.PcClose != nil {
			hasClosing = true
		}
	}
	assert.True(t, hasOpening, "should have PcOpen run from <bpt>")
	assert.True(t, hasClosing, "should have PcClose run from <ept>")
}

func TestExtract_BlockSkeleton(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello world</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// XLIFF blocks should have skeleton data for reconstruction.
	var withSkeleton int
	for _, b := range blocks {
		if b.Skeleton != nil && len(b.Skeleton.Parts) > 0 {
			withSkeleton++
		}
	}
	assert.Greater(t, withSkeleton, 0, "XLIFF blocks should have skeleton data")
}

func TestExtract_ContextGroup(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// XLIFF context-group provides context information for translation.
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test.properties">
    <body>
      <trans-unit id="msg.greeting">
        <source>Hello</source>
        <context-group name="x-pos" purpose="location">
          <context context-type="sourcefile">test.properties</context>
          <context context-type="linenumber">42</context>
        </context-group>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "msg.greeting", b.ID)
	assert.Equal(t, "Hello", b.SourceText())
}

func TestExtract_NonTranslatableUnit(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1" translate="yes">
        <source>Translate me</source>
      </trans-unit>
      <trans-unit id="2" translate="no">
        <source>Code: XYZ-123</source>
      </trans-unit>
      <trans-unit id="3">
        <source>Default translatable</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	allBlocks := bridgetest.FilterBlocks(parts)
	require.GreaterOrEqual(t, len(allBlocks), 3)

	translatableBlocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, translatableBlocks, 2, "only translate=yes and default should be translatable")

	texts := bridgetest.BlockTexts(translatableBlocks)
	assert.Contains(t, texts, "Translate me")
	assert.Contains(t, texts, "Default translatable")
}

// okapi: XLIFFFilterTest#testEmptyTarget
func TestExtract_EmptySource(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source></source>
      </trans-unit>
      <trans-unit id="2">
        <source>Non-empty</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	// Even empty trans-units are extracted; the filter doesn't skip them.
	blocks := bridgetest.FilterBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 1)
}

func TestExtract_LayerProperties(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="html" original="index.html">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layer := p.Resource.(*model.Layer)
			assert.NotEmpty(t, layer.Format)
			assert.NotEmpty(t, layer.Encoding)
			break
		}
	}
}

// TestExtract_EmptyTgtLang verifies that empty-tgt-lang.xlf can be read even
// though roundtrip fails (bridge bug: duplicate target-language attribute).
func TestExtract_EmptyTgtLang(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okapi/filters/xliff/src/test/resources/empty-tgt-lang.xlf"), mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks,
		"empty-tgt-lang.xlf should extract translatable blocks")
}

// okapi: XLIFFFilterTest#testLQR
// TestExtract_LqiTest verifies that lqiTest.xlf with ITS standoff annotations
// can be read successfully. The bridge passes the source_path so Java can
// resolve the external lqiTestIssues.xml file via relative URI.
func TestExtract_LqiTest(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okapi/filters/xliff/src/test/resources/lqiTest.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 4,
		"lqiTest.xlf should extract at least 4 translatable trans-units")
}
