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
	// SubTypeBreakStandalone tags a <w:br/> that began a fresh source
	// <w:r> (no preceding text/tab/break in the same <w:r>). The writer
	// must close any open run BEFORE emitting this break so the source
	// run boundary survives the round-trip. Mirrors upstream Okapi
	// RunBuilder (okapi/filters/openxml/RunBuilder.java:73-188) which
	// keeps each <w:br/> Markup chunk anchored to its own source
	// RunBuilder; RunMerger does not collapse break-bearing runs across
	// source <w:r> boundaries (RunMerger.java:156-229). Per ECMA-376-1
	// §17.3.3.1, <w:br/> is a run child whose containing <w:r> defines
	// its rPr context — moving it into the previous text's <w:r>
	// rewrites that envelope (1421-line-break.docx).
	SubTypeBreakStandalone = "openxml:br:standalone"
	// SubTypeTabStandalone is the analogue for <w:tab/>: a tab that
	// began a fresh source <w:r>. Defined for symmetry with
	// SubTypeBreakStandalone but currently never set — upstream
	// Okapi RunMerger fuses adjacent same-rPr runs across <w:tab/>
	// boundaries (Document-with-tabs.docx: `<r>Before</r>
	// <r><tab/>after</r>` merges to `<r><t>Before</t><tab/><t>after
	// </t></r>`), so the writer's inline-into-run path already
	// matches the reference output without a boundary marker.
	// ECMA-376-1 §17.3.3.31. Reserved for future use should a fixture
	// emerge that needs the same boundary semantics for tabs.
	SubTypeTabStandalone = "openxml:tab:standalone"
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
