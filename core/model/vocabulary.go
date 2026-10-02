package model

import (
	"bytes"
	"encoding/json"
	"errors"
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
	owners       map[string]string
	parents      map[string]string
	fallback     *SpanTypeInfo
	entityPrefix string
	readOnly     bool
}

// NewVocabularyRegistry creates an empty vocabulary registry.
func NewVocabularyRegistry() *VocabularyRegistry {
	return &VocabularyRegistry{
		types:   make(map[string]*SpanTypeInfo),
		owners:  make(map[string]string),
		parents: make(map[string]string),
	}
}

// Load parses and registers a vocabulary from JSON data.
// Names are unique within the registry. A parent must already be loaded, and
// a pack may override only types supplied by its ancestors. Invalid packs
// leave the registry unchanged. Load must finish before concurrent lookups.
func (r *VocabularyRegistry) Load(data []byte) error {
	if r.readOnly {
		return errors.New("the default vocabulary is read-only; create a registry to load custom packs")
	}
	var schema vocabularySchema
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&schema); err != nil {
		return fmt.Errorf("parse vocabulary: %w", err)
	}
	// Unmarshal also rejects trailing JSON values, which Decoder.Decode alone
	// accepts. Validate the full input before changing the registry.
	if !json.Valid(data) {
		return errors.New("parse vocabulary: expected one JSON document")
	}
	if err := r.validateVocabulary(schema); err != nil {
		return err
	}

	if schema.EntityPrefix != "" {
		r.entityPrefix = schema.EntityPrefix
	}
	if schema.Fallback != nil {
		r.fallback = schema.Fallback
	}

	for typeName, info := range schema.Types {
		r.types[typeName] = info
		r.owners[typeName] = schema.Name
	}
	parent := ""
	if schema.Extends != nil {
		parent = *schema.Extends
	}
	r.parents[schema.Name] = parent

	return nil
}

func (r *VocabularyRegistry) validateVocabulary(schema vocabularySchema) error {
	if schema.Name == "" || schema.Name != strings.TrimSpace(schema.Name) {
		return errors.New("vocabulary name must be non-empty and have no surrounding whitespace")
	}
	if _, exists := r.parents[schema.Name]; exists {
		return fmt.Errorf("vocabulary %q is already loaded", schema.Name)
	}
	ancestors := make(map[string]bool)
	if schema.Extends != nil {
		parent := *schema.Extends
		if _, exists := r.parents[parent]; !exists {
			return fmt.Errorf("vocabulary %q extends %q, which is not loaded", schema.Name, parent)
		}
		for parent != "" {
			ancestors[parent] = true
			parent = r.parents[parent]
		}
	}
	// Stable validation order makes a pack with several faults report the same
	// first fault on every run.
	names := make([]string, 0, len(schema.Types))
	for name := range schema.Types {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		info := schema.Types[name]
		switch {
		case name == "" || name != strings.TrimSpace(name):
			return fmt.Errorf("vocabulary %q has an empty or whitespace-padded type name", schema.Name)
		case info == nil:
			return fmt.Errorf("vocabulary %q type %q must be an object", schema.Name, name)
		case strings.TrimSpace(info.Category) == "":
			return fmt.Errorf("vocabulary %q type %q requires a category", schema.Name, name)
		}
		if owner, exists := r.owners[name]; exists && !ancestors[owner] {
			return fmt.Errorf("vocabulary %q type %q conflicts with unrelated vocabulary %q", schema.Name, name, owner)
		}
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
// The definition belongs to the registry and must be treated as read-only.
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
	seen := make(map[string]bool)
	for _, info := range r.types {
		seen[info.Category] = true
	}
	cats := make([]string, 0, len(seen))
	for cat := range seen {
		cats = append(cats, cat)
	}
	slices.Sort(cats)
	return cats
}

// TypesInCategory returns the type names in a category, sorted.
func (r *VocabularyRegistry) TypesInCategory(cat string) []string {
	types := make([]string, 0)
	for name, info := range r.types {
		if info.Category == cat {
			types = append(types, name)
		}
	}
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
	if err := r.LoadDefaults(); err != nil {
		panic(fmt.Sprintf("invalid embedded vocabularies: %v", err))
	}
	r.readOnly = true
	return r
})

// DefaultVocabulary returns the process-wide default vocabulary registry. It is
// loaded once and rejects further Load calls. Definitions returned by Lookup
// remain shared, read-only pointers. Use NewVocabularyRegistry and LoadDefaults
// to build a separate registry for custom packs. Format writers
// use it to project inline runs into their native markup on the cross-format
// (no-skeleton) path.
func DefaultVocabulary() *VocabularyRegistry { return sharedVocab() }
