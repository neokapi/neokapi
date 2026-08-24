// Package translatability answers one question about one element or attribute:
// does its text belong to the translator?
//
// The answer is the W3C HTML5 default translatability table. It is defined once,
// in packages/i18n-react/src/plugin/defaults.ts, because the JSX transform that
// first needed it is TypeScript; data/w3c.json is generated from that file and
// embedded here so the markdown and mdx readers give the same answers as the
// transform. `make check-translatability` fails if the two drift.
//
// The table is the reason there is no list of components to maintain: an element
// nobody has classified is a Container, and a Container holding direct text is
// promoted by the caller rather than skipped, so prose in an unfamiliar
// component is translated and reported, never dropped in silence.
package translatability

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:generate node --no-warnings --experimental-strip-types ../../scripts/gen-translatability.ts

//go:embed data/w3c.json
var tableJSON []byte

// Class is the W3C classification of one element.
type Class string

const (
	// Yes marks an element whose text is translatable content: h1, p, li, …
	Yes Class = "yes"
	// No marks an element whose text is code, markup or machine input and must
	// survive translation byte for byte: script, style, code, var, kbd, samp,
	// pre, textarea.
	No Class = "no"
	// Container marks an element whose content is flow rather than phrasing —
	// div, section — and every element the table does not classify, which is
	// every React component. See Classify.
	Container Class = "container"
)

type table struct {
	NonTranslatableElements         []string `json:"nonTranslatableElements"`
	TranslatableElements            []string `json:"translatableElements"`
	InlineElements                  []string `json:"inlineElements"`
	ContainerElements               []string `json:"containerElements"`
	HTMLTranslatableAttributes      []string `json:"htmlTranslatableAttributes"`
	ComponentTranslatableAttributes []string `json:"componentTranslatableAttributes"`
}

type sets struct {
	nonTranslatable map[string]bool
	translatable    map[string]bool
	inline          map[string]bool
	container       map[string]bool
	htmlAttrs       map[string]bool
	componentAttrs  map[string]bool
}

var (
	once   sync.Once
	loaded sets
)

func index(names []string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

func load() {
	var t table
	if err := json.Unmarshal(tableJSON, &t); err != nil {
		// The file is generated and embedded at build time, so a parse failure
		// is a broken build rather than a runtime condition.
		panic(fmt.Sprintf("translatability: parsing embedded w3c.json: %v", err))
	}
	loaded = sets{
		nonTranslatable: index(t.NonTranslatableElements),
		translatable:    index(t.TranslatableElements),
		inline:          index(t.InlineElements),
		container:       index(t.ContainerElements),
		htmlAttrs:       index(t.HTMLTranslatableAttributes),
		componentAttrs:  index(t.ComponentTranslatableAttributes),
	}
}

func get() sets {
	once.Do(load)
	return loaded
}

// Classify returns the W3C classification of element. An element the table does
// not name is a Container, which is how every React component arrives: the
// caller decides whether to promote it, so an unrecognized component's prose is
// never silently skipped.
func Classify(element string) Class {
	s := get()
	switch {
	case s.nonTranslatable[element]:
		return No
	case s.translatable[element]:
		return Yes
	default:
		return Container
	}
}

// IsInline reports whether element is phrasing content, which decides whether a
// container holding it can be promoted to a single translatable block.
func IsInline(element string) bool { return get().inline[element] }

// IsTranslatableAttribute reports whether the attribute carries user-visible
// text on this element. HTML and ARIA attributes count on any element; the
// component prop-name conventions (label, description, tooltip, …) count only on
// PascalCase components, because on a plain element those names are far more
// often DOM props or data-binding fields than copy.
//
// It mirrors isTranslatableAttribute in the TypeScript source, including the
// PascalCase scoping.
func IsTranslatableAttribute(attr, element string) bool {
	s := get()
	if s.htmlAttrs[attr] {
		return true
	}
	if !s.componentAttrs[attr] {
		return false
	}
	return isPascalCase(element)
}

// IsComponent reports whether element names a React component rather than an
// HTML element, by the same rule the transform uses: a leading capital.
func IsComponent(element string) bool { return isPascalCase(element) }

func isPascalCase(element string) bool {
	if element == "" {
		return false
	}
	c := element[0]
	return c >= 'A' && c <= 'Z'
}
