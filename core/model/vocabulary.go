package model

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/model/vocabularies"
)

// SpanTypeInfo describes a semantic span type from a vocabulary.
type SpanTypeInfo struct {
	Category    string         `json:"category"`
	Label       string         `json:"label"`
	HTML        HTMLRendering  `json:"html"`
	Display     TextRendering  `json:"display"`
	ChipLabel   ChipRendering  `json:"chipLabel"`
	Color       ColorScheme    `json:"color"`
	Equiv       string         `json:"equiv"`
	Constraints SpanConstraint `json:"constraints"`
}

// HTMLRendering defines how a span type renders as HTML.
type HTMLRendering struct {
	Open        string `json:"open,omitempty"`
	Close       string `json:"close,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// TextRendering defines how a span type renders as display text.
type TextRendering struct {
	Open        string `json:"open,omitempty"`
	Close       string `json:"close,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// ChipRendering defines how a span type renders as an editor chip label.
type ChipRendering struct {
	Open        string `json:"open,omitempty"`
	Close       string `json:"close,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// ColorScheme defines colors for editor rendering.
type ColorScheme struct {
	Bg     string `json:"bg"`
	Border string `json:"border"`
	Text   string `json:"text"`
}

// SpanConstraint defines editing constraints for a span type.
type SpanConstraint struct {
	Deletable   bool `json:"deletable"`
	Cloneable   bool `json:"cloneable"`
	Reorderable bool `json:"reorderable"`
}

// vocabularySchema is the JSON schema for a vocabulary file.
type vocabularySchema struct {
	Name         string                   `json:"name"`
	Version      string                   `json:"version"`
	Extends      *string                  `json:"extends"`
	EntityPrefix string                   `json:"entity_prefix,omitempty"`
	Types        map[string]*SpanTypeInfo `json:"types"`
	Fallback     *SpanTypeInfo            `json:"fallback,omitempty"`
}

// VocabularyRegistry manages loaded vocabularies and provides lookup.
type VocabularyRegistry struct {
	types        map[string]*SpanTypeInfo
	categories   map[string][]string
	fallback     *SpanTypeInfo
	entityPrefix string
}

// NewVocabularyRegistry creates an empty vocabulary registry.
func NewVocabularyRegistry() *VocabularyRegistry {
	return &VocabularyRegistry{
		types:      make(map[string]*SpanTypeInfo),
		categories: make(map[string][]string),
	}
}

// Load parses and registers a vocabulary from JSON data.
// If the vocabulary extends another, the parent must be loaded first.
func (r *VocabularyRegistry) Load(data []byte) error {
	var schema vocabularySchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return fmt.Errorf("parse vocabulary: %w", err)
	}

	if schema.EntityPrefix != "" {
		r.entityPrefix = schema.EntityPrefix
	}
	if schema.Fallback != nil {
		r.fallback = schema.Fallback
	}

	for typeName, info := range schema.Types {
		r.types[typeName] = info
		r.categories[info.Category] = append(r.categories[info.Category], typeName)
	}

	return nil
}

// LoadDefaults loads the embedded default vocabularies
// (common-formatting, rich-html, rich-jsx, code-tokens).
func (r *VocabularyRegistry) LoadDefaults() error {
	files := []string{
		"common-formatting.json",
		"rich-html.json",
		"rich-jsx.json",
		"code-tokens.json",
	}
	for _, name := range files {
		data, err := vocabularies.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read embedded vocabulary %s: %w", name, err)
		}
		if err := r.Load(data); err != nil {
			return fmt.Errorf("load vocabulary %s: %w", name, err)
		}
	}
	return nil
}

// Lookup returns the SpanTypeInfo for a semantic type name, or nil if not found.
func (r *VocabularyRegistry) Lookup(typeName string) *SpanTypeInfo {
	if info, ok := r.types[typeName]; ok {
		return info
	}
	return nil
}

// LookupOrFallback returns the SpanTypeInfo for a semantic type name,
// or the fallback info if the type is not found.
func (r *VocabularyRegistry) LookupOrFallback(typeName string) *SpanTypeInfo {
	if info := r.Lookup(typeName); info != nil {
		return info
	}
	return r.fallback
}

// Fallback returns the fallback SpanTypeInfo used for unknown types.
func (r *VocabularyRegistry) Fallback() *SpanTypeInfo {
	return r.fallback
}

// IsEntityType returns true if the type name represents an entity type.
func (r *VocabularyRegistry) IsEntityType(typeName string) bool {
	if r.entityPrefix == "" {
		return false
	}
	return strings.HasPrefix(typeName, r.entityPrefix)
}

// Categories returns the sorted list of distinct categories.
func (r *VocabularyRegistry) Categories() []string {
	cats := make([]string, 0, len(r.categories))
	for cat := range r.categories {
		cats = append(cats, cat)
	}
	slices.Sort(cats)
	return cats
}

// TypesInCategory returns the type names in a category, sorted.
func (r *VocabularyRegistry) TypesInCategory(cat string) []string {
	types := make([]string, len(r.categories[cat]))
	copy(types, r.categories[cat])
	slices.Sort(types)
	return types
}

// AllTypes returns all registered type names, sorted.
func (r *VocabularyRegistry) AllTypes() []string {
	types := make([]string, 0, len(r.types))
	for name := range r.types {
		types = append(types, name)
	}
	slices.Sort(types)
	return types
}

// HTMLOpen returns the HTML opening tag for a span type, with fallback.
func (r *VocabularyRegistry) HTMLOpen(typeName string) string {
	if info := r.Lookup(typeName); info != nil {
		return info.HTML.Open
	}
	if r.fallback != nil {
		return strings.ReplaceAll(r.fallback.HTML.Open, "{type}", typeName)
	}
	return ""
}

// HTMLClose returns the HTML closing tag for a span type, with fallback.
func (r *VocabularyRegistry) HTMLClose(typeName string) string {
	if info := r.Lookup(typeName); info != nil {
		return info.HTML.Close
	}
	if r.fallback != nil {
		return strings.ReplaceAll(r.fallback.HTML.Close, "{type}", typeName)
	}
	return ""
}

// HTMLPlaceholder returns the HTML placeholder tag for a span type, with fallback.
func (r *VocabularyRegistry) HTMLPlaceholder(typeName string) string {
	if info := r.Lookup(typeName); info != nil {
		return info.HTML.Placeholder
	}
	if r.fallback != nil {
		return strings.ReplaceAll(r.fallback.HTML.Placeholder, "{type}", typeName)
	}
	return ""
}

// sharedVocab is the process-wide default vocabulary (the embedded
// common-formatting + rich-html + rich-jsx + code-tokens packs), loaded once.
var sharedVocab = sync.OnceValue(func() *VocabularyRegistry {
	r := NewVocabularyRegistry()
	_ = r.LoadDefaults()
	return r
})

// DefaultVocabulary returns the process-wide default vocabulary registry. It is
// loaded once and shared; callers must treat it as read-only. Format writers
// use it to project inline runs into their native markup on the cross-format
// (no-skeleton) path.
func DefaultVocabulary() *VocabularyRegistry { return sharedVocab() }
