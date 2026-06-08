package model

import "sync"

// FacetValueFactory creates a new zero-valued typed facet payload (Span.Value)
// for a given facet/annotation type name.
type FacetValueFactory func() any

var (
	facetMu        sync.RWMutex
	facetFactories = map[string]FacetValueFactory{}
)

func init() {
	// Register built-in block-scoped facet payload types so the wire and store
	// layers can rehydrate the typed Value from a type name.
	RegisterFacetValue("alt-translation", func() any { return &AltTranslation{} })
	RegisterFacetValue("note", func() any { return &NoteAnnotation{} })
	RegisterFacetValue("generic", func() any { return &GenericAnnotation{Kind: "generic"} })
	RegisterFacetValue("entity", func() any { return &EntityAnnotation{} })
	RegisterFacetValue("term", func() any { return &TermAnnotation{} })
	RegisterFacetValue("term-candidate", func() any { return &TermCandidateAnnotation{} })
}

// RegisterFacetValue registers a factory for the given facet payload type name,
// so the serialization layer can create typed payload instances from the wire
// format without knowing all concrete types at compile time. Formats and
// plugins register their own stand-off payload types here.
func RegisterFacetValue(typeName string, factory FacetValueFactory) {
	facetMu.Lock()
	defer facetMu.Unlock()
	facetFactories[typeName] = factory
}

// NewFacetValue creates a new typed facet payload for the given type name.
// Returns the payload and true if the type is registered, or nil and false.
func NewFacetValue(typeName string) (any, bool) {
	facetMu.RLock()
	defer facetMu.RUnlock()
	factory, ok := facetFactories[typeName]
	if !ok {
		return nil, false
	}
	return factory(), true
}

// RegisterAnnotation is a deprecated alias for RegisterFacetValue.
func RegisterAnnotation(typeName string, factory FacetValueFactory) {
	RegisterFacetValue(typeName, factory)
}

// NewAnnotation is a deprecated alias for NewFacetValue.
func NewAnnotation(typeName string) (any, bool) { return NewFacetValue(typeName) }

// facetTyper is implemented by facet payloads that self-report their type name
// (the former Annotation interface method), used by the wire/store layers to
// discriminate the typed Value.
type facetTyper interface{ AnnotationType() string }

// PayloadTypeName returns the registered type name of a facet payload value, or
// "" if the value does not self-report one.
func PayloadTypeName(v any) string {
	if t, ok := v.(facetTyper); ok {
		return t.AnnotationType()
	}
	return ""
}
