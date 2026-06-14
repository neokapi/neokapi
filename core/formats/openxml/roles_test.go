package openxml

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildStyleRoleMap exercises styleId → semantic-role resolution from a
// synthetic styles.xml covering the built-in convention, localized names,
// explicit outlineLvl, basedOn inheritance, and the cases that must NOT map.
func TestBuildStyleRoleMap(t *testing.T) {
	stylesXML := []byte(`<?xml version="1.0"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:default="1" w:styleId="Normal">
    <w:name w:val="Normal"/>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Heading1">
    <w:name w:val="heading 1"/>
    <w:basedOn w:val="Normal"/>
    <w:pPr><w:outlineLvl w:val="0"/></w:pPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="MyHeading2">
    <w:name w:val="heading 2"/>
    <w:basedOn w:val="Normal"/>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Subheader">
    <w:name w:val="Subheader"/>
    <w:basedOn w:val="Heading1"/>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Quote">
    <w:name w:val="Quote"/>
    <w:basedOn w:val="Normal"/>
    <w:pPr><w:outlineLvl w:val="2"/></w:pPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Title">
    <w:name w:val="Title"/>
  </w:style>
  <w:style w:type="character" w:styleId="Heading3">
    <w:name w:val="heading 3"/>
  </w:style>
</w:styles>`)

	m := buildStyleRoleMap(stylesXML)
	require.NotNil(t, m)

	tests := []struct {
		styleID   string
		wantRole  string
		wantLevel int
	}{
		{"Heading1", model.RoleHeading, 1},   // built-in styleId convention
		{"MyHeading2", model.RoleHeading, 2}, // localized name "heading 2"
		{"Subheader", model.RoleHeading, 1},  // inherits Heading1 via basedOn
		{"Quote", model.RoleHeading, 3},      // explicit outlineLvl=2 → level 3
		{"Title", model.RoleTitle, 0},        // title
		{"Normal", "", 0},                    // no role
		{"Heading3", "", 0},                  // character style — not a block role
		{"Nonexistent", "", 0},               // absent → built-in heuristic (none)
	}
	for _, tt := range tests {
		t.Run(tt.styleID, func(t *testing.T) {
			r := m[tt.styleID]
			assert.Equal(t, tt.wantRole, r.role, "role for %s", tt.styleID)
			assert.Equal(t, tt.wantLevel, r.level, "level for %s", tt.styleID)
		})
	}
}

// TestBuildStyleRoleMapEmpty returns nil for empty/role-less input.
func TestBuildStyleRoleMapEmpty(t *testing.T) {
	assert.Nil(t, buildStyleRoleMap(nil))
	assert.Nil(t, buildStyleRoleMap([]byte("")))
	// Well-formed styles.xml with no heading/title styles → nil map.
	noRoles := []byte(`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
  <w:style w:type="paragraph" w:styleId="BodyText"><w:name w:val="Body Text"/></w:style>
</w:styles>`)
	assert.Nil(t, buildStyleRoleMap(noRoles))
}

// TestRoleForParaStyleFallback confirms the built-in styleId heuristic applies
// even when no styles.xml role map was loaded (nil map).
func TestRoleForParaStyleFallback(t *testing.T) {
	tests := []struct {
		styleID   string
		wantRole  string
		wantLevel int
	}{
		{"Heading1", model.RoleHeading, 1},
		{"Heading9", model.RoleHeading, 9},
		{"heading2", model.RoleHeading, 2}, // case-insensitive
		{"Title", model.RoleTitle, 0},
		{"Normal", "", 0},
		{"BodyText", "", 0},
		{"", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.styleID, func(t *testing.T) {
			r := roleForParaStyle(tt.styleID, nil)
			assert.Equal(t, tt.wantRole, r.role)
			assert.Equal(t, tt.wantLevel, r.level)
		})
	}
}

// TestRoleForParaStyleMapWins confirms a resolved map entry takes precedence
// over the built-in heuristic (custom heading styleId only the map knows).
func TestRoleForParaStyleMapWins(t *testing.T) {
	m := styleRoleMap{"CustomH": {role: model.RoleHeading, level: 4}}
	r := roleForParaStyle("CustomH", m)
	assert.Equal(t, model.RoleHeading, r.role)
	assert.Equal(t, 4, r.level)
}

func TestParaHasNumbering(t *testing.T) {
	assert.True(t, paraHasNumbering(`<w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr>`))
	assert.False(t, paraHasNumbering(`<w:pPr><w:pStyle w:val="Normal"/></w:pPr>`))
	assert.False(t, paraHasNumbering(""))
}

// TestNative_DocxHeadingRole reads a real .docx whose paragraph carries the
// built-in Heading1 style (and which ships NO styles.xml, exercising the
// fallback heuristic) and asserts the block records the heading role + level.
// This is the load-bearing WS2 signal that drives DOCX → clean Markdown export.
func TestNative_DocxHeadingRole(t *testing.T) {
	parts := readFile(t, "testdata/formatted.docx")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	var heading *model.Block
	for _, b := range blocks {
		if b.SourceText() == "A Heading" {
			heading = b
			break
		}
	}
	require.NotNil(t, heading, "expected an 'A Heading' block")

	assert.Equal(t, model.RoleHeading, heading.SemanticRole(), "Heading1 paragraph should carry the heading role")
	s, ok := heading.Structure()
	require.True(t, ok)
	assert.Equal(t, 1, s.Level, "Heading1 → level 1")

	// A non-heading paragraph must keep its role unset (falls back to block.Type).
	for _, b := range blocks {
		if b.SourceText() == "Simple paragraph" {
			assert.Empty(t, b.SemanticRole(), "plain paragraph should have no semantic role")
		}
	}
}
