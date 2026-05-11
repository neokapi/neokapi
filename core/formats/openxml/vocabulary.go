package openxml

// Semantic type constants for OpenXML inline formatting.
// These map OpenXML run properties to the vocabulary system.
const (
	TypeBold          = "fmt:bold"
	TypeItalic        = "fmt:italic"
	TypeUnderline     = "fmt:underline"
	TypeStrikethrough = "fmt:strikethrough"
	TypeSuperscript   = "fmt:superscript"
	TypeSubscript     = "fmt:subscript"
	TypeHyperlink     = "link:hyperlink"
	TypeSmartTag      = "struct:smarttag"
	TypeBreak         = "struct:break"
	TypeTab           = "struct:tab"
	TypeImage         = "media:image"
	TypeFootnoteRef   = "struct:footnote"
	TypeBookmark      = "struct:bookmark"
	// TypeCommentRange tags a <w:commentRangeStart/> or
	// <w:commentRangeEnd/> marker that delimits the run range a
	// comment annotates. ECMA-376 Part 1 §17.13.4.3 / §17.13.4.4
	// (CT_MarkupRangeStart / CT_MarkupRange). Upstream Okapi
	// wordConfiguration.ymlbal lines 59-63 classify both as INLINE
	// markup, so they survive the round-trip as inline placeholders
	// rather than translatable text.
	TypeCommentRange = "struct:commentRange"
	// TypeField is an opaque field-markup chunk: a <w:r> wrapping a
	// <w:fldChar> (begin/separate/end), a <w:r> wrapping <w:instrText>,
	// or a <w:fldSimple>...</w:fldSimple>. Per upstream Okapi
	// (RunParser.parseComplexField, lines 461-542 of
	// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/
	// RunParser.java; BlockParser.parse for fldSimple, lines 242-250 of
	// BlockParser.java) the entire complex-field structure and every
	// fldSimple is preserved verbatim as markup chunks on the block —
	// only the cached display text inside an *extractable* field code
	// (HYPERLINK et al., per ConditionalParameters.tsComplexField
	// DefinitionsToExtract) is parsed for translation. ECMA-376 Part 1
	// §17.16.5 (fldChar), §17.16.18 (instrText), §17.16.6 (fldSimple).
	TypeField = "struct:field"
)

// SubType constants provide format-specific refinement.
const (
	SubTypeBold          = "openxml:b"
	SubTypeItalic        = "openxml:i"
	SubTypeUnderline     = "openxml:u"
	SubTypeStrikethrough = "openxml:strike"
	SubTypeSuperscript   = "openxml:superscript"
	SubTypeSubscript     = "openxml:subscript"
	SubTypeHyperlink     = "openxml:hyperlink"
	SubTypeSmartTag      = "openxml:smartTag"
	SubTypeBreak         = "openxml:br"
	SubTypeTab           = "openxml:tab"
	SubTypeImage         = "openxml:drawing"
	SubTypeFootnoteRef   = "openxml:footnoteRef"
	SubTypeBookmarkStart     = "openxml:bookmarkStart"
	SubTypeBookmarkEnd       = "openxml:bookmarkEnd"
	SubTypeCommentRangeStart = "openxml:commentRangeStart"
	SubTypeCommentRangeEnd   = "openxml:commentRangeEnd"
	// SubTypeFieldChar tags a captured <w:r> that wraps a complex-field
	// fldChar marker (begin/separate/end) or an instrText element.
	// SubTypeFieldSimple tags a captured <w:fldSimple> element.
	SubTypeFieldChar   = "openxml:fldChar"
	SubTypeFieldSimple = "openxml:fldSimple"
)
