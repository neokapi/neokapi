package model

import (
	"encoding/json"
	"sync"
)

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
	RegisterPayload(AnnoTiming, func() Payload { return &TimingAnnotation{} })
	RegisterPayload(AnnoRelations, func() Payload { return &RelationAnnotation{} })
	RegisterPayload(AnnoSourceOrigin, func() Payload { return &Origin{} })
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

// DecodePayload rehydrates one stand-off payload from the type name it was
// filed under and its JSON body. Every store and wire reader decodes through
// here, so a payload means the same thing whichever door it arrives through and
// no reader has to guess what a writer meant.
//
// The chain preserves rather than refuses, because the body is data already
// written and a reader that cannot name its type must still hand it back
// unharmed:
//
//   - a registered type is decoded into its concrete Go type;
//   - a GenericAnnotation body — an object naming only "type" and "fields",
//     which is exactly what GenericAnnotation marshals to — into
//     GenericAnnotation, the shape formats use to file ad-hoc metadata;
//   - anything else into RawAnnotation, which keeps the bytes verbatim.
//
// The last arm is what keeps a plugin's own schema intact: a body it defined
// names keys GenericAnnotation has no room for, and decoding it there returned
// an annotation with every field gone.
func DecodePayload(typeName string, body []byte) Payload {
	if len(body) == 0 {
		body = []byte("null")
	}
	raw := func() Payload {
		return &RawAnnotation{Kind: typeName, Body: append(json.RawMessage(nil), body...)}
	}
	if p, ok := NewPayload(typeName); ok {
		if err := json.Unmarshal(body, p); err != nil {
			return raw()
		}
		return p
	}
	if isGenericAnnotationBody(body) {
		var ga GenericAnnotation
		if err := json.Unmarshal(body, &ga); err != nil {
			return raw()
		}
		if ga.Kind == "" {
			ga.Kind = typeName
		}
		return &ga
	}
	return raw()
}

// isGenericAnnotationBody reports whether body is what a GenericAnnotation
// marshals to: a JSON object naming at least one, and only, of "type" and
// "fields". An empty object carries no discriminator either way and is kept
// verbatim, so a caller that wrote "{}" reads "{}" back.
func isGenericAnnotationBody(body []byte) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || len(fields) == 0 {
		return false
	}
	for k := range fields {
		if k != "type" && k != "fields" {
			return false
		}
	}
	return true
}
