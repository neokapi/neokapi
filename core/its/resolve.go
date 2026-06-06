package its

// Resolver combines a global RuleSet with per-element local attribute
// overrides per the ITS 2.0 §5.4 precedence rules:
//
//  1. Local attributes on the element itself (its:translate="no",
//     its:locNote="…", …) win unconditionally.
//  2. Among rule-set rules, last-rule-wins: the rule appearing
//     later in document order overrides earlier rules with the
//     same data category and a matching selector.
//  3. Inheritance: when no rule matches at a given level, the
//     effective value is whatever the parent element resolved to
//     (caller responsibility — Resolver only produces per-element
//     values; callers maintain the inheritance stack).
//
// Resolver instances are cheap; create one per document and call
// Resolve for every element the reader visits.
type Resolver struct {
	rules *RuleSet
}

// NewResolver wraps a RuleSet for per-element resolution.
func NewResolver(rs *RuleSet) *Resolver {
	return &Resolver{rules: rs}
}

// Resolved is the per-element decision for every supported data
// category. Fields stay at zero (Unset / "") when no rule applied —
// callers inherit from the parent in that case.
type Resolved struct {
	Translate     Tristate
	WithinText    Tristate
	LocNoteText   string
	LocNoteType   LocNoteType
	LocNoteRef    string
	Term          Tristate
	Domain        string // resolved value when DomainPointer fires; "" otherwise
	PreserveSpace Tristate
}

// ResolveElement returns the per-element resolution for ctx, taking
// `localAttrs` into account. Local attributes are the element's own
// ITS attributes (its:translate, its:locNote, its:term, …).
func (r *Resolver) ResolveElement(ctx *ElementContext, localAttrs *LocalAttributes) Resolved {
	out := Resolved{}
	if r != nil && r.rules != nil {
		// Apply rules in document order; last match wins per ITS
		// 2.0 §5.4. We iterate and overwrite rather than reverse-
		// iterate-and-break so callers can later observe rule
		// hits in order for diagnostics.
		for i := range r.rules.Rules {
			rule := &r.rules.Rules[i]
			if !rule.Selector.MatchElement(ctx) {
				continue
			}
			applyRuleToResolved(rule, &out)
		}
	}
	if localAttrs != nil {
		applyLocalAttrs(localAttrs, &out)
	}
	return out
}

// ResolveAttribute returns the per-attribute resolution for an
// attribute on the element described by ctx. ITS data categories
// that target attributes (translateRule on @attr, locNoteRule on
// @attr, etc.) flow through here.
func (r *Resolver) ResolveAttribute(ctx *ElementContext, attr NameMatch, localAttrs *LocalAttributes) Resolved {
	out := Resolved{}
	if r != nil && r.rules != nil {
		for i := range r.rules.Rules {
			rule := &r.rules.Rules[i]
			if !rule.Selector.MatchAttribute(ctx, attr) {
				continue
			}
			applyRuleToResolved(rule, &out)
		}
	}
	if localAttrs != nil {
		applyLocalAttrs(localAttrs, &out)
	}
	return out
}

// LocalAttributes captures the ITS-namespace attributes present on
// an element. Empty / Unset fields mean the attribute wasn't set on
// this element and callers should fall back to inherited values.
type LocalAttributes struct {
	Translate     Tristate
	WithinText    Tristate
	WithinTextRaw string
	LocNote       string
	LocNoteRef    string
	LocNoteType   LocNoteType
	Term          Tristate
	TermInfoRef   string
	Domain        string
	PreserveSpace Tristate
}

func applyRuleToResolved(rule *Rule, out *Resolved) {
	switch rule.Category {
	case CatTranslate:
		if rule.Translate != Unset {
			out.Translate = rule.Translate
		}
	case CatElementsWithinText:
		if rule.WithinText != Unset {
			out.WithinText = rule.WithinText
		}
	case CatLocalizationNote:
		// Pointer-resolved notes need the document tree; readers
		// that can't resolve pointers leave LocNoteText empty here
		// and the caller falls back to inherited / no-note state.
		if rule.LocNoteText != "" {
			out.LocNoteText = rule.LocNoteText
			if rule.LocNoteType != "" {
				out.LocNoteType = rule.LocNoteType
			} else {
				out.LocNoteType = LocNoteDescription
			}
		}
		if rule.LocNoteRef != "" {
			out.LocNoteRef = rule.LocNoteRef
		}
	case CatTerminology:
		if rule.Term != Unset {
			out.Term = rule.Term
		}
	case CatPreserveSpace:
		if rule.PreserveSpace != Unset {
			out.PreserveSpace = rule.PreserveSpace
		}
	}
}

func applyLocalAttrs(la *LocalAttributes, out *Resolved) {
	if la.Translate != Unset {
		out.Translate = la.Translate
	}
	if la.WithinText != Unset {
		out.WithinText = la.WithinText
	}
	if la.LocNote != "" {
		out.LocNoteText = la.LocNote
		if la.LocNoteType != "" {
			out.LocNoteType = la.LocNoteType
		} else {
			out.LocNoteType = LocNoteDescription
		}
	}
	if la.LocNoteRef != "" {
		out.LocNoteRef = la.LocNoteRef
	}
	if la.Term != Unset {
		out.Term = la.Term
	}
	if la.PreserveSpace != Unset {
		out.PreserveSpace = la.PreserveSpace
	}
	if la.Domain != "" {
		out.Domain = la.Domain
	}
}

// LocalAttributesFrom extracts ITS local-attribute values from a
// generic `local: value` map keyed in the convention `<namespaceURI>:<local>`
// (matching the xml reader's attribute key format). Attributes not
// in the ITS namespace are ignored.
func LocalAttributesFrom(attrs map[string]string) LocalAttributes {
	la := LocalAttributes{}
	if v, ok := attrs[NamespaceURI+":translate"]; ok {
		la.Translate = ParseTristate(v)
	}
	if v, ok := attrs[NamespaceURI+":withinText"]; ok {
		la.WithinTextRaw = v
		switch v {
		case "yes", "nested":
			la.WithinText = Yes
		case "no":
			la.WithinText = No
		}
	}
	if v, ok := attrs[NamespaceURI+":locNote"]; ok {
		la.LocNote = v
	}
	if v, ok := attrs[NamespaceURI+":locNoteRef"]; ok {
		la.LocNoteRef = v
	}
	if v, ok := attrs[NamespaceURI+":locNoteType"]; ok {
		la.LocNoteType = LocNoteType(v)
	}
	if v, ok := attrs[NamespaceURI+":term"]; ok {
		la.Term = ParseTristate(v)
	}
	if v, ok := attrs[NamespaceURI+":termInfoRef"]; ok {
		la.TermInfoRef = v
	}
	if v, ok := attrs[NamespaceURI+":domain"]; ok {
		la.Domain = v
	}
	if v, ok := attrs[NamespaceURI+":preserveSpace"]; ok {
		la.PreserveSpace = ParseTristate(v)
	}
	return la
}
