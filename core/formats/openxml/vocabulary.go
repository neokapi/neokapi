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
	// TypeRawRunMarkup is a verbatim run-child markup chunk that
	// neither carries text nor maps to a structured neokapi span. The
	// reader uses this for elements that ECMA-376 defines as empty
	// run children (CT_Empty descendants) but that don't fit the
	// existing <w:tab/>, <w:br/>, drawing/pict/object envelopes:
	// <w:noBreakHyphen/> (§17.3.3.18) and <w:softHyphen/> (§17.3.3.30).
	// Upstream Okapi RunParser (RunParser.java lines 752-766) routes
	// these to runBuilder.addToMarkup, preserving the element verbatim
	// in the run's body; we mirror that by stashing the literal XML
	// in the Ph's Data field. The writer wraps Data in a <w:r> with
	// the source rPr context, just like <w:tab/>.
	TypeRawRunMarkup = "struct:raw-run-markup"
	// TypeRevisionIns tags a paired-code wrapper for a strict-OOXML
	// `<w:ins>` (revision insertion) or `<w:moveTo>` (revision move-in)
	// element. Per ECMA-376 Part 1 / ISO/IEC 29500-1 §17.13.5.16
	// (CT_RunTrackChange) these wrap inserted runs in tracked-revision
	// content. Upstream Okapi's
	// SkippableElement.RevisionInline.RUN_INSERTED_CONTENT and
	// MOVED_CONTENT_TO bind to the transitional WordprocessingML QName
	// (Namespaces.WordProcessingML.getQName("ins") /
	// .getQName("moveTo") — see Namespaces.java:26 and
	// SkippableElement.java:209-212), so the unwrap-and-keep-children
	// behaviour fires only for the transitional namespace. Strict-OOXML
	// `<w:ins>` (xmlns="http://purl.oclc.org/ooxml/wordprocessingml/main")
	// does NOT match that QName and is preserved verbatim around its
	// inner runs. We model the strict-mode preservation as paired
	// codes — the writer re-emits the captured `<w:ins ...>` start
	// tag and matching `</w:ins>` end tag around the inner content.
	// 859.docx (Strict OOXML) is the canonical fixture: a
	// `<w:ins w:id="0" w:author="User" w:date="…">` wrapping a
	// translatable run survives the round-trip intact.
	TypeRevisionIns = "struct:revision-ins"
	// TypeSDT tags a paired-code wrapper for an inline `<w:sdt>`
	// (Structured Document Tag) element. Per ECMA-376 Part 1 / ISO/IEC
	// 29500-1 §17.5.2 the SDT envelope wraps `<w:sdtPr>`, optional
	// `<w:sdtEndPr>`, and `<w:sdtContent>` around the placeholder
	// content. Upstream Okapi RunContainer (RunContainer.java:97-176)
	// preserves the outer markup as paired startMarkup / endMarkup
	// events around the extracted inner content. We model the
	// preservation as paired codes — the OPEN payload carries
	// `<w:sdt><w:sdtPr>...</w:sdtPr><w:sdtEndPr/><w:sdtContent>` and
	// the CLOSE payload carries `</w:sdtContent></w:sdt>`. 1085.docx
	// is the canonical fixture: an empty-sdtContent SDT must round-
	// trip with all wrapper metadata intact.
	TypeSDT = "struct:sdt"
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
	// SubTypeNoBreakHyphen / SubTypeSoftHyphen identify the two
	// CT_Empty hyphen run-children covered by TypeRawRunMarkup
	// (ECMA-376-1 §17.3.3.18 and §17.3.3.30 respectively). Stored on
	// the Ph so future writers can branch on the specific element type
	// without re-parsing Ph.Data.
	SubTypeNoBreakHyphen = "openxml:noBreakHyphen"
	SubTypeSoftHyphen    = "openxml:softHyphen"
	// SubTypeRevisionIns / SubTypeRevisionMoveTo distinguish the two
	// element variants paired-coded by TypeRevisionIns. ECMA-376-1
	// §17.13.5.16 (<w:ins> CT_RunTrackChange) and §17.13.5.25
	// (<w:moveTo> CT_RunTrackChange) share the same content model.
	SubTypeRevisionIns    = "openxml:ins"
	SubTypeRevisionMoveTo = "openxml:moveTo"
	// SubTypeSDT tags the standard inline `<w:sdt>` paired-code wrapper
	// emitted by parseInlineSDT for SDTs whose source carried a
	// `<w:sdtContent>` element (open or self-closing).
	SubTypeSDT = "openxml:sdt"
	// SubTypeSDTNoContent tags an inline `<w:sdt>` whose source had
	// no `<w:sdtContent>` child at all. Per ECMA-376-1 §17.5.2 this is
	// schema-questionable but observed in the wild; the close payload
	// is a synthesised `</w:sdt>` rather than `</w:sdtContent></w:sdt>`.
	SubTypeSDTNoContent = "openxml:sdt-no-content"
)
