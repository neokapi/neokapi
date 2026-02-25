package model

import "sync"

// AnnotationFactory creates a new zero-valued Annotation of a specific type.
type AnnotationFactory func() Annotation

var (
	annotationMu        sync.RWMutex
	annotationFactories = map[string]AnnotationFactory{}
)

func init() {
	// Register built-in annotation types.
	RegisterAnnotation("alt-translation", func() Annotation { return &AltTranslation{} })
	RegisterAnnotation("note", func() Annotation { return &NoteAnnotation{} })
	RegisterAnnotation("generic", func() Annotation { return &GenericAnnotation{Type_: "generic"} })
}

// RegisterAnnotation registers a factory for the given annotation type name.
// This allows the serialization layer to create typed Annotation instances
// from wire format without knowing all concrete types at compile time.
func RegisterAnnotation(typeName string, factory AnnotationFactory) {
	annotationMu.Lock()
	defer annotationMu.Unlock()
	annotationFactories[typeName] = factory
}

// NewAnnotation creates a new Annotation of the given type.
// Returns the annotation and true if the type is registered, or nil and false otherwise.
func NewAnnotation(typeName string) (Annotation, bool) {
	annotationMu.RLock()
	defer annotationMu.RUnlock()
	factory, ok := annotationFactories[typeName]
	if !ok {
		return nil, false
	}
	return factory(), true
}
