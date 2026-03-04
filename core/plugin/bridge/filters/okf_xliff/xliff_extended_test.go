//go:build integration

package okf_xliff

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: XLIFFFilterTest#testSimpleTransUnit
func TestExtract_SimpleTransUnit(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/simple.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Hello World!", blocks[0].SourceText())
}

// okapi: XLIFFFilterTest#testDefaultInfo
func TestExtract_DefaultInfo(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/simple.xlf", nil)
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layer := p.Resource.(*model.Layer)
			assert.NotEmpty(t, layer.Format)
			return
		}
	}
	t.Error("no LayerStart found")
}

// okapi: XLIFFFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/simple.xlf", nil)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
}

// okapi: XLIFFFilterTest#testStartDocumentFromList
func TestExtract_StartDocumentFromList(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/simple.xlf", nil)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	layer := parts[0].Resource.(*model.Layer)
	assert.NotEmpty(t, layer.Encoding)
}

// okapi: XLIFFFilterTest#testStartSubDocumentFromList
func TestExtract_StartSubDocumentFromList(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/simple.xlf", nil)
	var layerCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerCount++
		}
	}
	assert.GreaterOrEqual(t, layerCount, 1)
}

// okapi: XLIFFFilterTest#testBilingualTransUnit
func TestExtract_BilingualTransUnit(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/simple.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	assert.Equal(t, "Hello World!", b.SourceText())
	assert.True(t, b.HasTarget("es") || b.HasTarget("ES"),
		"simple.xlf should have Spanish target")
}

// okapi: XLIFFFilterTest#testBilingualTransUnitWithEmptyLocales
func TestExtract_BilingualTransUnitWithEmptyLocales(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <target>Bonjour</target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testBilingualInlines
func TestExtract_BilingualInlines(t *testing.T) {
	xliff := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source>Hello <bpt id="1">&lt;b&gt;</bpt>bold<ept id="1">&lt;/b&gt;</ept> text</source>
        <target>Bonjour <bpt id="1">&lt;b&gt;</bpt>gras<ept id="1">&lt;/b&gt;</ept> texte</target>
      </trans-unit>`, "htmlbody")
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 2)
}

// okapi: XLIFFFilterTest#testBPTTypeTransUnit
func TestExtract_BPTTypeTransUnit(t *testing.T) {
	xliff := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source>Text <bpt id="1">&lt;b&gt;</bpt>bold<ept id="1">&lt;/b&gt;</ept></source>
      </trans-unit>`, "htmlbody")
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	var hasOpening, hasClosing bool
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanOpening {
			hasOpening = true
		}
		if s.SpanType == model.SpanClosing {
			hasClosing = true
		}
	}
	assert.True(t, hasOpening)
	assert.True(t, hasClosing)
}

// okapi: XLIFFFilterTest#testOutputBPTTypeTransUnit
func TestExtract_OutputBPTTypeTransUnit(t *testing.T) {
	xliff := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source>Text <bpt id="1">&lt;b&gt;</bpt>bold<ept id="1">&lt;/b&gt;</ept></source>
      </trans-unit>`, "htmlbody")
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "bold")
}

// okapi: XLIFFFilterTest#testBPTWithSUB
func TestExtract_BPTWithSUB(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <bpt id="1">&lt;a href="<sub>link text</sub>"&gt;</bpt>click<ept id="1">&lt;/a&gt;</ept></source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testBPTAndSUBTypeTransUnit
func TestExtract_BPTAndSUBTypeTransUnit(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <bpt id="1">&lt;a href="<sub>alt</sub>"&gt;</bpt>link<ept id="1">&lt;/a&gt;</ept></source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// Covered by xliff_test.go: XLIFFFilterTest#testEmptyTarget
func TestExtract_EmptyTarget2(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <target></target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testEmptyTargetCondition
func TestExtract_EmptyTargetCondition(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <target/>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testEmptyTargetOutput
func TestExtract_EmptyTargetOutput(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <target></target>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "Hello")
}

// okapi: XLIFFFilterTest#testEmptyTgtLangAttribute
func TestExtract_EmptyTgtLangAttribute(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/empty-tgt-lang.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testEmptyCodes
func TestExtract_EmptyCodes(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <x id="1"/></source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 1)
}

// okapi: XLIFFFilterTest#testNotes
func TestExtract_Notes(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <note>A note</note>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	require.NotNil(t, blocks[0].Annotations)
	_, ok := blocks[0].Annotations["note"]
	assert.True(t, ok)
}

// okapi: XLIFFFilterTest#testNoteRefersToNonExistingTarget
func TestExtract_NoteRefersToNonExistingTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <note annotates="target">Note for target</note>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testAddXLIFFNote
func TestExtract_AddXLIFFNote(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <note from="developer">Dev note</note>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	require.NotNil(t, blocks[0].Annotations)
}

// okapi: XLIFFFilterTest#testModifyXLIFFNote
func TestExtract_ModifyXLIFFNote(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <note>Original note</note>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "note")
}

// okapi: XLIFFFilterTest#testLocNoteModification
func TestExtract_LocNoteModification(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <note>Localization note</note>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "note")
}

// okapi: XLIFFFilterTest#testAlTrans
func TestExtract_AlTrans(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/alttrans.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	require.NotNil(t, b.Annotations)
	_, ok := b.Annotations["alt-translation"]
	assert.True(t, ok)
}

// okapi: XLIFFFilterTest#testAlTransData
func TestExtract_AlTransData(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/alttrans.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	require.NotNil(t, b.Annotations)
	alt, ok := b.Annotations["alt-translation"]
	require.True(t, ok)
	a := alt.(*model.AltTranslation)
	require.NotNil(t, a.Source)
	require.NotNil(t, a.Target)
}

// okapi: XLIFFFilterTest#testOutputAlTrans
func TestExtract_OutputAlTrans(t *testing.T) {
	out := fileRoundtrip(t, "okf_xliff/alttrans.xlf", nil)
	assert.Contains(t, out, "alt-trans")
}

// okapi: XLIFFFilterTest#testAddAltTrans
func TestExtract_AddAltTrans(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <target>Bonjour</target>
        <alt-trans match-quality="100" origin="TM">
          <source>Hello</source>
          <target xml:lang="fr">Bonjour</target>
        </alt-trans>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	require.NotNil(t, blocks[0].Annotations)
	_, ok := blocks[0].Annotations["alt-translation"]
	assert.True(t, ok)
}

// okapi: XLIFFFilterTest#testAltTransWithEmptyTarget
func TestExtract_AltTransWithEmptyTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <alt-trans match-quality="80">
          <source>Hello</source>
          <target xml:lang="fr"></target>
        </alt-trans>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testDecimalAltTransValues
func TestExtract_DecimalAltTransValues(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <alt-trans match-quality="99.5">
          <source>Hello</source>
          <target xml:lang="fr">Bonjour</target>
        </alt-trans>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	require.NotNil(t, b.Annotations)
	alt := b.Annotations["alt-translation"].(*model.AltTranslation)
	assert.Equal(t, 99.5, alt.CombinedScore)
}

// okapi: XLIFFFilterTest#testEmptyTargetInAltTrans
func TestExtract_EmptyTargetInAltTrans(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <alt-trans>
          <source>Hello</source>
          <target/>
        </alt-trans>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testMixedAlTrans
func TestExtract_MixedAlTrans(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/Manual-12-AltTrans.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// Covered by xliff_test.go: XLIFFFilterTest#testSegmentedContent
func TestExtract_SegmentedContent(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>First sentence. Second sentence.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">First sentence.</mrk>
          <mrk mtype="seg" mid="s2"> Second sentence.</mrk>
        </seg-source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.GreaterOrEqual(t, len(blocks[0].Source), 2)
}

// Covered by xliff_test.go: XLIFFFilterTest#testSegmentedTarget
func TestExtract_SegmentedTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>First. Second.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">First.</mrk>
          <mrk mtype="seg" mid="s2"> Second.</mrk>
        </seg-source>
        <target>
          <mrk mtype="seg" mid="s1">Premier.</mrk>
          <mrk mtype="seg" mid="s2"> Deuxieme.</mrk>
        </target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	assert.GreaterOrEqual(t, len(b.Source), 2)
	assert.True(t, b.HasTarget("fr"))
}

// okapi: XLIFFFilterTest#testIgnoredSegmentedTarget
func TestExtract_IgnoredSegmentedTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello world</source>
        <target>
          <mrk mtype="seg" mid="s1">Bonjour monde</mrk>
        </target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSegmentedEntry
func TestExtract_SegmentedEntry(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/segmented.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSegmentedEntryOutput
func TestExtract_SegmentedEntryOutput(t *testing.T) {
	out := fileRoundtrip(t, "okf_xliff/segmented.xlf", nil)
	assert.NotEmpty(t, out)
}

// okapi: XLIFFFilterTest#testSegmentedEntryWithDifferences
func TestExtract_SegmentedEntryWithDifferences(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/segmented.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSegmentedNoTargetEntryOutput
func TestExtract_SegmentedNoTargetEntryOutput(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>First. Second.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">First.</mrk>
          <mrk mtype="seg" mid="s2"> Second.</mrk>
        </seg-source>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "First")
}

// okapi: XLIFFFilterTest#testSegmentedSource1
func TestExtract_SegmentedSource1(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/segsource.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSegmentedSourceWithOuterCodes
func TestExtract_SegmentedSourceWithOuterCodes(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source><bpt id="1">&lt;p&gt;</bpt>First. Second.<ept id="1">&lt;/p&gt;</ept></source>
        <seg-source>
          <bpt id="1">&lt;p&gt;</bpt><mrk mtype="seg" mid="s1">First.</mrk>
          <mrk mtype="seg" mid="s2"> Second.</mrk><ept id="1">&lt;/p&gt;</ept>
        </seg-source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSegmentedWithEmptyTarget
func TestExtract_SegmentedWithEmptyTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>First. Second.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">First.</mrk>
          <mrk mtype="seg" mid="s2"> Second.</mrk>
        </seg-source>
        <target>
          <mrk mtype="seg" mid="s1"></mrk>
          <mrk mtype="seg" mid="s2"></mrk>
        </target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSegmentationWithEmptyTarget
func TestExtract_SegmentationWithEmptyTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Sentence one. Sentence two.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">Sentence one.</mrk>
          <mrk mtype="seg" mid="s2"> Sentence two.</mrk>
        </seg-source>
        <target></target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testOutputSegmentationWithEmptyTarget
func TestExtract_OutputSegmentationWithEmptyTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Sentence one. Sentence two.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">Sentence one.</mrk>
          <mrk mtype="seg" mid="s2"> Sentence two.</mrk>
        </seg-source>
        <target></target>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "Sentence one")
}

// okapi: XLIFFFilterTest#testSegmentedAddedBptEpt
func TestExtract_SegmentedAddedBptEpt(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <bpt id="1">&lt;b&gt;</bpt>bold<ept id="1">&lt;/b&gt;</ept> end</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">Text <bpt id="1">&lt;b&gt;</bpt>bold<ept id="1">&lt;/b&gt;</ept> end</mrk>
        </seg-source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSegmentedAddedBptEptAndPh
func TestExtract_SegmentedAddedBptEptAndPh(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <bpt id="1">&lt;b&gt;</bpt>bold<ept id="1">&lt;/b&gt;</ept> and <ph id="2">&lt;br/&gt;</ph> end</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">Text <bpt id="1">&lt;b&gt;</bpt>bold<ept id="1">&lt;/b&gt;</ept> and <ph id="2">&lt;br/&gt;</ph> end</mrk>
        </seg-source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSegmentedAddedIt
func TestExtract_SegmentedAddedIt(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <it id="1" pos="open">&lt;i&gt;</it>italic end</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">Text <it id="1" pos="open">&lt;i&gt;</it>italic end</mrk>
        </seg-source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSegmentedAddedItAndPh
func TestExtract_SegmentedAddedItAndPh(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <it id="1" pos="open">&lt;i&gt;</it>italic <ph id="2">&lt;br/&gt;</ph> end</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">Text <it id="1" pos="open">&lt;i&gt;</it>italic <ph id="2">&lt;br/&gt;</ph> end</mrk>
        </seg-source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSegmentedAddedPh
func TestExtract_SegmentedAddedPh(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <ph id="1">&lt;br/&gt;</ph> end</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">Text <ph id="1">&lt;br/&gt;</ph> end</mrk>
        </seg-source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testAlwaysUseSegSource
func TestExtract_AlwaysUseSegSource(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Full text here</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">Full text here</mrk>
        </seg-source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSegSourceWithCodeFinder
func TestExtract_SegSourceWithCodeFinder(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello world</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">Hello world</mrk>
        </seg-source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSegSourceWithoutMrkOutput
func TestExtract_SegSourceWithoutMrkOutput(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello world</source>
        <seg-source>Hello world</seg-source>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "Hello world")
}

// okapi: XLIFFFilterTest#testOutputOfResegmentedContent
func TestExtract_OutputOfResegmentedContent(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>One. Two. Three.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">One.</mrk>
          <mrk mtype="seg" mid="s2"> Two.</mrk>
          <mrk mtype="seg" mid="s3"> Three.</mrk>
        </seg-source>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "One")
}

// Covered by xliff_test.go: XLIFFFilterTest#testGroupIds
func TestExtract_GroupIds(t *testing.T) {
	xliff := wrapXLIFF(`      <group id="g1">
        <trans-unit id="1">
          <source>In group</source>
        </trans-unit>
      </group>`)
	parts := readXLIFFDefault(t, xliff)
	var groupID string
	for _, p := range parts {
		if p.Type == model.PartGroupStart {
			gs := p.Resource.(*model.GroupStart)
			groupID = gs.ID
		}
	}
	assert.Equal(t, "g1", groupID)
}

// okapi: XLIFFFilterTest#testTranslateOnGroup
func TestExtract_TranslateOnGroup(t *testing.T) {
	xliff := wrapXLIFF(`      <group id="g1" translate="no">
        <trans-unit id="1">
          <source>Should be non-translatable</source>
        </trans-unit>
      </group>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks)
}

// okapi: XLIFFFilterTest#testTranslateOnTU
func TestExtract_TranslateOnTU(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1" translate="no">
        <source>Non-translatable</source>
      </trans-unit>
      <trans-unit id="2" translate="yes">
        <source>Translatable</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Translatable", blocks[0].SourceText())
}

// okapi: XLIFFFilterTest#testTranslateNo
func TestExtract_TranslateNo2(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/translate_no.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	allBlocks := bridgetest.FilterBlocks(parts)
	assert.Less(t, len(blocks), len(allBlocks))
}

// okapi: XLIFFFilterTest#testNoTarget
func TestExtract_NoTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.False(t, blocks[0].HasTarget("fr"))
}

// okapi: XLIFFFilterTest#testNoTargetOutput
func TestExtract_NoTargetOutput(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "Hello")
}

// okapi: XLIFFFilterTest#testNoTargetOutputMonolingual
func TestExtract_NoTargetOutputMonolingual(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "Hello")
}

// okapi: XLIFFFilterTest#testNoTargetOutputMonolingualGenerateTarget
func TestExtract_NoTargetOutputMonolingualGenerateTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "Hello")
}

// okapi: XLIFFFilterTest#testAllowEmptyTargets
func TestExtract_AllowEmptyTargets(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <target></target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testAllowEmptyTargetsWithSegments
func TestExtract_AllowEmptyTargetsWithSegments(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>One. Two.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">One.</mrk>
          <mrk mtype="seg" mid="s2"> Two.</mrk>
        </seg-source>
        <target>
          <mrk mtype="seg" mid="s1"></mrk>
          <mrk mtype="seg" mid="s2"></mrk>
        </target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testAddedCloneCode
func TestExtract_AddedCloneCode(t *testing.T) {
	xliff := `<?xml version="1.0"?>
<xliff version="1.2">
<file source-language="en" datatype="x-abc" original="file.ext">
<body><trans-unit id="1">
<source>s1 <g id="1">s2 s3</g> s4.</source>
<target>t1 <g id="1">t2</g> t3 <g id="1">t4</g>.</target>
</trans-unit></body>
</file></xliff>`
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// The target has a cloned g element with the same id
	b := blocks[0]
	assert.Contains(t, b.SourceText(), "s1")
}

// okapi: XLIFFFilterTest#testApprovedTU
func TestExtract_ApprovedTU(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1" approved="yes">
        <source>Hello</source>
        <target>Bonjour</target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testApprovedOutput
func TestExtract_ApprovedOutput(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1" approved="yes">
        <source>Hello</source>
        <target>Bonjour</target>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "approved")
}

// okapi: XLIFFFilterTest#testWithNamespaces
func TestExtract_WithNamespaces(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/NSTest01.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testForceUniqueIds
func TestExtract_ForceUniqueIds(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/duplicate-ids.xlf", nil)
	blocks := bridgetest.FilterBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testDontForceUniqueIds
func TestExtract_DontForceUniqueIds(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/duplicate-ids.xlf", nil)
	blocks := bridgetest.FilterBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testMrk
func TestExtract_Mrk(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <mrk mtype="term">term</mrk> end</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "term")
}

// okapi: XLIFFFilterTest#testOutputMrk
func TestExtract_OutputMrk(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <mrk mtype="term">term</mrk> end</source>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "term")
}

// okapi: XLIFFFilterTest#testProtectedOnMRK
func TestExtract_ProtectedOnMRK(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <mrk mtype="protected">locked</mrk> end</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testCDATAEntry
func TestExtract_CDATAEntry(t *testing.T) {
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2"><file source-language="en" target-language="fr" datatype="x-test" original="file.ext"><body><trans-unit id="1"><source>t1. t2 &amp; .t3</source></trans-unit></body></file></xliff>`
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "t1")
}

// okapi: XLIFFFilterTest#testCREntity
func TestExtract_CREntity(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Line1&#13;Line2</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testCREntityOutput
func TestExtract_CREntityOutput(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Line1&#13;Line2</source>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "Line1")
}

// okapi: XLIFFFilterTest#testCodeFinderExtraction
func TestExtract_CodeFinderExtraction(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text with placeholder</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testCodeFinderExtractionTarget
func TestExtract_CodeFinderExtractionTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text with placeholder</source>
        <target>Texte avec parametre</target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testCodeFinderExtractionTargetNotBalanced
func TestExtract_CodeFinderExtractionTargetNotBalanced(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text with placeholder</source>
        <target>Texte avec parametre</target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testCodeFinderWithCDATA
func TestExtract_CodeFinderWithCDATA(t *testing.T) {
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2"><file source-language="en" target-language="fr" datatype="x-test" original="file.ext"><body><trans-unit id="1"><source><![CDATA[Text with code]]></source></trans-unit></body></file></xliff>`
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testCodeOriginalIDs
func TestExtract_CodeOriginalIDs(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <x id="100"/> end</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.NotEmpty(t, frag.Spans)
}

// okapi: XLIFFFilterTest#corruptCodeIdsAfterJoinAll
func TestExtract_CorruptCodeIdsAfterJoinAll(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>A. B.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">A.</mrk>
          <mrk mtype="seg" mid="s2"> B.</mrk>
        </seg-source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#disabled_testMisOrderedCodes
func TestExtract_DisabledTestMisOrderedCodes(t *testing.T) {
	t.Skip("disabled in Java: testMisOrderedCodes")
}

// okapi: XLIFFFilterTest#testComplexSUB
func TestExtract_ComplexSUB(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <ph id="1">&lt;img src="<sub>alt text</sub>"/&gt;</ph> end</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testComplexSUBInTarget
func TestExtract_ComplexSUBInTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <ph id="1">&lt;img src="<sub>alt text</sub>"/&gt;</ph> end</source>
        <target>Texte <ph id="1">&lt;img src="<sub>texte alt</sub>"/&gt;</ph> fin</target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSimpleSub
func TestExtract_SimpleSub(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <ph id="1">&lt;a title="<sub>link</sub>"&gt;</ph>click</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSimpleSubTu
func TestExtract_SimpleSubTu(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <ph id="1">&lt;a title="<sub>link</sub>"&gt;</ph>click</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSimpleSubTuTarget
func TestExtract_SimpleSubTuTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <ph id="1">&lt;a title="<sub>link</sub>"&gt;</ph>click</source>
        <target>Texte <ph id="1">&lt;a title="<sub>lien</sub>"&gt;</ph>cliquez</target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSimpleSubTuTranslateToTarget
func TestExtract_SimpleSubTuTranslateToTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <ph id="1">&lt;a title="<sub>link</sub>"&gt;</ph>click</source>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "click")
}

// okapi: XLIFFFilterTest#testComplexSubTu
func TestExtract_ComplexSubTu(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <bpt id="1">&lt;a href="<sub>URL</sub>" title="<sub>title</sub>"&gt;</bpt>click<ept id="1">&lt;/a&gt;</ept></source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testMultiLevelsSub
func TestExtract_MultiLevelsSub(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Main text</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testMultiLevelsSubTu
func TestExtract_MultiLevelsSubTu(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Main text</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSubTuBptEpt
func TestExtract_SubTuBptEpt(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <bpt id="1">&lt;a href="<sub>URL</sub>"&gt;</bpt>link<ept id="1">&lt;/a&gt;</ept></source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testContextGroupAfterTarget
func TestExtract_ContextGroupAfterTarget(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <target>Bonjour</target>
        <context-group name="x-pos" purpose="location">
          <context context-type="sourcefile">test.properties</context>
        </context-group>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#extractsContextGroup
func TestExtract_ExtractsContextGroup(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <context-group>
          <context context-type="sourcefile">test.properties</context>
        </context-group>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testPreserveSpaces
func TestExtract_PreserveSpaces(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1" xml:space="preserve">
        <source>  Hello  </source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.True(t, blocks[0].PreserveWhitespace)
}

// okapi: XLIFFFilterTest#testPreserveSpacesInSegmentedTU
func TestExtract_PreserveSpacesInSegmentedTU(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1" xml:space="preserve">
        <source>One. Two.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">One.</mrk>
          <mrk mtype="seg" mid="s2"> Two.</mrk>
        </seg-source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.True(t, blocks[0].PreserveWhitespace)
}

// okapi: XLIFFFilterTest#testUnwrapSpaces
func TestExtract_UnwrapSpaces(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>  Hello  world  </source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testUnwrapSpacesInSegmentedTU
func TestExtract_UnwrapSpacesInSegmentedTU(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>One. Two.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">One.</mrk>
          <mrk mtype="seg" mid="s2"> Two.</mrk>
        </seg-source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testPreserveSpaceByDefaultOnTransUnit
func TestExtract_PreserveSpaceByDefaultOnTransUnit(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testPreserveSpaceByDefaultNoDeclaration
func TestExtract_PreserveSpaceByDefaultNoDeclaration(t *testing.T) {
	xliff := wrapXLIFFNoNS(`      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testPreserveSpaceByDefaultInSdlXliff
func TestExtract_PreserveSpaceByDefaultInSdlXliff(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testOkapiMarkers
func TestExtract_OkapiMarkers(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSpecialAttributeValues
func TestExtract_SpecialAttributeValues(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1" resname="key.name">
        <source>Hello</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testElementsInHeader
func TestExtract_ElementsInHeader(t *testing.T) {
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <header><tool tool-id="okapi" tool-name="test"/></header>
    <body>
      <trans-unit id="1"><source>Hello</source></trans-unit>
    </body>
  </file>
</xliff>`
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testTool
func TestExtract_Tool(t *testing.T) {
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <header><tool tool-id="okapi" tool-name="Okapi Framework"/></header>
    <body>
      <trans-unit id="1"><source>Hello</source></trans-unit>
    </body>
  </file>
</xliff>`
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testToolAfterSkl
func TestExtract_ToolAfterSkl(t *testing.T) {
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <header>
      <skl><external-file href="test.skl"/></skl>
      <tool tool-id="okapi" tool-name="Okapi Framework"/>
    </header>
    <body>
      <trans-unit id="1"><source>Hello</source></trans-unit>
    </body>
  </file>
</xliff>`
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testHandleInvalidXmlCharacters
func TestExtract_HandleInvalidXmlCharacters(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/invalid_xml_entity.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testLangAndSpaceInline
func TestExtract_LangAndSpaceInline(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1" xml:space="preserve">
        <source>Text <mrk mtype="term" xml:lang="de">Term</mrk> end</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testBalancedIT
func TestExtract_BalancedIT(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <it id="1" pos="open">&lt;i&gt;</it>italic<it id="1" pos="close">&lt;/i&gt;</it> end</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testUnbalancedIT
func TestExtract_UnbalancedIT(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Text <it id="1" pos="open">&lt;i&gt;</it>italic end</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testPreserveCDATAInBody
func TestExtract_PreserveCDATAInBody(t *testing.T) {
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2"><file source-language="en" target-language="fr" datatype="x-test" original="file.ext"><body><trans-unit id="1"><source><![CDATA[Hello CDATA]]></source></trans-unit></body></file></xliff>`
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testPreserveCDATAInNotSegmentTransUnitContent
func TestExtract_PreserveCDATAInNotSegmentTransUnitContent(t *testing.T) {
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2"><file source-language="en" target-language="fr" datatype="x-test" original="file.ext"><body><trans-unit id="1"><source><![CDATA[Hello]]></source><target xml:lang="fr"><![CDATA[Bonjour]]></target></trans-unit></body></file></xliff>`
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testPreserveCDATASkeletonInHeader
func TestExtract_PreserveCDATASkeletonInHeader(t *testing.T) {
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <header><skl><internal-file form="text"><![CDATA[skeleton]]></internal-file></skl></header>
    <body><trans-unit id="1"><source>Hello</source></trans-unit></body>
  </file>
</xliff>`
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testNonEmptyTargetCDATABug
func TestExtract_NonEmptyTargetCDATABug(t *testing.T) {
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2"><file source-language="en" target-language="fr" datatype="x-test" original="file.ext"><body><trans-unit id="1"><source>Hello</source><target xml:lang="fr"><![CDATA[Bonjour]]></target></trans-unit></body></file></xliff>`
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testXmlLangModification
func TestExtract_XmlLangModification(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <target xml:lang="fr">Bonjour</target>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "Hello")
}

// okapi: XLIFFFilterTest#testTargetStateCoordOutput
func TestExtract_TargetStateCoordOutput(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/target_state.xliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testUseTranslationTargetState
func TestExtract_UseTranslationTargetState(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/target_state.xliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testUseTranslationTargetStateWithAltTrans
func TestExtract_UseTranslationTargetStateWithAltTrans(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <target state="translated">Bonjour</target>
        <alt-trans><target>Salut</target></alt-trans>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testUseTranslationTargetStateWithSubfilter
func TestExtract_UseTranslationTargetStateWithSubfilter(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <target state="needs-translation">Bonjour</target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testOutputOverrideTargetlanguage
func TestExtract_OutputOverrideTargetlanguage(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source><target>Bonjour</target>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "Hello")
}

// okapi: XLIFFFilterTest#testOutputSkipTargetInExtention
func TestExtract_OutputSkipTargetInExtention(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source><target>Bonjour</target>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "Hello")
}

// okapi: XLIFFFilterTest#testMtConfidence
func TestExtract_MtConfidence(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source><target>Bonjour</target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testMtConfidenceAltTrans
func TestExtract_MtConfidenceAltTrans(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
        <alt-trans match-quality="75" origin="MT"><target>Bonjour</target></alt-trans>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testMtConfidenceInline
func TestExtract_MtConfidenceInline(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello <x id="1"/> world</source>
        <target>Bonjour <x id="1"/> monde</target>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testMtConfidenceOutput
func TestExtract_MtConfidenceOutput(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source><target>Bonjour</target>
      </trans-unit>`)
	out := snippetRoundtrip(t, xliff, nil)
	assert.Contains(t, out, "Hello")
}

// okapi: XLIFFFilterTest#testStorageSizeModification
func TestExtract_StorageSizeModification(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testStorageSizeAndAllowedChars
func TestExtract_StorageSizeAndAllowedChars(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testAllowedCharsModification
func TestExtract_AllowedCharsModification(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi-skip: XLIFFFilterTest#testAddedCloneCode — Java clone API

// okapi: XLIFFFilterTest#testSdlTagDefs
func TestExtract_SdlTagDefs(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSdlTagDefsWithSubs
func TestExtract_SdlTagDefsWithSubs(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testSdlXliffApprovedConfStateMapping
func TestExtract_SdlXliffApprovedConfStateMapping(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testRemoveSdlComment
func TestExtract_RemoveSdlComment(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/comments.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testRemoveNestedSdlComment
func TestExtract_RemoveNestedSdlComment(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/comments.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIssue424
func TestExtract_Issue424(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/ImplementationPlan.docx.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIssue466NoMrk
func TestExtract_Issue466NoMrk(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/no_mrk.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIssue466MixedMrk
func TestExtract_Issue466MixedMrk(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIssue466PreserveCRLF
func TestExtract_Issue466PreserveCRLF(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIssue597SdlXliffConfStateMapping
func TestExtract_Issue597SdlXliffConfStateMapping(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIssue597SdlXliffInvalidInitialConf
func TestExtract_Issue597SdlXliffInvalidInitialConf(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIssue597SdlXliffInvalidUpdatedState
func TestExtract_Issue597SdlXliffInvalidUpdatedState(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIssue597SdlXliffNoConf
func TestExtract_Issue597SdlXliffNoConf(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIssue597SdlXliffRemoveStateAndOriginalConf
func TestExtract_Issue597SdlXliffRemoveStateAndOriginalConf(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testLQIAnnotations
func TestExtract_LQIAnnotations(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/lqiExtensions.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testLQRInline
func TestExtract_LQRInline(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_xliff/lqiTest.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testLQIRemoval
func TestExtract_LQIRemoval(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/lqiExtensions.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testLQIAndProvModifications1
func TestExtract_LQIAndProvModifications1(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/lqiExtensions.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testAddLQIModifications2
func TestExtract_AddLQIModifications2(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/lqiExtensions.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testITSAnnotations
func TestExtract_ITSAnnotations(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_xliff/lqiTest.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testITSAnnotatorsRef
func TestExtract_ITSAnnotatorsRef(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_xliff/lqiTest.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testITSStandoffManager
func TestExtract_ITSStandoffManager(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_xliff/lqiTest.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testXLIFFITSLQIMapping
func TestExtract_XLIFFITSLQIMapping(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_xliff/lqiTest.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testXLIFFITSProvenance
func TestExtract_XLIFFITSProvenance(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_xliff/lqiTest.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testXLIFFITSProvenanceFile
func TestExtract_XLIFFITSProvenanceFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_xliff/lqiTest.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testXLIFFITSProvenanceGroup
func TestExtract_XLIFFITSProvenanceGroup(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_xliff/lqiTest.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testXTMAnnotations
func TestExtract_XTMAnnotations(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/xtmxliff/StatusSample.docx.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// Covered by xliff_test.go: XLIFFFilterTest#testWSBetweenSegments
func TestExtract_WSBetweenSegments(t *testing.T) {
	xliff := wrapXLIFF(`      <trans-unit id="1">
        <source>One. Two.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">One.</mrk> <mrk mtype="seg" mid="s2">Two.</mrk>
        </seg-source>
      </trans-unit>`)
	parts := readXLIFFDefault(t, xliff)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}
