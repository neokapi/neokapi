package its

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// ExtractRules walks the XML byte stream and returns every rule
// declared in any <its:rules> element it encounters. Rules from
// later <its:rules> blocks (in document order) take precedence —
// callers that combine these with externally-loaded rules per ITS
// 2.0 §5.4 should prepend the external rules so embedded rules win.
//
// The function never errors on unrecognised rule elements; it simply
// skips them. Authoring errors inside *recognised* rule elements
// (bad selectors, invalid Tristate values) are surfaced as errors
// because they would otherwise silently mis-extract content.
//
// External rule documents (`<its:rules xlink:href="..."/>`) are NOT
// followed here — pass the URI to ExtractRulesFromReader recursively
// after resolving it against the document base URI.
func ExtractRules(content []byte) (*RuleSet, []ExternalRef, error) {
	dec := xml.NewDecoder(strings.NewReader(string(content)))
	return parseRulesStream(dec)
}

// ExternalRef describes one <its:rules xlink:href="..."> reference
// the caller should resolve and process.
type ExternalRef struct {
	Href string
}

func parseRulesStream(dec *xml.Decoder) (*RuleSet, []ExternalRef, error) {
	rs := &RuleSet{}
	var externals []ExternalRef
	priority := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("its: parsing document: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Space != NamespaceURI || se.Name.Local != "rules" {
			continue
		}
		if href := attrValue(se.Attr, XLinkNamespaceURI, "href"); href != "" {
			externals = append(externals, ExternalRef{Href: href})
		}
		nsMap := buildNamespaceMap(se.Attr)
		err = parseRulesElement(dec, se, nsMap, rs, &priority)
		if err != nil {
			return nil, nil, err
		}
	}
	return rs, externals, nil
}

// parseRulesElement is invoked once we've consumed an <its:rules>
// start tag. It walks until the matching end tag, parsing each
// recognised rule child.
func parseRulesElement(dec *xml.Decoder, parent xml.StartElement, parentNS map[string]string, rs *RuleSet, priority *int) error {
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return fmt.Errorf("its: unexpected EOF inside <its:rules>")
		}
		if err != nil {
			return fmt.Errorf("its: parsing <its:rules>: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name == parent.Name {
				return nil
			}
		case xml.StartElement:
			// Layer per-element namespace declarations on top of
			// the inherited map so child rules can declare new
			// prefixes.
			ns := mergeNamespaceMaps(parentNS, buildNamespaceMap(t.Attr))
			if t.Name.Space == NamespaceURI {
				rule, err := parseRuleElement(t, ns, *priority+1)
				if err != nil {
					return err
				}
				if rule != nil {
					*priority++
					rs.Rules = append(rs.Rules, *rule)
				}
			}
			// Skip the element body — none of the rule elements
			// have nested rules in our scope.
			if err := dec.Skip(); err != nil {
				return fmt.Errorf("its: skipping element body: %w", err)
			}
		}
	}
}

// parseRuleElement turns one <its:*Rule> element into a Rule. Returns
// (nil, nil) for unrecognised element names so future categories can
// be added without breaking authoring.
func parseRuleElement(t xml.StartElement, nsMap map[string]string, priority int) (*Rule, error) {
	r := Rule{Priority: priority}
	switch t.Name.Local {
	case "translateRule":
		r.Category = CatTranslate
		r.Translate = ParseTristate(attrValueLocal(t.Attr, "translate"))
	case "withinTextRule":
		r.Category = CatElementsWithinText
		raw := attrValueLocal(t.Attr, "withinText")
		r.WithinTextRaw = raw
		switch raw {
		case "yes", "nested":
			r.WithinText = Yes
		case "no":
			r.WithinText = No
		default:
			r.WithinText = Unset
		}
	case "locNoteRule":
		r.Category = CatLocalizationNote
		r.LocNoteType = LocNoteType(attrValueLocal(t.Attr, "locNoteType"))
		if r.LocNoteType == "" {
			r.LocNoteType = LocNoteDescription
		}
		// locNoteRule may carry the literal note as a child <locNote>
		// element OR via a locNotePointer attribute.
		r.LocNotePointer = attrValueLocal(t.Attr, "locNotePointer")
		r.LocNoteRefPointer = attrValueLocal(t.Attr, "locNoteRefPointer")
		r.LocNoteRef = attrValueLocal(t.Attr, "locNoteRef")
		// Inline literal note text comes in via a child element;
		// caller-side parser pre-collected child element text into
		// LocNoteText if present.  Empty stays empty.
	case "termRule":
		r.Category = CatTerminology
		r.Term = ParseTristate(attrValueLocal(t.Attr, "term"))
		r.TermInfoRef = attrValueLocal(t.Attr, "termInfoRef")
		r.TermInfoRefPtr = attrValueLocal(t.Attr, "termInfoRefPointer")
		r.TermConfidence = attrValueLocal(t.Attr, "termConfidence")
	case "domainRule":
		r.Category = CatDomain
		r.DomainPointer = attrValueLocal(t.Attr, "domainPointer")
		r.DomainMapping = attrValueLocal(t.Attr, "domainMapping")
	case "preserveSpaceRule":
		r.Category = CatPreserveSpace
		r.PreserveSpace = ParseTristate(attrValueLocal(t.Attr, "space"))
	case "externalResourceRefRule":
		r.Category = CatExternalResource
		r.ExternalResourceRefPointer = attrValueLocal(t.Attr, "externalResourceRefPointer")
	case "localeFilterRule":
		r.Category = CatLocaleFilter
		r.LocaleFilterList = attrValueLocal(t.Attr, "localeFilterList")
		r.LocaleFilterType = attrValueLocal(t.Attr, "localeFilterType")
	case "idValueRule":
		r.Category = CatIDValue
		r.IDValuePointer = attrValueLocal(t.Attr, "idValuePointer")
	default:
		return nil, nil
	}
	r.SelectorRaw = attrValueLocal(t.Attr, "selector")
	if r.SelectorRaw != "" {
		sel, err := ParseSelector(r.SelectorRaw, nsMap)
		if err != nil {
			return nil, fmt.Errorf("its: parsing selector for %s: %w", t.Name.Local, err)
		}
		r.Selector = sel
	}
	return &r, nil
}

// buildNamespaceMap reads xmlns / xmlns:prefix attributes from a
// start element's attribute list and returns a prefix→URI map. The
// default xmlns binds to the empty prefix "".
func buildNamespaceMap(attrs []xml.Attr) map[string]string {
	m := map[string]string{}
	for _, a := range attrs {
		if a.Name.Space == "" && a.Name.Local == "xmlns" {
			m[""] = a.Value
		} else if a.Name.Space == "xmlns" {
			m[a.Name.Local] = a.Value
		}
	}
	// Always-resolvable prefixes.
	m["xml"] = XMLNamespaceURI
	m["xlink"] = XLinkNamespaceURI
	if _, ok := m["its"]; !ok {
		m["its"] = NamespaceURI
	}
	return m
}

// mergeNamespaceMaps returns the union of parent + child, with child
// entries overriding on prefix collision.
func mergeNamespaceMaps(parent, child map[string]string) map[string]string {
	out := make(map[string]string, len(parent)+len(child))
	for k, v := range parent {
		out[k] = v
	}
	for k, v := range child {
		out[k] = v
	}
	return out
}

// attrValue returns the value of the attribute matching (ns, local)
// or "" if absent.
func attrValue(attrs []xml.Attr, ns, local string) string {
	for _, a := range attrs {
		if a.Name.Space == ns && a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// attrValueLocal returns the value of the first attribute with the
// given local name (any namespace). Most ITS rule attributes are
// unprefixed so this matches the authoring convention.
func attrValueLocal(attrs []xml.Attr, local string) string {
	for _, a := range attrs {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}
