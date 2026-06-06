package its

// NamespaceURI is the W3C ITS 2.0 namespace. Authors typically bind
// the `its:` prefix to it, but the prefix is arbitrary — only the
// namespace URI is normative.
const NamespaceURI = "http://www.w3.org/2005/11/its"

// XMLNamespaceURI is the reserved XML namespace used by `xml:lang`,
// `xml:space`, and similar attributes. ITS rule selectors that target
// xml:lang treat this as the namespace.
const XMLNamespaceURI = "http://www.w3.org/XML/1998/namespace"

// XLinkNamespaceURI is the namespace for xlink:href used by the
// External Resource data category and for linking external rule
// documents from <its:rules xlink:href="..."/>.
const XLinkNamespaceURI = "http://www.w3.org/1999/xlink"

// DataCategory enumerates the ITS 2.0 data categories. Only the
// categories that drive extraction or content-model decisions are
// listed today; the rest can be added incrementally as readers learn
// to act on them.
type DataCategory int

const (
	// CatTranslate is the Translate data category (§6.2). Drives
	// whether content is extracted as translatable.
	CatTranslate DataCategory = iota + 1

	// CatLocalizationNote is the Localization Note data category
	// (§6.4). Attaches translator-facing notes to extracted content.
	CatLocalizationNote

	// CatTerminology is the Terminology data category (§6.5). Marks
	// inline spans as terms (with optional confidence and definition
	// pointer).
	CatTerminology

	// CatElementsWithinText is the Elements Within Text data
	// category (§6.8). Drives whether an element is treated as inline
	// (no extraction boundary) or as block (extraction boundary).
	CatElementsWithinText

	// CatDomain is the Domain data category (§6.10). Tags content
	// with subject-domain metadata for downstream MT/glossary
	// selection.
	CatDomain

	// CatPreserveSpace is the Preserve Space data category (§6.16).
	// Drives whitespace handling — equivalent to xml:space="preserve"
	// when "preserve".
	CatPreserveSpace

	// CatExternalResource is the External Resource data category
	// (§6.11). Marks attribute values as URIs to non-translatable
	// resources (e.g. <img src=…>).
	CatExternalResource

	// CatLocaleFilter is the Locale Filter data category (§6.12).
	// Restricts which locales process the content.
	CatLocaleFilter

	// CatIDValue is the Id Value data category (§6.16 in 2.0). Picks
	// an attribute as the stable identifier for the element.
	CatIDValue
)

// LocNoteType distinguishes informational notes from constraints.
// "alert" notes are warnings the translator MUST act on; "description"
// notes are informational. ITS 2.0 §6.4.1.
type LocNoteType string

const (
	LocNoteDescription LocNoteType = "description"
	LocNoteAlert       LocNoteType = "alert"
)

// Tristate represents the three-valued logic ITS uses for boolean
// data categories so the absence of a rule can be distinguished from
// an explicit yes/no.
type Tristate int8

const (
	Unset Tristate = iota
	Yes
	No
)

func (t Tristate) String() string {
	switch t {
	case Yes:
		return "yes"
	case No:
		return "no"
	default:
		return ""
	}
}

// ParseTristate converts an ITS attribute value ("yes" / "no") to a
// Tristate. Unrecognised values return Unset so callers can treat
// authoring errors as "no rule applied" rather than crashing.
func ParseTristate(v string) Tristate {
	switch v {
	case "yes":
		return Yes
	case "no":
		return No
	default:
		return Unset
	}
}

// Rule captures one ITS global rule declaration (one
// <its:translateRule>, <its:withinTextRule>, …) parsed out of an
// <its:rules> container. The fields covered depend on the Category;
// fields not relevant to a given category stay at their zero values.
type Rule struct {
	// Category is the data category this rule contributes to.
	Category DataCategory

	// Selector is the XPath selector picking which nodes the rule
	// applies to. nil for rules that target the whole document
	// (rare).
	Selector *Selector

	// SelectorRaw is the original selector string before parsing,
	// kept for diagnostics and for parity with okapi's error
	// messages.
	SelectorRaw string

	// Priority resolves rule conflicts. Higher wins. Default 0; ITS
	// rules are not formally prioritized, but okapi orders by
	// document position so later rules override earlier ones —
	// expressed here by descending priority (1 = first, N = last).
	Priority int

	// --- per-category payload below; only the relevant subset is
	// populated for a given Category ---

	// Translate value for translateRule (Yes / No).
	Translate Tristate

	// WithinText value for withinTextRule. ITS 2.0 §6.8 allows
	// "yes" / "no" / "nested"; we model "nested" as Yes for now
	// (callers that care about nested vs simple inline can read the
	// raw string from WithinTextRaw).
	WithinText    Tristate
	WithinTextRaw string

	// LocNote payload — set when LocNotePointer / LocNoteRefPointer
	// dereferences another node, otherwise the literal LocNoteText
	// is used.
	LocNoteType       LocNoteType
	LocNoteText       string
	LocNotePointer    string // XPath expression resolving to note text
	LocNoteRef        string
	LocNoteRefPointer string

	// Term flag for termRule. true marks the matched element as a
	// term occurrence.
	Term           Tristate
	TermInfoRef    string
	TermInfoRefPtr string
	TermConfidence string
	TermInfo       string

	// Domain payload for domainRule.
	DomainPointer string
	DomainMapping string

	// PreserveSpace value for preserveSpaceRule.
	PreserveSpace Tristate

	// ExternalResource pointer string for externalResourceRefRule
	// (selects the attribute holding the URI).
	ExternalResourceRefPointer string

	// LocaleFilter list / type for localeFilterRule.
	LocaleFilterList string
	LocaleFilterType string // "include" | "exclude"

	// IDValuePointer for idValueRule.
	IDValuePointer string
}

// RuleSet is the ordered collection of rules extracted from one or
// more <its:rules> elements. Rules later in the slice win when they
// match the same node; this matches ITS 2.0 §5.4 "Last rule wins"
// semantics within a single rule set, and within combined rule sets
// (linked + embedded) okapi processes linked rules first then
// embedded ones, so embedded rules end up later and therefore win.
type RuleSet struct {
	Rules []Rule
}

// IsEmpty reports whether the rule set has any rules.
func (rs *RuleSet) IsEmpty() bool {
	return rs == nil || len(rs.Rules) == 0
}

// Append adds rules from `other` to the end of this set. Used to
// combine rules from linked rule documents with embedded rules.
func (rs *RuleSet) Append(other *RuleSet) {
	if other == nil {
		return
	}
	rs.Rules = append(rs.Rules, other.Rules...)
}
