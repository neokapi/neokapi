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
	TypeBreak         = "struct:break"
	TypeTab           = "struct:tab"
	TypeImage         = "media:image"
	TypeFootnoteRef   = "struct:footnote"
	TypeBookmark      = "struct:bookmark"
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
	SubTypeBreak         = "openxml:br"
	SubTypeTab           = "openxml:tab"
	SubTypeImage         = "openxml:drawing"
	SubTypeFootnoteRef   = "openxml:footnoteRef"
	SubTypeBookmarkStart = "openxml:bookmarkStart"
	SubTypeBookmarkEnd   = "openxml:bookmarkEnd"
)
