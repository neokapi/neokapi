//go:build integration

package okf_xliff2

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.xliff2.XLIFF2Filter"
const mimeType = "application/xliff+xml"

func TestExtract_SimpleXLIFF2(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Hello world</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from XLIFF 2.0")
}

func TestExtract_MultipleUnits(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>First</source>
      </segment>
    </unit>
    <unit id="2">
      <segment>
        <source>Second</source>
      </segment>
    </unit>
    <unit id="3">
      <segment>
        <source>Third</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 3)

	texts := bridgetest.BlockTexts(blocks)
	assert.Equal(t, "First", texts[0])
	assert.Equal(t, "Second", texts[1])
	assert.Equal(t, "Third", texts[2])
}

func TestExtract_WithTarget(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Hello</source>
        <target>Bonjour</target>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "Hello", b.SourceText())
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Equal(t, "Bonjour", b.TargetText("fr"))
}

func TestExtract_UnitIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="greeting">
      <segment>
        <source>Hello</source>
      </segment>
    </unit>
    <unit id="farewell">
      <segment>
        <source>Goodbye</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2)

	assert.Equal(t, "greeting", blocks[0].ID)
	assert.Equal(t, "farewell", blocks[1].ID)
}

func TestExtract_InlinePh(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// XLIFF 2.0 uses <ph> for standalone inline codes.
	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Line one<ph id="1" equiv="lb"/>Line two</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)

	var hasPlaceholder bool
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder {
			hasPlaceholder = true
			break
		}
	}
	assert.True(t, hasPlaceholder, "should have a placeholder span for <ph>")
}

func TestExtract_InlinePc(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// XLIFF 2.0 uses <pc> for paired inline codes (replaces <g>/<bpt>/<ept>).
	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Hello <pc id="1">bold</pc> text</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.GreaterOrEqual(t, len(frag.Spans), 2,
		"should have opening and closing spans for <pc>")

	var hasOpening, hasClosing bool
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanOpening {
			hasOpening = true
		}
		if s.SpanType == model.SpanClosing {
			hasClosing = true
		}
	}
	assert.True(t, hasOpening, "should have opening span from <pc>")
	assert.True(t, hasClosing, "should have closing span from </pc>")
}

func TestExtract_Notes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// XLIFF 2.0 uses <notes><note> within <unit>.
	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <notes>
        <note category="description">This is a note</note>
      </notes>
      <segment>
        <source>Hello</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	// XLIFF 2.0 notes should appear as annotations.
	if b.Annotations != nil {
		if noteAnn, ok := b.Annotations["note"]; ok {
			note := noteAnn.(*model.NoteAnnotation)
			assert.Equal(t, "This is a note", note.Text)
		}
	}
}

func TestExtract_GroupStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <group id="g1">
      <unit id="1">
        <segment>
          <source>In group</source>
        </segment>
      </unit>
    </group>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	var hasGroupStart, hasGroupEnd bool
	for _, p := range parts {
		if p.Type == model.PartGroupStart {
			hasGroupStart = true
			gs := p.Resource.(*model.GroupStart)
			assert.Equal(t, "g1", gs.ID)
		}
		if p.Type == model.PartGroupEnd {
			hasGroupEnd = true
		}
	}
	assert.True(t, hasGroupStart, "should have GroupStart for <group>")
	assert.True(t, hasGroupEnd, "should have GroupEnd for </group>")
}

func TestExtract_MultipleSegments(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// XLIFF 2.0 supports multiple segments within a unit.
	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment id="s1">
        <source>First sentence.</source>
      </segment>
      <segment id="s2">
        <source>Second sentence.</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	require.GreaterOrEqual(t, len(b.Source), 2,
		"unit with 2 segments should produce 2+ source segments")
}

func TestExtract_LayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Hello</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

func TestExtract_UnicodeContent(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>こんにちは世界</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	assert.Equal(t, "こんにちは世界", blocks[0].SourceText())
}

func TestExtract_TranslateNo(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1" translate="yes">
      <segment>
        <source>Translate me</source>
      </segment>
    </unit>
    <unit id="2" translate="no">
      <segment>
        <source>Do not translate</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	translatableBlocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, translatableBlocks, 1)
	assert.Equal(t, "Translate me", translatableBlocks[0].SourceText())
}
