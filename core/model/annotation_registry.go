package model

import "sync"

// PayloadFactory creates a new zero-valued typed stand-off payload for a given
// type name — either a block annotation value or an overlay span Value.
type PayloadFactory func() any

var (
	payloadMu        sync.RWMutex
	payloadFactories = map[string]PayloadFactory{}
)

func init() {
	// Register built-in stand-off payload types so the wire and store layers can
	// rehydrate the typed value from a type name. Block annotations:
	RegisterPayload("alt-translation", func() any { return &AltTranslation{} })
	RegisterPayload("note", func() any { return &NoteAnnotation{} })
	RegisterPayload("generic", func() any { return &GenericAnnotation{Kind: "generic"} })
	// Overlay span payloads:
	RegisterPayload("entity", func() any { return &EntityAnnotation{} })
	RegisterPayload("term", func() any { return &TermAnnotation{} })
	RegisterPayload("term-candidate", func() any { return &TermCandidateAnnotation{} })
}

// RegisterPayload registers a factory for the given stand-off payload type
// name, so the serialization layer can create typed payload instances from the
// wire format without knowing all concrete types at compile time. Formats and
// plugins register their own stand-off payload types here.
func RegisterPayload(typeName string, factory PayloadFactory) {
	payloadMu.Lock()
	defer payloadMu.Unlock()
	payloadFactories[typeName] = factory
}

// NewPayload creates a new typed stand-off payload for the given type name.
// Returns the payload and true if the type is registered, or nil and false.
func NewPayload(typeName string) (any, bool) {
	payloadMu.RLock()
	defer payloadMu.RUnlock()
	factory, ok := payloadFactories[typeName]
	if !ok {
		return nil, false
	}
	return factory(), true
}

// payloadTyper is implemented by stand-off payloads that self-report their type
// name (the former Annotation interface method), used by the wire/store layers
// to discriminate the typed value.
type payloadTyper interface{ AnnotationType() string }

// PayloadTypeName returns the registered type name of a stand-off payload
// value, or "" if the value does not self-report one.
func PayloadTypeName(v any) string {
	if t, ok := v.(payloadTyper); ok {
		return t.AnnotationType()
	}
	return ""
}
