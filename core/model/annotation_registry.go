package model

import "sync"

// Payload is the interface every typed stand-off value implements — both a
// block annotation value (Block.Annotations / Layer.Annotations) and an overlay
// span value (Span.Value). It self-reports a stable type name so the wire and
// store layers can discriminate and rehydrate the concrete type. Formats and
// plugins define their own payload types; all that is required is TypeName.
type Payload interface {
	// TypeName returns the stable type name this payload registers under.
	TypeName() string
}

// PayloadFactory creates a new zero-valued typed stand-off payload for a given
// type name.
type PayloadFactory func() Payload

var (
	payloadMu        sync.RWMutex
	payloadFactories = map[string]PayloadFactory{}
)

func init() {
	// Register built-in stand-off payload types so the wire and store layers can
	// rehydrate the typed value from a type name. Block annotations:
	RegisterPayload("alt-translation", func() Payload { return &AltTranslations{} })
	RegisterPayload("note", func() Payload { return &Notes{} })
	RegisterPayload("generic", func() Payload { return &GenericAnnotation{Kind: "generic"} })
	// Overlay span payloads:
	RegisterPayload("entity", func() Payload { return &EntityAnnotation{} })
	RegisterPayload("term", func() Payload { return &TermAnnotation{} })
	RegisterPayload("term-candidate", func() Payload { return &TermCandidateAnnotation{} })
	RegisterPayload(string(OverlayEditorAnchor), func() Payload { return &EditorAnchor{} })
	// Structural layer (see structure.go):
	RegisterPayload(AnnoStructure, func() Payload { return &StructureAnnotation{} })
	RegisterPayload(AnnoGeometry, func() Payload { return &GeometryAnnotation{} })
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
func NewPayload(typeName string) (Payload, bool) {
	payloadMu.RLock()
	defer payloadMu.RUnlock()
	factory, ok := payloadFactories[typeName]
	if !ok {
		return nil, false
	}
	return factory(), true
}

// PayloadTypeName returns the type name of a stand-off payload, or "" if nil.
func PayloadTypeName(p Payload) string {
	if p == nil {
		return ""
	}
	return p.TypeName()
}
