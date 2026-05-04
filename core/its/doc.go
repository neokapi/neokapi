// Package its implements the W3C Internationalization Tag Set (ITS)
// 2.0 specification (https://www.w3.org/TR/its20/) for XML and HTML
// content. It exposes:
//
//   - Rule and RuleSet types that capture the data categories
//     declared inside <its:rules> elements (translateRule,
//     withinTextRule, locNoteRule, termRule, …) and via local
//     attributes (its:translate, its:locNote, its:term, …).
//   - A streaming-friendly selector evaluator implementing the subset
//     of XPath actually used by ITS authors in the wild (absolute and
//     descendant axis steps, name and attribute steps, predicates with
//     attribute-equality and ancestor:: tests, and unions).
//   - A Resolver that combines global rules with local attribute
//     overrides per the ITS inheritance/precedence rules
//     (Locally-set values win over rule-set values; child rule sets
//     win over parent rule sets; attribute selectors apply per
//     attribute, element selectors apply per element).
//
// Format readers (xml, html, dita, …) delegate to this package so the
// data category determination stays consistent across content types
// and so future categories (terminology, domain, locale filter, …)
// can be added once and consumed everywhere.
package its
