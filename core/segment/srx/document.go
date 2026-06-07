// Package srx implements a faithful SRX 2.0 sentence-segmentation engine,
// modelled on Okapi's net.sf.okapi.lib.segmentation.SRXSegmenter. It parses an
// SRX 2.0 ruleset, selects language rules for a locale via the rule map
// (honouring header cascade), and applies the ordered before-break/after-break
// rules over the masked text supplied by core/segment. The engine returns the
// rune offsets at which new segments begin, which segment.Flattened.Spans
// projects to run-anchored stand-off overlays (AD-002).
//
// SRX rule regexes use lookbehind/lookahead beyond Go's RE2 engine, so this
// package matches with github.com/dlclark/regexp2. regexp2 reports match
// Index/Length as rune offsets (verified against astral and combining input),
// which is exactly what segment.Flattened.Spans expects — no UTF-16 or byte
// conversion is needed.
package srx

import (
	"encoding/xml"
	"fmt"
	"os"
)

// Document is a parsed SRX 2.0 ruleset.
type Document struct {
	Version string

	// SegmentSubflows mirrors header @segmentsubflows. Subflows are not a
	// concept in the run/overlay model (inline codes are atomic), so the field
	// is parsed for fidelity but does not affect segmentation here.
	SegmentSubflows bool
	// Cascade mirrors header @cascade. When true, the rules of every matching
	// language map are applied in map order; when false, only the first
	// matching map's rules apply.
	Cascade bool
	// FormatHandles mirror the header <formathandle> entries. Inline-code
	// handling is governed by segment.MaskOptions in this implementation, so
	// these are parsed for fidelity but not consulted by the algorithm.
	FormatHandles []FormatHandle
	// UseICUBreakRules mirrors Okapi's header extension
	// <okpsrx:options useIcu4jBreakRules="yes"/>. When true, a UAX-29 base
	// breaker supplies the candidate sentence breaks and the SRX rules are
	// applied on top as exceptions (Okapi parity). When the ruleset relies on
	// ICU for breaking — as Okapi's defaultSegmentation.srx does — pure-rule
	// mode under-segments, so the engine only takes the hybrid path when a base
	// breaker is available (cgo/ICU builds) and otherwise falls back.
	UseICUBreakRules bool

	// TrimLeadingWS / TrimTrailingWS mirror Okapi's header extension
	// <okpsrx:options trimLeadingWhitespaces="yes" trimTrailingWhitespaces="yes"/>.
	// When set, each segment span excludes its leading / trailing whitespace,
	// leaving inter-sentence whitespace as uncovered (ignorable) material — so a
	// segment is the clean sentence regardless of which side of the space the
	// break fell on. Okapi's defaultSegmentation.srx sets both to yes.
	TrimLeadingWS  bool
	TrimTrailingWS bool

	// LanguageRules are the named rule groups from <languagerules>.
	LanguageRules []LanguageRule
	// LanguageMaps map a locale pattern to a named language rule, in order.
	LanguageMaps []LanguageMap
}

// FormatHandle is a header <formathandle> entry.
type FormatHandle struct {
	Type    string // start | end | isolated
	Include bool
}

// LanguageRule is a named group of ordered break/no-break rules.
type LanguageRule struct {
	Name  string
	Rules []Rule
}

// Rule is one <rule>: a break decision plus the before/after regex context.
// The candidate boundary sits between the before-break match and the
// after-break match.
type Rule struct {
	Break       bool
	BeforeBreak string
	AfterBreak  string
}

// LanguageMap maps a locale pattern (a regex over the BCP-47 tag) to a named
// language rule.
type LanguageMap struct {
	LanguagePattern string
	LanguageRule    string
}

// --- XML binding types ---

type xmlSRX struct {
	XMLName xml.Name  `xml:"srx"`
	Version string    `xml:"version,attr"`
	Header  xmlHeader `xml:"header"`
	Body    xmlBody   `xml:"body"`
}

type xmlHeader struct {
	SegmentSubflows string            `xml:"segmentsubflows,attr"`
	Cascade         string            `xml:"cascade,attr"`
	FormatHandle    []xmlFormatHandle `xml:"formathandle"`
	// Options is Okapi's namespaced header extension
	// (<okpsrx:options .../>). Go's xml matches by local name regardless of the
	// okpsrx: prefix, so this binds without declaring the namespace.
	Options xmlOptions `xml:"options"`
}

type xmlOptions struct {
	UseIcu4jBreakRules     string `xml:"useIcu4jBreakRules,attr"`
	TrimLeadingWhitespaces string `xml:"trimLeadingWhitespaces,attr"`
	TrimTrailingWhitespace string `xml:"trimTrailingWhitespaces,attr"`
}

type xmlFormatHandle struct {
	Type    string `xml:"type,attr"`
	Include string `xml:"include,attr"`
}

type xmlBody struct {
	LanguageRules xmlLanguageRules `xml:"languagerules"`
	MapRules      xmlMapRules      `xml:"maprules"`
}

type xmlLanguageRules struct {
	LanguageRule []xmlLanguageRule `xml:"languagerule"`
}

type xmlLanguageRule struct {
	Name string    `xml:"languagerulename,attr"`
	Rule []xmlRule `xml:"rule"`
}

type xmlRule struct {
	Break       string `xml:"break,attr"`
	BeforeBreak string `xml:"beforebreak"`
	AfterBreak  string `xml:"afterbreak"`
}

type xmlMapRules struct {
	LanguageMap []xmlLanguageMap `xml:"languagemap"`
}

type xmlLanguageMap struct {
	LanguagePattern string `xml:"languagepattern,attr"`
	LanguageRule    string `xml:"languagerulename,attr"`
}

// Parse decodes an SRX 2.0 document. It is liberal: a missing before/after
// break is treated as empty (match-anything), an unspecified break attribute
// defaults to a break ("yes" is the SRX default), and an unspecified cascade
// defaults to true (the SRX 2.0 default).
func Parse(data []byte) (*Document, error) {
	var x xmlSRX
	if err := xml.Unmarshal(data, &x); err != nil {
		return nil, fmt.Errorf("srx: parse: %w", err)
	}

	doc := &Document{
		Version:          x.Version,
		SegmentSubflows:  parseYesNo(x.Header.SegmentSubflows, true),
		Cascade:          parseYesNo(x.Header.Cascade, true),
		UseICUBreakRules: parseYesNo(x.Header.Options.UseIcu4jBreakRules, false),
		TrimLeadingWS:    parseYesNo(x.Header.Options.TrimLeadingWhitespaces, false),
		TrimTrailingWS:   parseYesNo(x.Header.Options.TrimTrailingWhitespace, false),
	}

	for _, fh := range x.Header.FormatHandle {
		doc.FormatHandles = append(doc.FormatHandles, FormatHandle{
			Type:    fh.Type,
			Include: parseYesNo(fh.Include, false),
		})
	}

	for _, lr := range x.Body.LanguageRules.LanguageRule {
		rule := LanguageRule{Name: lr.Name}
		for _, r := range lr.Rule {
			rule.Rules = append(rule.Rules, Rule{
				Break:       parseYesNo(r.Break, true),
				BeforeBreak: r.BeforeBreak,
				AfterBreak:  r.AfterBreak,
			})
		}
		doc.LanguageRules = append(doc.LanguageRules, rule)
	}

	for _, lm := range x.Body.MapRules.LanguageMap {
		doc.LanguageMaps = append(doc.LanguageMaps, LanguageMap(lm))
	}

	return doc, nil
}

// ParseFile reads and parses an SRX 2.0 document from path.
func ParseFile(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("srx: read %s: %w", path, err)
	}
	return Parse(data)
}

// languageRule returns the named language rule, or nil if absent.
func (d *Document) languageRule(name string) *LanguageRule {
	for i := range d.LanguageRules {
		if d.LanguageRules[i].Name == name {
			return &d.LanguageRules[i]
		}
	}
	return nil
}

func parseYesNo(v string, def bool) bool {
	switch v {
	case "yes", "Yes", "YES", "true", "True", "1":
		return true
	case "no", "No", "NO", "false", "False", "0":
		return false
	default:
		return def
	}
}
